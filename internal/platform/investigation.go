package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type Signal struct {
	Component  string    `json:"component"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	ObservedAt time.Time `json:"observed_at"`
}
type Diagnosis struct {
	Summary     string   `json:"summary"`
	Uncertainty string   `json:"uncertainty"`
	Evidence    []Signal `json:"evidence"`
	Sources     []string `json:"sources"`
}
type ModelGateway interface {
	Investigate(context.Context, []Signal, []string) (Diagnosis, error)
}
type MockGateway struct{}

func (MockGateway) Investigate(_ context.Context, signals []Signal, sources []string) (Diagnosis, error) {
	d := Diagnosis{Summary: "No degraded components in the received telemetry.", Uncertainty: "This is a simulated investigation. It cannot confirm the underlying system state.", Evidence: signals, Sources: sources}
	for _, s := range signals {
		if s.Status == "degraded" {
			d.Summary = "Approved telemetry reports a degraded " + s.Kind + " component: " + s.Component + ". Review the component and a current diagnostic snapshot."
			break
		}
	}
	if len(signals) == 0 {
		d.Summary = "No current telemetry is available for investigation."
	}
	return d, nil
}
func (s *Server) startInvestigation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "operate")
	if !ok {
		return
	}
	if s.Config.Mode != "demo" {
		fail(w, 503, "model gateway provider is not configured")
		return
	}
	var enabled bool
	err := s.Store.DB.QueryRow(r.Context(), "SELECT monitoring_approved AND NOT kill_switch AND NOT (SELECT kill_switch FROM platform_controls WHERE id=true) FROM tenants WHERE id=$1", id).Scan(&enabled)
	if err != nil || !enabled {
		fail(w, 403, "investigations disabled")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "investigation unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var investigationID string
	err = tx.QueryRow(r.Context(), "INSERT INTO investigations(tenant_id,requested_by) VALUES($1,$2) RETURNING id", id, actorFrom(r.Context()).Subject).Scan(&investigationID)
	if err != nil {
		fail(w, 503, "investigation unavailable")
		return
	}
	payload, _ := json.Marshal(map[string]string{"investigation_id": investigationID})
	_, err = tx.Exec(r.Context(), "INSERT INTO jobs(tenant_id,kind,payload,idempotency_key) VALUES($1,'investigate',$2,$3)", id, payload, investigationID)
	if err != nil {
		fail(w, 503, "queue unavailable")
		return
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 503, "investigation unavailable")
		return
	}
	s.log(r, &id, "investigation.started", investigationID, "queued", nil, true)
	writeJSON(w, 202, map[string]string{"id": investigationID, "status": "queued"})
}
func (s *Server) investigations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allow(w, r, "view")
	if !ok {
		return
	}
	read, ok := s.tenantRead(w, r, id)
	if !ok {
		return
	}
	defer read.Rollback(r.Context())
	rows, err := read.Query(r.Context(), "SELECT id,requested_by,status,summary,uncertainty,evidence,sources,provider,created_at,completed_at FROM investigations WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 50", id)
	if err != nil {
		fail(w, 503, "investigations unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var iid, by, status, summary, uncertainty, provider string
		var evidence, sources json.RawMessage
		var at time.Time
		var completed *time.Time
		if rows.Scan(&iid, &by, &status, &summary, &uncertainty, &evidence, &sources, &provider, &at, &completed) != nil {
			break
		}
		items = append(items, map[string]any{"id": iid, "requested_by": by, "status": status, "summary": summary, "uncertainty": uncertainty, "evidence": evidence, "sources": sources, "provider": provider, "created_at": at, "completed_at": completed})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (w *Worker) investigate(ctx context.Context, tenant, id string) error {
	if w.Config.Mode != "demo" {
		return errors.New("approved model gateway unavailable")
	}
	var killed bool
	err := w.Store.DB.QueryRow(ctx, "SELECT kill_switch OR (SELECT kill_switch FROM platform_controls WHERE id=true) FROM tenants WHERE id=$1", tenant).Scan(&killed)
	if err != nil || killed {
		return errors.New("investigation disabled")
	}
	rows, err := w.Store.DB.Query(ctx, "SELECT name,kind,status,observed_at FROM components WHERE tenant_id=$1 AND observed_at>now()-interval '30 minutes' ORDER BY name LIMIT 100", tenant)
	if err != nil {
		return err
	}
	signals := []Signal{}
	for rows.Next() {
		var signal Signal
		if rows.Scan(&signal.Component, &signal.Kind, &signal.Status, &signal.ObservedAt) == nil {
			signals = append(signals, signal)
		}
	}
	rows.Close()
	refs, err := w.Store.DB.Query(ctx, "SELECT source_url FROM knowledge_documents WHERE review_status='approved' AND (tenant_id IS NULL OR tenant_id=$1) ORDER BY created_at DESC LIMIT 10", tenant)
	if err != nil {
		return err
	}
	sources := []string{}
	for refs.Next() {
		var source string
		if refs.Scan(&source) == nil {
			sources = append(sources, source)
		}
	}
	refs.Close()
	diagnosis, err := (MockGateway{}).Investigate(ctx, signals, sources)
	if err != nil {
		return err
	}
	evidence, _ := json.Marshal(diagnosis.Evidence)
	links, _ := json.Marshal(diagnosis.Sources)
	metadata, _ := json.Marshal(map[string]string{"mode": "local-simulation"})
	_, err = w.Store.DB.Exec(ctx, "UPDATE investigations SET status='simulated',summary=$3,uncertainty=$4,evidence=$5,sources=$6,provider='mock',model_metadata=$7,completed_at=now() WHERE tenant_id=$1 AND id=$2 AND status='queued'", tenant, id, diagnosis.Summary, diagnosis.Uncertainty, evidence, links, metadata)
	if err != nil {
		return err
	}
	_, _ = w.Store.Audit(ctx, &tenant, "model:mock", "investigation.completed", id, "", "simulated", "", map[string]any{"evidence_count": len(signals), "source_count": len(sources)}, true)
	return nil
}
func (s *Server) platformKillSwitch(w http.ResponseWriter, r *http.Request) {
	if !actorFrom(r.Context()).has("platform_admin") {
		fail(w, 403, "platform administrator required")
		return
	}
	var b struct {
		Engaged bool `json:"engaged"`
	}
	if decode(r, &b) != nil {
		fail(w, 400, "invalid request")
		return
	}
	_, err := s.Store.DB.Exec(r.Context(), "UPDATE platform_controls SET kill_switch=$1,updated_at=now() WHERE id=true", b.Engaged)
	if err != nil {
		fail(w, 503, "control unavailable")
		return
	}
	s.log(r, nil, "platform.kill_switch.changed", "platform", "success", map[string]any{"engaged": b.Engaged}, false)
	writeJSON(w, 200, map[string]any{"engaged": b.Engaged})
}
