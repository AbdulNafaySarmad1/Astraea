package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type ActionInput struct {
	Target            string         `json:"target"`
	ActionType        string         `json:"action_type"`
	Purpose           string         `json:"purpose"`
	Parameters        map[string]any `json:"parameters"`
	Impact            string         `json:"impact"`
	Preconditions     string         `json:"preconditions"`
	RollbackPlan      string         `json:"rollback_plan"`
	VerificationSteps string         `json:"verification_steps"`
}

func validAction(b ActionInput) bool {
	return len(b.Target) > 0 && len(b.Target) <= 120 && len(b.ActionType) > 0 && len(b.ActionType) <= 80 && len(b.Purpose) >= 8 && len(b.Purpose) <= 500 && len(b.Impact) > 0 && len(b.Preconditions) > 0 && len(b.RollbackPlan) > 0 && len(b.VerificationSteps) > 0
}
func actionHash(tenant string, b ActionInput, version int) string {
	raw, _ := json.Marshal([]any{tenant, b.Target, b.ActionType, b.Parameters, version})
	return digest(string(raw))
}
func (s *Server) proposeAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "operate")
	if !ok {
		return
	}
	var b ActionInput
	if decode(r, &b) != nil || !validAction(b) {
		fail(w, 400, "typed action fields required")
		return
	}
	var enabled, approval, ops, kill bool
	var version int
	var risk string
	err := s.Store.DB.QueryRow(r.Context(), "SELECT p.enabled,p.approval_required,p.version,p.risk,t.operations_approved,t.kill_switch OR (SELECT kill_switch FROM platform_controls WHERE id=true) FROM policies p JOIN tenants t ON t.id=p.tenant_id WHERE p.tenant_id=$1 AND p.action_type=$2", id, b.ActionType).Scan(&enabled, &approval, &version, &risk, &ops, &kill)
	if err != nil || !enabled || !ops || kill {
		fail(w, 403, "action disabled by policy")
		return
	}
	var targetExists bool
	_ = s.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM components WHERE tenant_id=$1 AND name=$2)", id, b.Target).Scan(&targetExists)
	if !targetExists {
		fail(w, 403, "target not in tenant inventory")
		return
	}
	params, _ := json.Marshal(b.Parameters)
	hash := actionHash(id, b, version)
	var actionID string
	err = s.Store.DB.QueryRow(r.Context(), "INSERT INTO action_requests(tenant_id,target,action_type,purpose,parameters,impact,preconditions,rollback_plan,verification_steps,parameter_hash,policy_version,requested_by,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending') RETURNING id", id, b.Target, b.ActionType, b.Purpose, params, b.Impact, b.Preconditions, b.RollbackPlan, b.VerificationSteps, hash, version, actorFrom(r.Context()).Subject).Scan(&actionID)
	if err != nil {
		fail(w, 503, "action request unavailable")
		return
	}
	s.log(r, &id, "action.proposed", actionID, "pending", map[string]any{"target": b.Target, "action_type": b.ActionType, "risk": risk, "approval_required": approval}, true)
	writeJSON(w, 201, map[string]any{"id": actionID, "status": "pending", "approval_required": approval})
}
func (s *Server) approveAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "approve")
	if !ok {
		return
	}
	actionID := r.PathValue("action")
	var requestedBy, status string
	err := s.Store.DB.QueryRow(r.Context(), "SELECT requested_by,status FROM action_requests WHERE id=$1 AND tenant_id=$2", actionID, id).Scan(&requestedBy, &status)
	if err != nil {
		fail(w, 404, "action not found")
		return
	}
	a := actorFrom(r.Context())
	if status != "pending" || requestedBy == a.Subject {
		fail(w, 409, "approval requires a different operator and pending request")
		return
	}
	tag, err := s.Store.DB.Exec(r.Context(), "UPDATE action_requests SET status='approved',approved_by=$3,approval_expires_at=now()+interval '30 minutes' WHERE id=$1 AND tenant_id=$2 AND status='pending'", actionID, id, a.Subject)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 409, "approval failed")
		return
	}
	s.log(r, &id, "action.approved", actionID, "success", nil, true)
	writeJSON(w, 200, map[string]string{"status": "approved", "expires_in": "30m"})
}
func (s *Server) executeAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "operate")
	if !ok {
		return
	}
	actionID := r.PathValue("action")
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "policy unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var target, kind, status, hash string
	var params json.RawMessage
	var policyVersion int
	var approvedBy *string
	var approvalExpiry *time.Time
	err = tx.QueryRow(r.Context(), "SELECT target,action_type,status,parameters,parameter_hash,policy_version,approved_by,approval_expires_at FROM action_requests WHERE id=$1 AND tenant_id=$2 FOR UPDATE", actionID, id).Scan(&target, &kind, &status, &params, &hash, &policyVersion, &approvedBy, &approvalExpiry)
	if err != nil {
		fail(w, 404, "action not found")
		return
	}
	var enabled, approval, ops, kill bool
	var currentVersion, maxPerHour int
	err = tx.QueryRow(r.Context(), "SELECT p.enabled,p.approval_required,p.version,p.max_per_hour,t.operations_approved,t.kill_switch OR (SELECT kill_switch FROM platform_controls WHERE id=true) FROM policies p JOIN tenants t ON t.id=p.tenant_id WHERE p.tenant_id=$1 AND p.action_type=$2", id, kind).Scan(&enabled, &approval, &currentVersion, &maxPerHour, &ops, &kill)
	if err != nil || !enabled || !ops || kill || currentVersion != policyVersion {
		fail(w, 403, "policy changed or operations disabled")
		return
	}
	var parsed map[string]any
	_ = json.Unmarshal(params, &parsed)
	if hash != actionHash(id, ActionInput{Target: target, ActionType: kind, Parameters: parsed}, currentVersion) {
		fail(w, 403, "action parameters changed")
		return
	}
	if approval && (status != "approved" || approvedBy == nil || approvalExpiry == nil || time.Now().After(*approvalExpiry)) {
		fail(w, 403, "valid exact approval required")
		return
	}
	if !approval && status != "pending" && status != "approved" {
		fail(w, 409, "action already dispatched")
		return
	}
	var count int
	_ = tx.QueryRow(r.Context(), "SELECT count(*) FROM action_requests WHERE tenant_id=$1 AND action_type=$2 AND status IN ('queued','running','completed') AND created_at>now()-interval '1 hour'", id, kind).Scan(&count)
	if count >= maxPerHour {
		fail(w, 429, "tenant action limit reached")
		return
	}
	var connected bool
	_ = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM components c JOIN connectors x ON x.id=c.connector_id WHERE c.tenant_id=$1 AND c.name=$2 AND x.revoked_at IS NULL AND x.last_seen_at>now()-interval '5 minutes')", id, target).Scan(&connected)
	if !connected {
		fail(w, 409, "connector unavailable or telemetry stale")
		return
	}
	payload, _ := json.Marshal(map[string]string{"action_id": actionID})
	_, err = tx.Exec(r.Context(), "INSERT INTO jobs(tenant_id,kind,payload,idempotency_key) VALUES($1,'execute_action',$2,$3) ON CONFLICT DO NOTHING", id, payload, actionID)
	if err != nil {
		fail(w, 503, "queue unavailable")
		return
	}
	tag, err := tx.Exec(r.Context(), "UPDATE action_requests SET status='queued' WHERE id=$1 AND tenant_id=$2 AND status=$3", actionID, id, status)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 409, "action state changed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 503, "dispatch failed")
		return
	}
	s.log(r, &id, "action.dispatched", actionID, "queued", map[string]string{"target": target, "action_type": kind}, true)
	writeJSON(w, 202, map[string]string{"status": "queued"})
}
func (s *Server) actions(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	rows, err := s.Store.DB.Query(r.Context(), "SELECT id,target,action_type,purpose,impact,status,requested_by,approved_by,created_at,approval_expires_at FROM action_requests WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 100", id)
	if err != nil {
		fail(w, 503, "actions unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var aid, target, kind, purpose, impact, status, requested string
		var approved *string
		var at time.Time
		var expires *time.Time
		if rows.Scan(&aid, &target, &kind, &purpose, &impact, &status, &requested, &approved, &at, &expires) != nil {
			break
		}
		items = append(items, map[string]any{"id": aid, "target": target, "action_type": kind, "purpose": purpose, "impact": impact, "status": status, "requested_by": requested, "approved_by": approved, "created_at": at, "approval_expires_at": expires})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) policyDecision(ctx context.Context, tenant, actionID string) error {
	var status string
	err := s.Store.DB.QueryRow(ctx, "SELECT status FROM action_requests WHERE tenant_id=$1 AND id=$2", tenant, actionID).Scan(&status)
	if err == pgx.ErrNoRows {
		return errDenied
	}
	if err != nil {
		return err
	}
	if status != "queued" {
		return fmt.Errorf("action state %s", status)
	}
	return nil
}
