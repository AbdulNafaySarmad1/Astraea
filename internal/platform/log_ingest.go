package platform

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"nocturn.example/aegis-operations/internal/logsafe"
)

type incomingLogBatch struct {
	TenantID    string    `json:"tenant_id"`
	ConnectorID string    `json:"connector_id"`
	BatchID     string    `json:"batch_id"`
	SourceName  string    `json:"source_name"`
	SourceKind  string    `json:"source_kind"`
	PathSHA256  string    `json:"path_sha256"`
	CollectedAt time.Time `json:"collected_at"`
	Lines       []string  `json:"lines"`
}

func (s *Server) ingestLogs(w http.ResponseWriter, r *http.Request) {
	storage, err := openHotLogStorage(s.Config)
	if err != nil {
		fail(w, 503, "service log storage unavailable")
		return
	}
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") || len(authorization) != len("Bearer ")+64 {
		fail(w, 401, "connector credential required")
		return
	}
	var batch incomingLogBatch
	if decode(r, &batch) != nil || len(batch.BatchID) != 32 || !logSourceName.MatchString(batch.SourceName) || len(batch.PathSHA256) != 64 || len(batch.Lines) == 0 || len(batch.Lines) > 1024 {
		fail(w, 400, "invalid service log batch")
		return
	}
	if _, err := hex.DecodeString(batch.BatchID); err != nil || batch.BatchID != strings.ToLower(batch.BatchID) || batch.CollectedAt.IsZero() || batch.CollectedAt.Before(time.Now().Add(-90*24*time.Hour)) || batch.CollectedAt.After(time.Now().Add(time.Minute)) {
		fail(w, 400, "invalid service log batch identity or time")
		return
	}
	if _, err := hex.DecodeString(batch.PathSHA256); err != nil || batch.PathSHA256 != strings.ToLower(batch.PathSHA256) {
		fail(w, 400, "invalid service log path binding")
		return
	}
	if batch.SourceKind != "postgres" && batch.SourceKind != "valkey" && batch.SourceKind != "vault" && batch.SourceKind != "aegiscore" {
		fail(w, 400, "unsupported service log kind")
		return
	}
	var content bytes.Buffer
	kept := 0
	for _, line := range batch.Lines {
		filtered, ok := logsafe.FilterLine(batch.SourceKind, line)
		if !ok {
			continue
		}
		encoded, _ := json.Marshal(filtered)
		content.Write(encoded)
		content.WriteByte('\n')
		kept++
		if content.Len() > maxLogPlaintext {
			fail(w, 400, "service log batch too large")
			return
		}
	}
	if kept == 0 {
		fail(w, 400, "service log batch contains no permitted lines")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "service log database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var tenant, connector string
	var capabilities []string
	var monitoring, kill bool
	err = tx.QueryRow(r.Context(), `SELECT c.tenant_id,c.id,c.capabilities,t.monitoring_approved,t.kill_switch
 FROM connectors c JOIN tenants t ON t.id=c.tenant_id
 WHERE c.credential_hash=$1 AND c.revoked_at IS NULL FOR UPDATE OF c,t`, digest(strings.TrimPrefix(authorization, "Bearer "))).Scan(&tenant, &connector, &capabilities, &monitoring, &kill)
	if err != nil {
		fail(w, 401, "connector revoked or unknown")
		return
	}
	if kill || !monitoring || !slices.Contains(capabilities, "service_logs") || tenant != batch.TenantID || connector != batch.ConnectorID {
		fail(w, 403, "service log collection not authorized")
		return
	}
	var approved bool
	if tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM connector_log_sources WHERE tenant_id=$1 AND connector_id=$2 AND name=$3 AND kind=$4 AND path_sha256=$5)", tenant, connector, batch.SourceName, batch.SourceKind, batch.PathSHA256).Scan(&approved) != nil || !approved {
		fail(w, 403, "service log source not approved")
		return
	}
	hash := logDigest(content.Bytes())
	var existingHash, existingSource, existingKind, existingPathHash string
	err = tx.QueryRow(r.Context(), "SELECT plaintext_sha256,source_name,source_kind,path_sha256 FROM log_batches WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3", tenant, connector, batch.BatchID).Scan(&existingHash, &existingSource, &existingKind, &existingPathHash)
	if err == nil {
		if existingHash != hash || existingSource != batch.SourceName || existingKind != batch.SourceKind || existingPathHash != batch.PathSHA256 {
			fail(w, 409, "service log batch ID already used")
			return
		}
		stored, readErr := storage.readHot(tenant, connector, batch.BatchID)
		if readErr != nil || logDigest(stored) != hash {
			fail(w, 503, "accepted service log batch is unavailable")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "duplicate"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "service log database unavailable")
		return
	}
	if err = storage.saveHot(tenant, connector, batch.BatchID, content.Bytes()); err != nil {
		fail(w, 503, "service log storage unavailable")
		return
	}
	if err = checkStoreRoot(storage.hot, storage.hotMarker); err != nil {
		fail(w, 503, "service log storage unavailable")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO log_batches(tenant_id,connector_id,batch_id,source_name,source_kind,path_sha256,collected_at,plaintext_sha256,line_count)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenant, connector, batch.BatchID, batch.SourceName, batch.SourceKind, batch.PathSHA256, batch.CollectedAt, hash, kept)
	if err == nil {
		_, err = tx.Exec(r.Context(), "UPDATE connectors SET last_seen_at=now() WHERE id=$1 AND tenant_id=$2", connector, tenant)
	}
	if err == nil {
		_, err = s.Store.AuditTx(r.Context(), tx, &tenant, "connector:"+connector, "service_logs.collected", batch.SourceName, batch.BatchID, "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"kind": batch.SourceKind, "line_count": kept, "sha256": hash}, false)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "service log metadata unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}
