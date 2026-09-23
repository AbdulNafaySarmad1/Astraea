package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

type componentHealth struct {
	id, name, status string
	observed         *time.Time
	healthyStreak    int
}

// evaluateStale runs independently of incoming telemetry, so a disconnected
// connector produces a visible incident instead of leaving old status healthy.
func (w *Worker) evaluateStale(ctx context.Context) {
	rows, err := w.Store.DB.Query(ctx, `SELECT DISTINCT c.tenant_id FROM components c
 JOIN tenants t ON t.id=c.tenant_id
 WHERE t.monitoring_approved AND NOT t.kill_switch
   AND (c.observed_at IS NULL OR c.observed_at<now()-interval '30 minutes')`)
	if err != nil {
		log.Printf("freshness evaluation unavailable: %v", err)
		return
	}
	tenants := []string{}
	for rows.Next() {
		var tenant string
		if rows.Scan(&tenant) == nil {
			tenants = append(tenants, tenant)
		}
	}
	rows.Close()
	for _, tenant := range tenants {
		checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := w.evaluateHealth(checkCtx, tenant); err != nil {
			log.Printf("tenant %s freshness evaluation failed: %v", tenant, err)
		}
		cancel()
	}
}

func (w *Worker) evaluateHealth(ctx context.Context, tenant string) error {
	if tenant == "" {
		return errors.New("tenant context required")
	}
	rows, err := w.Store.DB.Query(ctx, "SELECT id,name,status,observed_at,healthy_streak FROM components WHERE tenant_id=$1 ORDER BY name", tenant)
	if err != nil {
		return err
	}
	components := []componentHealth{}
	for rows.Next() {
		var c componentHealth
		if err = rows.Scan(&c.id, &c.name, &c.status, &c.observed, &c.healthyStreak); err != nil {
			rows.Close()
			return err
		}
		components = append(components, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, c := range components {
		stale := c.observed == nil || now.Sub(*c.observed) >= 30*time.Minute
		if stale {
			if err = w.openHealthIncident(ctx, tenant, c, "Telemetry stale", "warning"); err != nil {
				return err
			}
		} else if err = w.resolveHealthIncident(ctx, tenant, c, "Telemetry stale"); err != nil {
			return err
		}
		switch c.status {
		case "degraded":
			if !stale {
				if err = w.openHealthIncident(ctx, tenant, c, "Component degraded", "degraded"); err != nil {
					return err
				}
			}
			if err = w.resolveHealthIncident(ctx, tenant, c, "Component unknown"); err != nil {
				return err
			}
		case "unknown":
			if !stale {
				if err = w.openHealthIncident(ctx, tenant, c, "Component unknown", "warning"); err != nil {
					return err
				}
			}
			if err = w.resolveHealthIncident(ctx, tenant, c, "Component degraded"); err != nil {
				return err
			}
		case "healthy":
			if c.healthyStreak >= 2 && !stale {
				if err = w.resolveHealthIncident(ctx, tenant, c, "Component degraded"); err != nil {
					return err
				}
				if err = w.resolveHealthIncident(ctx, tenant, c, "Component unknown"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Worker) openHealthIncident(ctx context.Context, tenant string, c componentHealth, kind, severity string) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	title := kind + ": " + c.name
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO incidents(tenant_id,title,severity,status,component_id,summary)
 VALUES($1,$2,$3,'open',$4,$5) ON CONFLICT DO NOTHING RETURNING id`, tenant, title, severity, c.id, "Detected from approved infrastructure telemetry").Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = w.Store.AuditTx(ctx, tx, &tenant, "worker:health", "incident.opened", id, "", "success", "", map[string]any{"component": c.name, "signal": kind}, false)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) resolveHealthIncident(ctx context.Context, tenant string, c componentHealth, kind string) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `UPDATE incidents SET status='resolved',resolved_at=now()
 WHERE tenant_id=$1 AND component_id=$2 AND title=$3 AND status='open' RETURNING id`, tenant, c.id, fmt.Sprintf("%s: %s", kind, c.name)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = w.Store.AuditTx(ctx, tx, &tenant, "worker:health", "incident.resolved", id, "", "success", "", map[string]any{"component": c.name, "signal": kind}, false)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
