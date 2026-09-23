package platform

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Server struct {
	Store    *Store
	Config   Config
	Verifier *OIDCVerifier
}

func NewServer(s *Store, c Config) *Server {
	return &Server{Store: s, Config: c, Verifier: NewOIDCVerifier(c.OIDCIssuer, c.OIDCAudience)}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func decode(r *http.Request, out any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" || r.URL.Path == "/v1/connectors/enroll" || strings.HasPrefix(r.URL.Path, "/v1/connectors/heartbeat") || r.URL.Path == "/v1/public/contact" {
			next.ServeHTTP(w, r)
			return
		}
		var a Actor
		var err error
		if s.Config.Mode == "demo" && r.Header.Get("X-Demo-Actor") == "console" {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				a = Actor{Subject: "demo-operator", Roles: map[string]bool{"platform_admin": true}}
			}
		}
		if a.Subject == "" {
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if token == r.Header.Get("Authorization") || token == "" {
				_, _ = s.Store.Audit(r.Context(), nil, "anonymous", "authentication.failed", r.URL.Path, "", "denied", requestIP(r, s.Config.ProxyCIDRs), nil, false)
				fail(w, 401, "authentication required")
				return
			}
			a, err = s.Verifier.Verify(r.Context(), token)
			if err != nil {
				_, _ = s.Store.Audit(r.Context(), nil, "anonymous", "authentication.failed", r.URL.Path, "", "denied", requestIP(r, s.Config.ProxyCIDRs), nil, false)
				fail(w, 401, "invalid session")
				return
			}
		}
		a.IP = requestIP(r, s.Config.ProxyCIDRs)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, a)))
	})
}
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.Store.DB.Ping(ctx) != nil {
			fail(w, 503, "database unavailable")
			return
		}
		if s.Config.Mode != "demo" && (s.Store.Reader == nil || s.Store.Reader.Ping(ctx) != nil) {
			fail(w, 503, "tenant reader unavailable")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /v1/me", s.me)
	mux.HandleFunc("GET /v1/tenants", s.listTenants)
	mux.HandleFunc("POST /v1/tenants", s.createTenant)
	mux.HandleFunc("GET /v1/tenants/{tenant}/overview", s.overview)
	mux.HandleFunc("GET /v1/tenants/{tenant}/incidents", s.incidents)
	mux.HandleFunc("GET /v1/tenants/{tenant}/metrics", s.metrics)
	mux.HandleFunc("GET /v1/tenants/{tenant}/audit", s.auditList)
	mux.HandleFunc("GET /v1/tenants/{tenant}/notifications", s.notifications)
	mux.HandleFunc("POST /v1/tenants/{tenant}/domains", s.createDomain)
	mux.HandleFunc("GET /v1/tenants/{tenant}/domains", s.domains)
	mux.HandleFunc("POST /v1/tenants/{tenant}/domains/{domain}/verify", s.verifyDomain)
	mux.HandleFunc("POST /v1/tenants/{tenant}/enrollment-tokens", s.enrollmentToken)
	mux.HandleFunc("GET /v1/tenants/{tenant}/connectors", s.connectors)
	mux.HandleFunc("POST /v1/tenants/{tenant}/connectors/{connector}/revoke", s.revokeConnector)
	mux.HandleFunc("POST /v1/tenants/{tenant}/monitoring/approve", s.approveMonitoring)
	mux.HandleFunc("POST /v1/tenants/{tenant}/operations/approve", s.approveOperations)
	mux.HandleFunc("POST /v1/tenants/{tenant}/kill-switch", s.killSwitch)
	mux.HandleFunc("GET /v1/tenants/{tenant}/policies", s.policies)
	mux.HandleFunc("PUT /v1/tenants/{tenant}/policies/{actionType}", s.upsertPolicy)
	mux.HandleFunc("POST /v1/connectors/enroll", s.enroll)
	mux.HandleFunc("POST /v1/connectors/heartbeat", s.heartbeat)
	mux.HandleFunc("POST /v1/tenants/{tenant}/actions", s.proposeAction)
	mux.HandleFunc("GET /v1/tenants/{tenant}/investigations", s.investigations)
	mux.HandleFunc("POST /v1/tenants/{tenant}/investigations", s.startInvestigation)
	mux.HandleFunc("POST /v1/platform/kill-switch", s.platformKillSwitch)
	mux.HandleFunc("POST /v1/tenants/{tenant}/actions/{action}/approve", s.approveAction)
	mux.HandleFunc("POST /v1/tenants/{tenant}/actions/{action}/execute", s.executeAction)
	mux.HandleFunc("GET /v1/tenants/{tenant}/actions", s.actions)
	mux.HandleFunc("GET /v1/edge", s.edgeStatus)
	mux.HandleFunc("POST /v1/public/contact", s.publicContact)
	return s.securityHeaders(s.auth(mux))
}
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) allow(w http.ResponseWriter, r *http.Request, permission string) (string, bool) {
	id := r.PathValue("tenant")
	if !s.Store.TenantAllowed(r.Context(), actorFrom(r.Context()), id, permission) {
		a := actorFrom(r.Context())
		_, _ = s.Store.Audit(r.Context(), nil, a.Subject, "authorization.denied", r.URL.Path, "", "denied", a.IP, map[string]string{"permission": permission}, false)
		fail(w, 403, "tenant permission denied")
		return "", false
	}
	return id, true
}
func (s *Server) log(r *http.Request, tenant *string, kind, resource, outcome string, detail any, notify bool) error {
	a := actorFrom(r.Context())
	_, err := s.Store.Audit(r.Context(), tenant, a.Subject, kind, resource, r.Header.Get("X-Correlation-ID"), outcome, a.IP, detail, notify)
	return err
}
func (s *Server) tenantRead(w http.ResponseWriter, r *http.Request, id string) (pgx.Tx, bool) {
	tx, err := s.Store.TenantRead(r.Context(), id)
	if err != nil {
		fail(w, 503, "tenant read boundary unavailable")
		return nil, false
	}
	return tx, true
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	roles := []string{}
	for role := range a.Roles {
		roles = append(roles, role)
	}
	writeJSON(w, 200, map[string]any{"subject": a.Subject, "roles": roles})
}
func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	var rows pgx.Rows
	var err error
	if a.Roles["platform_admin"] {
		rows, err = s.Store.DB.Query(r.Context(), "SELECT id,name,slug,status,isolation_mode,monitoring_approved,operations_approved,kill_switch,created_at FROM tenants ORDER BY name LIMIT 200")
	} else {
		rows, err = s.Store.DB.Query(r.Context(), "SELECT t.id,t.name,t.slug,t.status,t.isolation_mode,t.monitoring_approved,t.operations_approved,t.kill_switch,t.created_at FROM tenants t JOIN memberships m ON m.tenant_id=t.id WHERE m.subject=$1 ORDER BY t.name LIMIT 200", a.Subject)
	}
	if err != nil {
		fail(w, 503, "tenant registry unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, slug, status, isolation string
		var mon, ops, kill bool
		var at time.Time
		if rows.Scan(&id, &name, &slug, &status, &isolation, &mon, &ops, &kill, &at) != nil {
			fail(w, 500, "query failed")
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "slug": slug, "status": status, "isolation_mode": isolation, "monitoring_approved": mon, "operations_approved": ops, "kill_switch": kill, "created_at": at})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if !a.has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	var body struct {
		Name                string   `json:"name"`
		Slug                string   `json:"slug"`
		IsolationMode       string   `json:"isolation_mode"`
		AuthorizationRecord string   `json:"authorization_record"`
		Recipients          []string `json:"notification_recipients"`
	}
	if decode(r, &body) != nil || len(body.Name) < 3 || !regexp.MustCompile(`^[a-z][a-z0-9-]{2,48}$`).MatchString(body.Slug) || len(body.AuthorizationRecord) < 8 || len(body.Recipients) == 0 {
		fail(w, 400, "name, slug, authorization record and recipients required")
		return
	}
	if body.IsolationMode != "dedicated" {
		body.IsolationMode = "shared"
	}
	for _, e := range body.Recipients {
		if !strings.Contains(e, "@") || strings.ContainsAny(e, "\r\n") {
			fail(w, 400, "invalid recipient")
			return
		}
	}
	var id string
	err := s.Store.DB.QueryRow(r.Context(), "INSERT INTO tenants(name,slug,isolation_mode,authorization_record,notification_recipients) VALUES($1,$2,$3,$4,$5) RETURNING id", body.Name, body.Slug, body.IsolationMode, body.AuthorizationRecord, body.Recipients).Scan(&id)
	if err != nil {
		fail(w, 409, "tenant creation failed")
		return
	}
	s.log(r, &id, "tenant.created", id, "success", map[string]any{"name": body.Name}, true)
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	var name, status, isolation string
	var mon, ops, kill bool
	var recipients []string
	err := read.QueryRow(r.Context(), "SELECT name,status,isolation_mode,monitoring_approved,operations_approved,kill_switch,notification_recipients FROM tenants WHERE id=$1", id).Scan(&name, &status, &isolation, &mon, &ops, &kill, &recipients)
	if err != nil {
		fail(w, 404, "tenant not found")
		return
	}
	rows, err := read.Query(r.Context(), "SELECT name,kind,status,observed_at FROM components WHERE tenant_id=$1 ORDER BY name", id)
	if err != nil {
		fail(w, 503, "components unavailable")
		return
	}
	defer rows.Close()
	components := []map[string]any{}
	for rows.Next() {
		var n, k, st string
		var at *time.Time
		if rows.Scan(&n, &k, &st, &at) != nil {
			break
		}
		fresh := at != nil && time.Since(*at) < 5*time.Minute
		if !fresh {
			st = "stale"
		}
		components = append(components, map[string]any{"name": n, "kind": k, "status": st, "observed_at": at})
	}
	rows.Close()
	if rows.Err() != nil {
		fail(w, 503, "components unavailable")
		return
	}
	var connectorCount, incidentCount, approvalCount int
	if read.QueryRow(r.Context(), "SELECT count(*) FROM connectors WHERE tenant_id=$1 AND revoked_at IS NULL AND last_seen_at>now()-interval '5 minutes'", id).Scan(&connectorCount) != nil ||
		read.QueryRow(r.Context(), "SELECT count(*) FROM incidents WHERE tenant_id=$1 AND status<>'resolved'", id).Scan(&incidentCount) != nil ||
		read.QueryRow(r.Context(), "SELECT count(*) FROM action_requests WHERE tenant_id=$1 AND status='pending'", id).Scan(&approvalCount) != nil {
		fail(w, 503, "overview counts unavailable")
		return
	}
	if s.log(r, &id, "customer.overview.viewed", id, "success", map[string]string{"category": "component inventory"}, true) != nil {
		fail(w, 503, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "status": status, "isolation_mode": isolation, "monitoring_approved": mon, "operations_approved": ops, "kill_switch": kill, "notification_recipients": recipients, "connected_connectors": connectorCount, "open_incidents": incidentCount, "pending_approvals": approvalCount, "components": components})
}
func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	rows, err := read.Query(r.Context(), "SELECT id,title,severity,status,opened_at,acknowledged_at,resolved_at,summary FROM incidents WHERE tenant_id=$1 ORDER BY opened_at DESC LIMIT 100", id)
	if err != nil {
		fail(w, 503, "incidents unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var iid, title, severity, status, summary string
		var opened time.Time
		var ack, res *time.Time
		if rows.Scan(&iid, &title, &severity, &status, &opened, &ack, &res, &summary) != nil {
			break
		}
		items = append(items, map[string]any{"id": iid, "title": title, "severity": severity, "status": status, "opened_at": opened, "acknowledged_at": ack, "resolved_at": res, "summary": summary})
	}
	if s.log(r, &id, "incidents.viewed", id, "success", map[string]any{"count": len(items)}, true) != nil {
		fail(w, 503, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "audit")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	limit := 50
	if q, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil && q > 0 && q <= 200 {
		limit = q
	}
	rows, err := read.Query(r.Context(), "SELECT id,actor,event_type,resource,correlation_id,outcome,occurred_at,detail FROM audit_events WHERE tenant_id=$1 ORDER BY occurred_at DESC LIMIT $2", id, limit)
	if err != nil {
		fail(w, 503, "audit unavailable")
		return
	}
	defer rows.Close()
	items := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		e.TenantID = &id
		if rows.Scan(&e.ID, &e.Actor, &e.EventType, &e.Resource, &e.CorrelationID, &e.Outcome, &e.OccurredAt, &e.Detail) != nil {
			break
		}
		items = append(items, e)
	}
	s.log(r, &id, "audit.viewed", id, "success", map[string]any{"count": len(items)}, false)
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "audit")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	rows, err := read.Query(r.Context(), "SELECT o.id,o.recipient,o.status,o.attempts,o.delivered_at,e.event_type,e.occurred_at FROM notification_outbox o JOIN audit_events e ON e.id=o.audit_event_id WHERE o.tenant_id=$1 ORDER BY e.occurred_at DESC LIMIT 100", id)
	if err != nil {
		fail(w, 503, "notifications unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var oid, recipient, status, kind string
		var attempts int
		var delivered *time.Time
		var at time.Time
		if rows.Scan(&oid, &recipient, &status, &attempts, &delivered, &kind, &at) != nil {
			break
		}
		items = append(items, map[string]any{"id": oid, "recipient": recipient, "status": status, "attempts": attempts, "delivered_at": delivered, "event_type": kind, "occurred_at": at})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func normalizeHost(input string) (string, error) {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input)), ".")
	if len(h) > 253 || net.ParseIP(h) != nil || strings.ContainsAny(h, "/:?#@\\") {
		return "", errors.New("invalid hostname")
	}
	parts := strings.Split(h, ".")
	if len(parts) < 2 {
		return "", errors.New("hostname requires public suffix")
	}
	for _, p := range parts {
		if !labelPattern.MatchString(p) {
			return "", errors.New("invalid hostname label")
		}
	}
	if len(parts[len(parts)-1]) < 2 {
		return "", errors.New("invalid suffix")
	}
	return h, nil
}
func (s *Server) createDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	var b struct {
		Hostname string `json:"hostname"`
		Surface  string `json:"surface"`
	}
	if decode(r, &b) != nil {
		fail(w, 400, "invalid request")
		return
	}
	host, err := normalizeHost(b.Hostname)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if b.Surface != "customer" && b.Surface != "health_target" {
		fail(w, 400, "unsupported surface")
		return
	}
	challenge, err := randomToken()
	if err != nil {
		fail(w, 500, "challenge unavailable")
		return
	}
	var did string
	err = s.Store.DB.QueryRow(r.Context(), "INSERT INTO domains(tenant_id,hostname,surface,challenge_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '24 hours') RETURNING id", id, host, b.Surface, digest(challenge)).Scan(&did)
	if err != nil {
		fail(w, 409, "hostname already registered")
		return
	}
	s.log(r, &id, "domain.challenge.created", host, "success", nil, true)
	writeJSON(w, 201, map[string]string{"id": did, "hostname": host, "txt_name": "_nocturn-verification." + host, "txt_value": "nocturn=" + challenge, "expires_in": "24h"})
}
func (s *Server) verifyDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	did := r.PathValue("domain")
	var host, hash, status string
	var expires time.Time
	err := s.Store.DB.QueryRow(r.Context(), "SELECT hostname,challenge_hash,status,expires_at FROM domains WHERE id=$1 AND tenant_id=$2", did, id).Scan(&host, &hash, &status, &expires)
	if err != nil {
		fail(w, 404, "domain not found")
		return
	}
	if status != "pending" || time.Now().After(expires) {
		fail(w, 409, "challenge expired or already used")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	records, err := net.DefaultResolver.LookupTXT(ctx, "_nocturn-verification."+host)
	if err != nil {
		fail(w, 503, "DNS lookup unavailable")
		return
	}
	matched := false
	for _, txt := range records {
		if strings.HasPrefix(txt, "nocturn=") && subtle.ConstantTimeCompare([]byte(digest(strings.TrimPrefix(txt, "nocturn="))), []byte(hash)) == 1 {
			matched = true
		}
	}
	if !matched {
		fail(w, 409, "DNS challenge not found")
		return
	}
	_, err = s.Store.DB.Exec(r.Context(), "UPDATE domains SET status='verified',verified_at=now() WHERE id=$1 AND tenant_id=$2 AND status='pending'", did, id)
	if err != nil {
		fail(w, 503, "verification could not be saved")
		return
	}
	s.log(r, &id, "domain.verified", host, "success", nil, true)
	writeJSON(w, 200, map[string]string{"status": "verified"})
}
func (s *Server) enrollmentToken(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	var authorized string
	err := s.Store.DB.QueryRow(r.Context(), "SELECT authorization_record FROM tenants WHERE id=$1", id).Scan(&authorized)
	if err != nil || authorized == "" {
		fail(w, 409, "customer authorization required")
		return
	}
	token, err := randomToken()
	if err != nil {
		fail(w, 500, "token unavailable")
		return
	}
	_, err = s.Store.DB.Exec(r.Context(), "INSERT INTO enrollment_tokens(tenant_id,token_hash,expires_at,created_by) VALUES($1,$2,now()+interval '15 minutes',$3)", id, digest(token), actorFrom(r.Context()).Subject)
	if err != nil {
		fail(w, 503, "token unavailable")
		return
	}
	s.log(r, &id, "connector.enrollment.issued", id, "success", nil, true)
	writeJSON(w, 201, map[string]string{"token": token, "expires_in": "15m"})
}
func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Token        string   `json:"token"`
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if decode(r, &b) != nil || len(b.Token) != 64 || len(b.Name) > 80 || b.Name == "" || len(b.Capabilities) > 20 {
		fail(w, 400, "invalid enrollment")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "enrollment unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var tenant string
	err = tx.QueryRow(r.Context(), "UPDATE enrollment_tokens SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING tenant_id", digest(b.Token)).Scan(&tenant)
	if err != nil {
		fail(w, 403, "enrollment token invalid")
		return
	}
	secret, err := randomToken()
	if err != nil {
		fail(w, 500, "identity unavailable")
		return
	}
	var cid string
	err = tx.QueryRow(r.Context(), "INSERT INTO connectors(tenant_id,name,version,capabilities,credential_hash) VALUES($1,$2,$3,$4,$5) RETURNING id", tenant, b.Name, b.Version, b.Capabilities, digest(secret)).Scan(&cid)
	if err != nil {
		fail(w, 503, "enrollment failed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 503, "enrollment failed")
		return
	}
	_, _ = s.Store.Audit(r.Context(), &tenant, "connector:"+cid, "connector.enrolled", cid, "", "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"version": b.Version, "capabilities": b.Capabilities}, true)
	writeJSON(w, 201, map[string]string{"connector_id": cid, "tenant_id": tenant, "credential": secret})
}
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || len(token) != 64 {
		fail(w, 401, "connector credential required")
		return
	}
	var b struct {
		Version    string `json:"version"`
		Components []struct {
			Name       string    `json:"name"`
			Kind       string    `json:"kind"`
			Status     string    `json:"status"`
			ObservedAt time.Time `json:"observed_at"`
			LatencyMS  *float64  `json:"latency_ms"`
		} `json:"components"`
	}
	if decode(r, &b) != nil || len(b.Components) > 100 {
		fail(w, 400, "invalid telemetry")
		return
	}
	var tenant, cid string
	var approved, kill bool
	err := s.Store.DB.QueryRow(r.Context(), "SELECT c.tenant_id,c.id,t.monitoring_approved,t.kill_switch FROM connectors c JOIN tenants t ON t.id=c.tenant_id WHERE c.credential_hash=$1 AND c.revoked_at IS NULL", digest(token)).Scan(&tenant, &cid, &approved, &kill)
	if err != nil {
		fail(w, 401, "connector revoked or unknown")
		return
	}
	if kill || !approved {
		fail(w, 403, "monitoring disabled")
		return
	}
	_, err = s.Store.DB.Exec(r.Context(), "UPDATE connectors SET last_seen_at=now(),last_telemetry_at=now(),version=$2 WHERE id=$1", cid, b.Version)
	if err != nil {
		fail(w, 503, "telemetry unavailable")
		return
	}
	for _, c := range b.Components {
		if len(c.Name) > 120 || len(c.Kind) > 60 || c.Name == "" || strings.ContainsAny(c.Name, "\r\n") || c.ObservedAt.After(time.Now().Add(time.Minute)) || time.Since(c.ObservedAt) > time.Hour {
			continue
		}
		if c.Status != "healthy" && c.Status != "degraded" && c.Status != "unknown" {
			continue
		}
		_, _ = s.Store.DB.Exec(r.Context(), "INSERT INTO components(tenant_id,connector_id,name,kind,status,observed_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,name) DO UPDATE SET status=EXCLUDED.status,observed_at=EXCLUDED.observed_at,connector_id=EXCLUDED.connector_id", tenant, cid, c.Name, c.Kind, c.Status, c.ObservedAt)
		if c.LatencyMS != nil && *c.LatencyMS >= 0 && *c.LatencyMS <= 30000 {
			_, _ = s.Store.DB.Exec(r.Context(), "INSERT INTO telemetry(tenant_id,connector_id,metric,value,unit,observed_at) VALUES($1,$2,'probe_latency_ms',$3,'ms',$4)", tenant, cid, *c.LatencyMS, c.ObservedAt)
			if s.Config.InfluxURL != "" {
				line := fmt.Sprintf("probe_latency,tenant=%s,connector=%s value=%f %d", tenant, cid, *c.LatencyMS, c.ObservedAt.UnixNano())
				payload, _ := json.Marshal(line)
				_, _ = s.Store.DB.Exec(r.Context(), "INSERT INTO jobs(tenant_id,kind,payload,idempotency_key) VALUES($1,'influx_write',$2,$3) ON CONFLICT DO NOTHING", tenant, payload, cid+":"+c.Name+":"+c.ObservedAt.Format(time.RFC3339Nano))
			}
		}
	}
	_, _ = s.Store.Audit(r.Context(), &tenant, "connector:"+cid, "telemetry.collected", cid, "", "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"component_count": len(b.Components)}, false)
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	window := r.URL.Query().Get("window")
	if window != "1h" && window != "24h" && window != "7d" {
		window = "24h"
	}
	hours := 24
	if window == "1h" {
		hours = 1
	}
	if window == "7d" {
		hours = 168
	}
	rows, err := read.Query(r.Context(), "SELECT observed_at,avg(value) FROM telemetry WHERE tenant_id=$1 AND metric='probe_latency_ms' AND observed_at>=now()-($2::int * interval '1 hour') GROUP BY observed_at ORDER BY observed_at LIMIT 500", id, hours)
	if err != nil {
		fail(w, 503, "telemetry unavailable")
		return
	}
	defer rows.Close()
	points := []map[string]any{}
	for rows.Next() {
		var at time.Time
		var value float64
		if rows.Scan(&at, &value) != nil {
			break
		}
		points = append(points, map[string]any{"at": at, "value": value})
	}
	if s.log(r, &id, "telemetry.viewed", id, "success", map[string]any{"window": window}, true) != nil {
		fail(w, 503, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"metric": "probe_latency_ms", "unit": "ms", "window": window, "points": points})
}
func (s *Server) edgeStatus(w http.ResponseWriter, r *http.Request) {
	if !actorFrom(r.Context()).platform() {
		fail(w, 403, "platform role required")
		return
	}
	var provider, status string
	var protected bool
	var checked *time.Time
	err := s.Store.DB.QueryRow(r.Context(), "SELECT provider,status,origin_protected,last_checked_at FROM edge_settings WHERE id=true").Scan(&provider, &status, &protected, &checked)
	if err != nil {
		fail(w, 503, "edge status unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"provider": provider, "status": status, "origin_protected": protected, "last_checked_at": checked})
}
func (s *Server) publicContact(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Email          string `json:"email"`
		Message        string `json:"message"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if decode(r, &b) != nil || len(b.Email) > 254 || len(b.Message) > 2000 || !strings.Contains(b.Email, "@") {
		fail(w, 400, "invalid contact request")
		return
	}
	if s.Config.TurnstileSecret == "" {
		fail(w, 503, "public contact is disabled")
		return
	}
	if err := s.verifyTurnstile(r.Context(), b.TurnstileToken, requestIP(r, s.Config.ProxyCIDRs)); err != nil {
		fail(w, 403, "verification failed; retry the challenge")
		return
	}
	_, err := s.Store.DB.Exec(r.Context(), "INSERT INTO contact_requests(email,message) VALUES($1,$2)", b.Email, b.Message)
	if err != nil {
		fail(w, 503, "contact service unavailable")
		return
	}
	writeJSON(w, 202, map[string]string{"status": "accepted"})
}

type turnstileResult struct {
	Success     bool      `json:"success"`
	Hostname    string    `json:"hostname"`
	Action      string    `json:"action"`
	ChallengeTS time.Time `json:"challenge_ts"`
}

func validateTurnstileResult(result turnstileResult, hostname, action string, now time.Time) error {
	if !result.Success || now.Sub(result.ChallengeTS) > 5*time.Minute || result.ChallengeTS.After(now.Add(time.Minute)) {
		return errors.New("challenge invalid")
	}
	if hostname != "" && result.Hostname != hostname {
		return errors.New("wrong hostname")
	}
	if result.Action != action {
		return errors.New("wrong action")
	}
	return nil
}
func (s *Server) verifyTurnstile(ctx context.Context, token, ip string) error {
	if len(token) < 1 || len(token) > 2048 {
		return errors.New("invalid token")
	}
	body := strings.NewReader("secret=" + urlQuery(s.Config.TurnstileSecret) + "&response=" + urlQuery(token) + "&remoteip=" + urlQuery(ip))
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://challenges.cloudflare.com/turnstile/v0/siteverify", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("siteverify %d", resp.StatusCode)
	}
	var result turnstileResult
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil {
		return errors.New("invalid siteverify response")
	}
	return validateTurnstileResult(result, s.Config.TurnstileHostname, s.Config.TurnstileAction, time.Now())
}
func urlQuery(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
