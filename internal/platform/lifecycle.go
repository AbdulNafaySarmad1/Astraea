package platform

import (
	"net/http"
	"regexp"
	"time"
)

func (s *Server) domains(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	rows, err := s.Store.DB.Query(r.Context(), "SELECT id,hostname,surface,status,certificate_status,verified_at,expires_at FROM domains WHERE tenant_id=$1 ORDER BY created_at DESC", id)
	if err != nil {
		fail(w, 503, "domains unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var did, host, surface, status, cert string
		var verified *time.Time
		var expires time.Time
		if rows.Scan(&did, &host, &surface, &status, &cert, &verified, &expires) != nil {
			break
		}
		items = append(items, map[string]any{"id": did, "hostname": host, "surface": surface, "status": status, "certificate_status": cert, "verified_at": verified, "expires_at": expires})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) connectors(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	rows, err := s.Store.DB.Query(r.Context(), "SELECT id,name,version,capabilities,revoked_at,last_seen_at,last_telemetry_at,created_at FROM connectors WHERE tenant_id=$1 ORDER BY created_at DESC", id)
	if err != nil {
		fail(w, 503, "connectors unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var cid, name, version string
		var capabilities []string
		var revoked, seen, telemetry *time.Time
		var at time.Time
		if rows.Scan(&cid, &name, &version, &capabilities, &revoked, &seen, &telemetry, &at) != nil {
			break
		}
		state := "disconnected"
		if revoked != nil {
			state = "revoked"
		} else if seen != nil && time.Since(*seen) < 5*time.Minute {
			state = "connected"
		}
		items = append(items, map[string]any{"id": cid, "name": name, "version": version, "capabilities": capabilities, "status": state, "last_seen_at": seen, "last_telemetry_at": telemetry, "created_at": at})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) revokeConnector(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	cid := r.PathValue("connector")
	tag, err := s.Store.DB.Exec(r.Context(), "UPDATE connectors SET revoked_at=now() WHERE id=$1 AND tenant_id=$2 AND revoked_at IS NULL", cid, id)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 404, "active connector not found")
		return
	}
	s.log(r, &id, "connector.revoked", cid, "success", nil, true)
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}
func (s *Server) approveMonitoring(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	if !actorFrom(r.Context()).has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	var ready bool
	err := s.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM domains WHERE tenant_id=$1 AND status='verified') AND EXISTS(SELECT 1 FROM connectors WHERE tenant_id=$1 AND revoked_at IS NULL) AND (SELECT cardinality(notification_recipients)>0 AND authorization_record<>'' FROM tenants WHERE id=$1)", id).Scan(&ready)
	if err != nil || !ready {
		fail(w, 409, "verified domain, connector, authorization, and recipients required")
		return
	}
	_, err = s.Store.DB.Exec(r.Context(), "UPDATE tenants SET monitoring_approved=true,status='active' WHERE id=$1 AND kill_switch=false", id)
	if err != nil {
		fail(w, 503, "approval failed")
		return
	}
	s.log(r, &id, "monitoring.approved", id, "success", nil, true)
	writeJSON(w, 200, map[string]string{"status": "active"})
}
func (s *Server) approveOperations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	if !actorFrom(r.Context()).has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	tag, err := s.Store.DB.Exec(r.Context(), "UPDATE tenants SET operations_approved=true WHERE id=$1 AND monitoring_approved=true AND kill_switch=false", id)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 409, "monitoring approval required")
		return
	}
	s.log(r, &id, "operations.approved", id, "success", nil, true)
	writeJSON(w, 200, map[string]string{"status": "approved"})
}
func (s *Server) killSwitch(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	if !actorFrom(r.Context()).has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	var body struct {
		Engaged bool `json:"engaged"`
	}
	if decode(r, &body) != nil {
		fail(w, 400, "invalid request")
		return
	}
	_, err := s.Store.DB.Exec(r.Context(), "UPDATE tenants SET kill_switch=$2 WHERE id=$1", id, body.Engaged)
	if err != nil {
		fail(w, 503, "kill switch unavailable")
		return
	}
	s.log(r, &id, "tenant.kill_switch.changed", id, "success", map[string]any{"engaged": body.Engaged}, true)
	writeJSON(w, 200, map[string]any{"engaged": body.Engaged})
}
func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	rows, err := s.Store.DB.Query(r.Context(), "SELECT action_type,version,enabled,risk,approval_required,max_per_hour,updated_at FROM policies WHERE tenant_id=$1 ORDER BY action_type", id)
	if err != nil {
		fail(w, 503, "policies unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var kind, risk string
		var version, max int
		var enabled, approval bool
		var at time.Time
		if rows.Scan(&kind, &version, &enabled, &risk, &approval, &max, &at) != nil {
			break
		}
		items = append(items, map[string]any{"action_type": kind, "version": version, "enabled": enabled, "risk": risk, "approval_required": approval, "max_per_hour": max, "updated_at": at})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) upsertPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "manage")
	if !ok {
		return
	}
	if !actorFrom(r.Context()).has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	kind := r.PathValue("actionType")
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{2,79}$`).MatchString(kind) {
		fail(w, 400, "invalid action type")
		return
	}
	var body struct {
		Enabled          bool   `json:"enabled"`
		Risk             string `json:"risk"`
		ApprovalRequired bool   `json:"approval_required"`
		MaxPerHour       int    `json:"max_per_hour"`
	}
	if decode(r, &body) != nil || body.MaxPerHour < 1 || body.MaxPerHour > 20 {
		fail(w, 400, "invalid policy")
		return
	}
	if body.Risk != "read_only" && body.Risk != "low" && body.Risk != "high" {
		fail(w, 400, "invalid risk class")
		return
	}
	if body.Risk == "high" && !body.ApprovalRequired {
		fail(w, 400, "high risk requires approval")
		return
	}
	_, err := s.Store.DB.Exec(r.Context(), "INSERT INTO policies(tenant_id,action_type,enabled,risk,approval_required,max_per_hour) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,action_type) DO UPDATE SET version=policies.version+1,enabled=EXCLUDED.enabled,risk=EXCLUDED.risk,approval_required=EXCLUDED.approval_required,max_per_hour=EXCLUDED.max_per_hour,updated_at=now()", id, kind, body.Enabled, body.Risk, body.ApprovalRequired, body.MaxPerHour)
	if err != nil {
		fail(w, 503, "policy update failed")
		return
	}
	s.log(r, &id, "policy.changed", kind, "success", body, true)
	writeJSON(w, 200, map[string]string{"status": "saved"})
}
