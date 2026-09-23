CREATE TABLE IF NOT EXISTS platform_controls (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), kill_switch boolean NOT NULL DEFAULT false,
 model_provider text NOT NULL DEFAULT 'none', model_enabled boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO platform_controls(id) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS investigations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 requested_by text NOT NULL, status text NOT NULL DEFAULT 'queued',
 summary text NOT NULL DEFAULT '', uncertainty text NOT NULL DEFAULT '',
 evidence jsonb NOT NULL DEFAULT '[]', sources jsonb NOT NULL DEFAULT '[]',
 provider text NOT NULL DEFAULT 'mock', model_metadata jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE INDEX IF NOT EXISTS investigations_tenant_time ON investigations(tenant_id,created_at DESC);
CREATE TABLE IF NOT EXISTS contact_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL, message text NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now()
);
