-- Two consecutive healthy observations close a degraded/unknown incident.
ALTER TABLE components ADD COLUMN healthy_streak integer NOT NULL DEFAULT 0 CHECK (healthy_streak >= 0);

-- Worker retries and periodic freshness checks cannot create duplicate open
-- incidents for the same component and deterministic signal.
CREATE UNIQUE INDEX incidents_one_open_signal
  ON incidents(tenant_id,component_id,title)
  WHERE status='open' AND component_id IS NOT NULL;
