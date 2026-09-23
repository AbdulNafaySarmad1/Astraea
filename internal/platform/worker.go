package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

type Worker struct {
	Store  *Store
	Config Config
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	freshnessTicker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer freshnessTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.processJob(ctx)
			w.processNotification(ctx)
		case <-freshnessTicker.C:
			w.evaluateStale(ctx)
		}
	}
}
func (w *Worker) processJob(ctx context.Context) {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, tenant, kind string
	var payload json.RawMessage
	var attempts, maxAttempts int
	err = tx.QueryRow(ctx, `SELECT j.id,j.tenant_id,j.kind,j.payload,j.attempts,j.max_attempts FROM jobs j
 WHERE (j.status='queued' OR (j.status='running' AND j.locked_until<now())) AND j.run_at<=now() AND NOT EXISTS(
 SELECT 1 FROM jobs running WHERE running.tenant_id=j.tenant_id AND running.status='running' AND running.locked_until>now())
 ORDER BY j.run_at,j.created_at FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&id, &tenant, &kind, &payload, &attempts, &maxAttempts)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, "UPDATE jobs SET status='running',attempts=attempts+1,locked_until=now()+interval '2 minutes' WHERE id=$1", id)
	if err != nil {
		return
	}
	if tx.Commit(ctx) != nil {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var runErr error
	switch kind {
	case "execute_action":
		var p struct {
			ActionID string `json:"action_id"`
		}
		if json.Unmarshal(payload, &p) != nil {
			runErr = errors.New("bad job payload")
		} else {
			runErr = w.executeMock(runCtx, tenant, p.ActionID)
		}
	case "influx_write":
		runErr = w.writeInflux(runCtx, payload)
	case "investigate":
		var p map[string]string
		if json.Unmarshal(payload, &p) != nil {
			runErr = errors.New("bad investigation payload")
		} else {
			runErr = w.investigate(runCtx, tenant, p["investigation_id"])
		}
	case "evaluate_health":
		runErr = w.evaluateHealth(runCtx, tenant)
	default:
		runErr = errors.New("unknown job kind")
	}
	if runErr == nil {
		_, _ = w.Store.DB.Exec(ctx, "UPDATE jobs SET status='done',locked_until=NULL,last_error=NULL WHERE id=$1", id)
		return
	}
	attempts++
	status := "queued"
	if attempts >= maxAttempts {
		status = "dead"
	}
	delay := time.Duration(math.Min(float64(int64(1)<<min(attempts, 8)), 256)) * time.Second
	_, _ = w.Store.DB.Exec(ctx, "UPDATE jobs SET status=$2,locked_until=NULL,last_error=$3,run_at=now()+$4::interval WHERE id=$1", id, status, runErr.Error(), fmt.Sprintf("%d seconds", int(delay.Seconds())))
	log.Printf("job %s (%s) failed: %v", id, kind, runErr)
}
func (w *Worker) executeMock(ctx context.Context, tenant, actionID string) error {
	if w.Config.Mode != "demo" {
		return errors.New("no approved connector execution adapter configured")
	}
	var status string
	var kill, ops, enabled, approval bool
	var target, kind, storedHash string
	var params json.RawMessage
	var policyVersion, currentVersion int
	var approvalExpiry *time.Time
	err := w.Store.DB.QueryRow(ctx, "SELECT a.status,a.target,a.action_type,a.parameters,a.parameter_hash,a.policy_version,a.approval_expires_at,t.kill_switch OR (SELECT kill_switch FROM platform_controls WHERE id=true),t.operations_approved,p.enabled,p.approval_required,p.version FROM action_requests a JOIN tenants t ON t.id=a.tenant_id JOIN policies p ON p.tenant_id=a.tenant_id AND p.action_type=a.action_type WHERE a.id=$1 AND a.tenant_id=$2", actionID, tenant).Scan(&status, &target, &kind, &params, &storedHash, &policyVersion, &approvalExpiry, &kill, &ops, &enabled, &approval, &currentVersion)
	if err != nil {
		return err
	}
	if status != "queued" || kill || !ops || !enabled || policyVersion != currentVersion {
		return errors.New("action no longer allowed")
	}
	var parsed map[string]any
	_ = json.Unmarshal(params, &parsed)
	if storedHash != actionHash(tenant, ActionInput{Target: target, ActionType: kind, Parameters: parsed}, currentVersion) {
		return errors.New("parameters changed")
	}
	if approval && (approvalExpiry == nil || time.Now().After(*approvalExpiry)) {
		return errors.New("approval expired")
	}
	_, err = w.Store.DB.Exec(ctx, "UPDATE action_requests SET status='running' WHERE id=$1 AND tenant_id=$2 AND status='queued'", actionID, tenant)
	if err != nil {
		return err
	}
	_, _ = w.Store.Audit(ctx, &tenant, "worker:mock", "operation.started", actionID, "", "success", "", map[string]string{"target": target, "action_type": kind}, true)
	// The mock connector records a simulated result. It never reaches customer infrastructure.
	_, err = w.Store.DB.Exec(ctx, "UPDATE action_requests SET status='simulated',completed_at=now() WHERE id=$1 AND tenant_id=$2 AND status='running'", actionID, tenant)
	if err != nil {
		return err
	}
	_, _ = w.Store.Audit(ctx, &tenant, "worker:mock", "operation.verified", actionID, "", "simulated", "", map[string]string{"target": target}, true)
	return nil
}
func (w *Worker) writeInflux(ctx context.Context, payload []byte) error {
	if w.Config.InfluxURL == "" || w.Config.InfluxToken == "" || w.Config.InfluxDatabase == "" {
		return errors.New("InfluxDB not configured")
	}
	var line string
	if json.Unmarshal(payload, &line) != nil {
		return errors.New("invalid telemetry payload")
	}
	url := strings.TrimRight(w.Config.InfluxURL, "/") + "/api/v3/write_lp?db=" + urlQuery(w.Config.InfluxDatabase) + "&precision=ns"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(line))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.Config.InfluxToken)
	req.Header.Set("Content-Type", "text/plain")
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("InfluxDB status %d", resp.StatusCode)
	}
	return nil
}
func (w *Worker) processNotification(ctx context.Context) {
	if w.Config.EmailMode == "disabled" {
		return
	}
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, tenant, eventID, recipient, kind, actor, resource, outcome string
	var at time.Time
	var attempts int
	err = tx.QueryRow(ctx, `SELECT o.id,o.tenant_id,o.audit_event_id,o.recipient,e.event_type,e.actor,e.resource,e.outcome,e.occurred_at,o.attempts
 FROM notification_outbox o JOIN audit_events e ON e.id=o.audit_event_id
 WHERE o.status IN ('queued','sending') AND o.next_attempt_at<=now() ORDER BY o.next_attempt_at
 FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &tenant, &eventID, &recipient, &kind, &actor, &resource, &outcome, &at, &attempts)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, "UPDATE notification_outbox SET status='sending',attempts=attempts+1,next_attempt_at=now()+interval '2 minutes' WHERE id=$1", id)
	if err != nil || tx.Commit(ctx) != nil {
		return
	}
	event := map[string]any{"event_id": eventID, "recipient": recipient, "category": kind, "actor": actor, "resource": resource, "outcome": outcome, "occurred_at": at, "audit_url": strings.TrimRight(w.Config.PublicURL, "/") + "/audit?tenant=" + tenant}
	if w.Config.EmailMode == "mock" {
		_, _ = w.Store.DB.Exec(ctx, "UPDATE notification_outbox SET status='simulated',provider_response='mock adapter; no email sent',delivered_at=NULL WHERE id=$1", id)
		_, _ = w.Store.Audit(ctx, &tenant, "notification:mock", "notification.simulated", id, "", "simulated", "", event, false)
		return
	}
	payload, _ := json.Marshal(event)
	sendCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, "POST", w.Config.EmailProviderURL, bytes.NewReader(payload))
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+w.Config.EmailProviderToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", id)
		client := http.Client{Timeout: 8 * time.Second}
		var resp *http.Response
		resp, err = client.Do(req)
		if resp != nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				err = fmt.Errorf("provider status %d: %s", resp.StatusCode, string(raw))
			}
		}
	}
	if err == nil {
		_, _ = w.Store.DB.Exec(ctx, "UPDATE notification_outbox SET status='delivered',provider_response='accepted',delivered_at=now() WHERE id=$1", id)
		_, _ = w.Store.Audit(ctx, &tenant, "notification:http", "notification.delivered", id, "", "success", "", map[string]any{"event_id": eventID, "recipient": recipient}, false)
		return
	}
	status := "queued"
	if attempts+1 >= 8 {
		status = "dead"
	}
	delay := time.Duration(1<<min(attempts+1, 8)) * time.Second
	_, _ = w.Store.DB.Exec(ctx, "UPDATE notification_outbox SET status=$2,next_attempt_at=now()+$3::interval,provider_response=$4 WHERE id=$1", id, status, fmt.Sprintf("%d seconds", int(delay.Seconds())), truncate(err.Error(), 250))
	_, _ = w.Store.Audit(ctx, &tenant, "notification:http", "notification.failed", id, "", status, "", map[string]any{"event_id": eventID, "recipient": recipient, "attempt": attempts + 1}, false)
	if status == "dead" {
		log.Printf("customer notification %s delivery failed permanently", id)
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
