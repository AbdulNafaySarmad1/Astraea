package platform

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) listLogBatches(w http.ResponseWriter, r *http.Request) {
	tenant, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			fail(w, 400, "limit must be 1 to 100")
			return
		}
		limit = parsed
	}
	before := time.Now().UTC().Add(time.Minute)
	if value := r.URL.Query().Get("before"); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			fail(w, 400, "invalid before time")
			return
		}
		before = parsed
	}
	tx, ok := s.tenantRead(w, r, tenant)
	if !ok {
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `SELECT connector_id,batch_id,source_name,source_kind,collected_at,received_at,line_count,status,archive_attempts,archive_retry_at,cleanup_attempts,cleanup_retry_at,archived_at,hot_deleted_at,legal_hold
 FROM log_batches WHERE tenant_id=$1 AND received_at<$2 ORDER BY received_at DESC LIMIT $3`, tenant, before, limit)
	if err != nil {
		fail(w, 503, "service log metadata unavailable")
		return
	}
	items := []map[string]any{}
	for rows.Next() {
		var connector, batch, source, kind, status string
		var collected, received time.Time
		var archived, deleted *time.Time
		var count int
		var attempts int
		var retryAt time.Time
		var cleanupAttempts int
		var cleanupRetryAt time.Time
		var hold bool
		if err = rows.Scan(&connector, &batch, &source, &kind, &collected, &received, &count, &status, &attempts, &retryAt, &cleanupAttempts, &cleanupRetryAt, &archived, &deleted, &hold); err != nil {
			rows.Close()
			fail(w, 503, "service log metadata unavailable")
			return
		}
		items = append(items, map[string]any{"connector_id": connector, "batch_id": batch, "source_name": source, "source_kind": kind, "collected_at": collected, "received_at": received, "line_count": count, "status": status, "archive_attempts": attempts, "archive_retry_at": retryAt, "cleanup_attempts": cleanupAttempts, "cleanup_retry_at": cleanupRetryAt, "archived_at": archived, "hot_deleted_at": deleted, "legal_hold": hold})
	}
	err = rows.Err()
	rows.Close()
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "service log metadata unavailable")
		return
	}
	if err = s.log(r, &tenant, "service_logs.metadata_viewed", tenant, "success", map[string]any{"count": len(items)}, true); err != nil {
		fail(w, 503, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) setLogLegalHold(w http.ResponseWriter, r *http.Request) {
	tenant, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	connector, batch := r.PathValue("connector"), r.PathValue("batch")
	if len(batch) != 32 || strings.ToLower(batch) != batch {
		fail(w, 400, "invalid log batch ID")
		return
	}
	if _, err := hex.DecodeString(batch); err != nil {
		fail(w, 400, "invalid log batch ID")
		return
	}
	var body struct {
		Engaged *bool `json:"engaged"`
	}
	if decode(r, &body) != nil || body.Engaged == nil {
		fail(w, 400, "engaged boolean required")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "legal hold unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), "UPDATE log_batches SET legal_hold=$4,cleanup_retry_at=CASE WHEN $4=false THEN now() ELSE cleanup_retry_at END WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3", tenant, connector, batch, *body.Engaged)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 404, "log batch not found")
		return
	}
	_, err = s.Store.AuditTx(r.Context(), tx, &tenant, actorFrom(r.Context()).Subject, "service_logs.legal_hold_changed", batch, "", "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"connector_id": connector, "engaged": *body.Engaged}, true)
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "legal hold audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"legal_hold": *body.Engaged})
}

func (s *Server) retryLogArchive(w http.ResponseWriter, r *http.Request) {
	tenant, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	connector, batch := r.PathValue("connector"), r.PathValue("batch")
	if len(batch) != 32 || strings.ToLower(batch) != batch {
		fail(w, 400, "invalid log batch ID")
		return
	}
	if _, err := hex.DecodeString(batch); err != nil {
		fail(w, 400, "invalid log batch ID")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "archive retry unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `UPDATE log_batches SET status='hot',archive_attempts=0,archive_retry_at=now(),archive_last_error=NULL
 WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3 AND status='blocked'`, tenant, connector, batch)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 404, "blocked log batch not found")
		return
	}
	_, err = s.Store.AuditTx(r.Context(), tx, &tenant, actorFrom(r.Context()).Subject, "service_logs.archive_retry_requested", batch, "", "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"connector_id": connector}, true)
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "archive retry audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "hot"})
}
