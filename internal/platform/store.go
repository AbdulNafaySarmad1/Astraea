package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	DB     *pgxpool.Pool
	Reader *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{DB: p}, nil
}

// OpenTenantReader requires a distinct non-owner login that has been granted
// nocturn_tenant_reader. A table owner or BYPASSRLS role would defeat the
// policy and is rejected before the API starts serving requests.
func (s *Store) OpenTenantReader(ctx context.Context, url string) error {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	var member, privileged, owner, policiesReady bool
	err = p.QueryRow(ctx, `SELECT pg_has_role(session_user,'nocturn_tenant_reader','MEMBER'),
      session_role.rolsuper OR session_role.rolbypassrls OR active_role.rolsuper OR active_role.rolbypassrls,
      (SELECT coalesce(bool_or(session_user = pg_get_userbyid(owned.relowner)
        OR current_user = pg_get_userbyid(owned.relowner)),true)
       FROM pg_class owned JOIN pg_policies owner_policies
         ON owner_policies.tablename=owned.relname AND owner_policies.schemaname='public'
       WHERE owner_policies.policyname='tenant_reader_scope'
         AND owned.relnamespace='public'::regnamespace),
	      (SELECT count(*)=18 AND count(DISTINCT tables.oid)=18
         AND bool_and(policies.policyname='tenant_reader_scope')
       FROM pg_class tables
       JOIN pg_policies policies ON policies.tablename=tables.relname
         AND policies.schemaname='public'
       WHERE tables.oid IN ('public.tenants'::regclass,'public.memberships'::regclass,
         'public.domains'::regclass,'public.connectors'::regclass,
         'public.enrollment_tokens'::regclass,'public.components'::regclass,
         'public.telemetry'::regclass,'public.incidents'::regclass,
         'public.policies'::regclass,'public.action_requests'::regclass,
         'public.jobs'::regclass,'public.audit_events'::regclass,
         'public.notification_outbox'::regclass,'public.knowledge_documents'::regclass,
	         'public.investigations'::regclass,'public.connector_batches'::regclass,
	         'public.connector_log_sources'::regclass,'public.log_batches'::regclass) AND tables.relrowsecurity)
      FROM pg_roles session_role, pg_roles active_role
      WHERE session_role.rolname=session_user AND active_role.rolname=current_user
      `).Scan(&member, &privileged, &owner, &policiesReady)
	if err != nil || !member || privileged || owner || !policiesReady {
		p.Close()
		if err != nil {
			return fmt.Errorf("tenant reader validation: %w", err)
		}
		return errors.New("tenant reader requires a non-owner, non-bypass login and all tenant read policies")
	}
	s.Reader = p
	return nil
}

// TenantRead scopes every read to a transaction-local tenant ID. The
// dedicated reader pool is required in production; demo mode may use DB.
func (s *Store) TenantRead(ctx context.Context, tenantID string) (pgx.Tx, error) {
	if tenantID == "" {
		return nil, errors.New("tenant ID required")
	}
	if s.Reader == nil {
		return s.DB.Begin(ctx)
	}
	tx, err := s.Reader.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SET LOCAL search_path=public,pg_catalog"); err == nil {
		_, err = tx.Exec(ctx, "SET LOCAL ROLE nocturn_tenant_reader")
	}
	if err == nil {
		_, err = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenantID)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:]), nil
}
func digest(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }

type AuditEvent struct {
	ID            string          `json:"id"`
	TenantID      *string         `json:"tenant_id"`
	Actor         string          `json:"actor"`
	EventType     string          `json:"event_type"`
	Resource      string          `json:"resource"`
	CorrelationID string          `json:"correlation_id"`
	Outcome       string          `json:"outcome"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Detail        json.RawMessage `json:"detail"`
}

func (s *Store) Audit(ctx context.Context, tenantID *string, actor, kind, resource, correlation, outcome, ip string, detail any, notify bool) (string, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	id, err := s.AuditTx(ctx, tx, tenantID, actor, kind, resource, correlation, outcome, ip, detail, notify)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// AuditTx lets ingestion commit its data and hash-chained audit record together.
func (s *Store) AuditTx(ctx context.Context, tx pgx.Tx, tenantID *string, actor, kind, resource, correlation, outcome, ip string, detail any, notify bool) (string, error) {
	raw, _ := json.Marshal(detail)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var prev string
	if err := tx.QueryRow(ctx, "SELECT event_hash FROM audit_chain_head WHERE id=true FOR UPDATE").Scan(&prev); err != nil {
		return "", err
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	material := strings.Join([]string{prev, id, actor, kind, resource, correlation, outcome, at.Format(time.RFC3339Nano), string(raw)}, "|")
	hash := digest(material)
	var tenant any
	if tenantID != nil {
		tenant = *tenantID
	}
	var source any
	if parsed := net.ParseIP(ip); parsed != nil {
		source = parsed.String()
	}
	_, err = tx.Exec(ctx, "INSERT INTO audit_events(id,tenant_id,actor,event_type,resource,correlation_id,outcome,source_ip,detail,occurred_at,previous_hash,event_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)", id, tenant, actor, kind, resource, correlation, outcome, source, raw, at, prev, hash)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, "UPDATE audit_chain_head SET event_hash=$1 WHERE id=true", hash)
	if err != nil {
		return "", err
	}
	if notify && tenantID != nil {
		_, err = tx.Exec(ctx, "INSERT INTO notification_outbox(tenant_id,audit_event_id,recipient) SELECT id,$2,unnest(notification_recipients) FROM tenants WHERE id=$1 ON CONFLICT DO NOTHING", *tenantID, id)
		if err != nil {
			return "", err
		}
	}
	return id, nil
}

func (s *Store) TenantAllowed(ctx context.Context, a Actor, id, permission string) bool {
	if id == "" || a.Subject == "" {
		return false
	}
	if a.Roles["platform_admin"] {
		return true
	}
	var role string
	err := s.DB.QueryRow(ctx, "SELECT role FROM memberships WHERE tenant_id=$1 AND subject=$2", id, a.Subject).Scan(&role)
	if err != nil {
		return false
	}
	switch permission {
	case "view":
		return role == "ops_engineer" || role == "read_only_operator" || role == "audit_reviewer" || role == "customer_viewer"
	case "audit":
		return role == "audit_reviewer" || role == "customer_viewer" || role == "ops_engineer"
	case "operate":
		return role == "ops_engineer"
	case "approve":
		return role == "ops_engineer"
	case "manage":
		return role == "ops_engineer"
	}
	return false
}
func nullTime(t sql.NullTime) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

var errDenied = errors.New("permission denied")
