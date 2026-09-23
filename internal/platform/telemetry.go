package platform

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type incomingMetric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type incomingComponent struct {
	Name       string           `json:"name"`
	Kind       string           `json:"kind"`
	Status     string           `json:"status"`
	ObservedAt time.Time        `json:"observed_at"`
	LatencyMS  *float64         `json:"latency_ms"`
	Metrics    []incomingMetric `json:"metrics"`
}

type incomingHeartbeat struct {
	TenantID    string              `json:"tenant_id"`
	ConnectorID string              `json:"connector_id"`
	BatchID     string              `json:"batch_id"`
	Version     string              `json:"version"`
	Components  []incomingComponent `json:"components"`
}

var metricUnits = map[string]string{
	"probe_latency_ms":              "ms",
	"host_cpu_usage_pct":            "percent",
	"host_memory_used_pct":          "percent",
	"host_root_disk_used_pct":       "percent",
	"host_network_rx_bytes_total":   "bytes",
	"host_network_tx_bytes_total":   "bytes",
	"postgres_connections":          "connections",
	"postgres_rollbacks_total":      "transactions",
	"postgres_replica_count":        "replicas",
	"postgres_replay_backlog_bytes": "bytes",
	"valkey_used_memory_bytes":      "bytes",
	"valkey_connected_clients":      "count",
	"valkey_evicted_keys_total":     "count",
}

var componentCapabilities = map[string]string{
	"tcp":      "tcp_health",
	"postgres": "postgres_health",
	"valkey":   "valkey_health",
	"host":     "host_metrics",
}

func (b incomingHeartbeat) validate(capabilities []string, now time.Time) bool {
	if b.Version == "" || len(b.Version) > 80 || len(b.Components) > 100 {
		return false
	}
	if len(b.Components) > 0 {
		if len(b.BatchID) != 32 {
			return false
		}
		if _, err := hex.DecodeString(b.BatchID); err != nil || b.BatchID != strings.ToLower(b.BatchID) {
			return false
		}
	}
	for _, component := range b.Components {
		capability, known := componentCapabilities[component.Kind]
		if !known || !slices.Contains(capabilities, capability) || component.Name == "" || len(component.Name) > 120 || strings.ContainsAny(component.Name, "\r\n") || len(component.Metrics) > 16 {
			return false
		}
		if component.Status != "healthy" && component.Status != "degraded" && component.Status != "unknown" {
			return false
		}
		if component.ObservedAt.IsZero() || component.ObservedAt.After(now.Add(time.Minute)) || component.ObservedAt.Before(now.Add(-8*24*time.Hour)) {
			return false
		}
		if component.LatencyMS != nil && (math.IsNaN(*component.LatencyMS) || math.IsInf(*component.LatencyMS, 0) || *component.LatencyMS < 0 || *component.LatencyMS > 30000) {
			return false
		}
		seen := make(map[string]bool, len(component.Metrics))
		for _, metric := range component.Metrics {
			unit, allowed := metricUnits[metric.Name]
			if !allowed || metric.Name == "probe_latency_ms" || metric.Unit != unit || seen[metric.Name] || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) || metric.Value < 0 || metric.Value > 1e18 {
				return false
			}
			if unit == "percent" && metric.Value > 100 {
				return false
			}
			seen[metric.Name] = true
		}
	}
	return true
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") || len(authorization) != len("Bearer ")+64 {
		fail(w, http.StatusUnauthorized, "connector credential required")
		return
	}
	var body incomingHeartbeat
	if decode(r, &body) != nil {
		fail(w, http.StatusBadRequest, "invalid telemetry")
		return
	}
	tx, err := s.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var tenant, connector string
	var capabilities []string
	var approved, kill bool
	err = tx.QueryRow(r.Context(), "SELECT c.tenant_id,c.id,c.capabilities,t.monitoring_approved,t.kill_switch FROM connectors c JOIN tenants t ON t.id=c.tenant_id WHERE c.credential_hash=$1 AND c.revoked_at IS NULL FOR UPDATE OF c,t", digest(strings.TrimPrefix(authorization, "Bearer "))).Scan(&tenant, &connector, &capabilities, &approved, &kill)
	if err != nil {
		fail(w, http.StatusUnauthorized, "connector revoked or unknown")
		return
	}
	if kill || !approved {
		fail(w, http.StatusForbidden, "monitoring disabled")
		return
	}
	if body.TenantID != tenant || body.ConnectorID != connector || !body.validate(capabilities, time.Now().UTC()) {
		fail(w, http.StatusBadRequest, "telemetry outside connector capability or limits")
		return
	}
	if len(body.Components) > 0 {
		var inserted string
		err = tx.QueryRow(r.Context(), "INSERT INTO connector_batches(tenant_id,connector_id,batch_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING batch_id", tenant, connector, body.BatchID).Scan(&inserted)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
			return
		}
		if inserted == "" {
			if _, err = tx.Exec(r.Context(), "UPDATE connectors SET last_seen_at=now() WHERE id=$1 AND tenant_id=$2", connector, tenant); err != nil || tx.Commit(r.Context()) != nil {
				fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
			return
		}
	}
	if len(body.Components) == 0 {
		if _, err = tx.Exec(r.Context(), "UPDATE connectors SET last_seen_at=now(),version=$3 WHERE id=$1 AND tenant_id=$2", connector, tenant, body.Version); err != nil || tx.Commit(r.Context()) != nil {
			fail(w, http.StatusServiceUnavailable, "heartbeat unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
		return
	}
	var latest time.Time
	metricCount := 0
	for _, component := range body.Components {
		if component.ObservedAt.After(latest) {
			latest = component.ObservedAt
		}
		var componentID string
		err = tx.QueryRow(r.Context(), `INSERT INTO components(tenant_id,connector_id,name,kind,status,observed_at,healthy_streak)
 VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $5='healthy' THEN 1 ELSE 0 END) ON CONFLICT(tenant_id,name) DO UPDATE SET
 status=CASE WHEN components.observed_at IS NULL OR EXCLUDED.observed_at>=components.observed_at THEN EXCLUDED.status ELSE components.status END,
 observed_at=CASE WHEN components.observed_at IS NULL OR EXCLUDED.observed_at>=components.observed_at THEN EXCLUDED.observed_at ELSE components.observed_at END,
 connector_id=CASE WHEN components.observed_at IS NULL OR EXCLUDED.observed_at>=components.observed_at THEN EXCLUDED.connector_id ELSE components.connector_id END,
 kind=CASE WHEN components.observed_at IS NULL OR EXCLUDED.observed_at>=components.observed_at THEN EXCLUDED.kind ELSE components.kind END,
 healthy_streak=CASE WHEN components.observed_at IS NULL OR EXCLUDED.observed_at>components.observed_at THEN
   CASE WHEN EXCLUDED.status='healthy' AND components.status='healthy' THEN components.healthy_streak+1
        WHEN EXCLUDED.status='healthy' THEN 1 ELSE 0 END
   ELSE components.healthy_streak END
 RETURNING id`, tenant, connector, component.Name, component.Kind, component.Status, component.ObservedAt).Scan(&componentID)
		if err != nil {
			fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
			return
		}
		metrics := component.Metrics
		if component.LatencyMS != nil {
			metrics = append(metrics, incomingMetric{Name: "probe_latency_ms", Value: *component.LatencyMS, Unit: "ms"})
		}
		for _, metric := range metrics {
			_, err = tx.Exec(r.Context(), "INSERT INTO telemetry(tenant_id,connector_id,component_id,metric,value,unit,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7)", tenant, connector, componentID, metric.Name, metric.Value, metric.Unit, component.ObservedAt)
			if err != nil {
				fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
				return
			}
			metricCount++
			if s.Config.InfluxURL != "" {
				line := fmt.Sprintf("infrastructure_metric,tenant=%s,connector=%s,component=%s,metric=%s value=%g %d", tenant, connector, componentID, metric.Name, metric.Value, component.ObservedAt.UnixNano())
				payload, _ := json.Marshal(line)
				_, err = tx.Exec(r.Context(), "INSERT INTO jobs(tenant_id,kind,payload,idempotency_key) VALUES($1,'influx_write',$2,$3) ON CONFLICT DO NOTHING", tenant, payload, body.BatchID+":"+componentID+":"+metric.Name)
				if err != nil {
					fail(w, http.StatusServiceUnavailable, "telemetry queue unavailable")
					return
				}
			}
		}
	}
	_, err = tx.Exec(r.Context(), "UPDATE connectors SET last_seen_at=now(),last_telemetry_at=greatest(coalesce(last_telemetry_at,$3),$3),version=$4 WHERE id=$1 AND tenant_id=$2", connector, tenant, latest, body.Version)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "telemetry unavailable")
		return
	}
	jobPayload, _ := json.Marshal(map[string]string{"source_batch": body.BatchID})
	if _, err = tx.Exec(r.Context(), "INSERT INTO jobs(tenant_id,kind,payload,idempotency_key) VALUES($1,'evaluate_health',$2,$3) ON CONFLICT DO NOTHING", tenant, jobPayload, body.BatchID); err != nil {
		fail(w, http.StatusServiceUnavailable, "health evaluation queue unavailable")
		return
	}
	_, err = s.Store.AuditTx(r.Context(), tx, &tenant, "connector:"+connector, "telemetry.collected", connector, body.BatchID, "success", requestIP(r, s.Config.ProxyCIDRs), map[string]any{"component_count": len(body.Components), "metric_count": metricCount}, false)
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, http.StatusServiceUnavailable, "telemetry audit unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}
