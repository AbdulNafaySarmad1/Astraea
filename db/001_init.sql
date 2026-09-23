CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, slug text NOT NULL UNIQUE,
 isolation_mode text NOT NULL DEFAULT 'shared' CHECK (isolation_mode IN ('shared','dedicated')),
 status text NOT NULL DEFAULT 'onboarding' CHECK (status IN ('onboarding','active','suspended')),
 monitoring_approved boolean NOT NULL DEFAULT false, operations_approved boolean NOT NULL DEFAULT false,
 kill_switch boolean NOT NULL DEFAULT false, notification_recipients text[] NOT NULL DEFAULT '{}',
 authorization_record text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE memberships (
 subject text NOT NULL, tenant_id uuid NOT NULL REFERENCES tenants(id), role text NOT NULL,
 PRIMARY KEY(subject,tenant_id)
);
CREATE TABLE domains (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid REFERENCES tenants(id), hostname text NOT NULL UNIQUE,
 surface text NOT NULL CHECK (surface IN ('customer','console','health_target')),
 challenge_hash text NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','provisioning','active','failed','disabled')),
 provider text, certificate_status text NOT NULL DEFAULT 'none', verified_at timestamptz,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((surface = 'console' AND tenant_id IS NULL) OR (surface <> 'console' AND tenant_id IS NOT NULL))
);
CREATE TABLE connectors (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 name text NOT NULL, version text NOT NULL, capabilities text[] NOT NULL DEFAULT '{}',
 credential_hash text NOT NULL UNIQUE, revoked_at timestamptz, last_seen_at timestamptz,
 last_telemetry_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(id,tenant_id)
);
CREATE TABLE enrollment_tokens (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 token_hash text NOT NULL UNIQUE, expires_at timestamptz NOT NULL, used_at timestamptz,
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE components (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 connector_id uuid REFERENCES connectors(id), name text NOT NULL, kind text NOT NULL,
 status text NOT NULL DEFAULT 'unknown' CHECK (status IN ('healthy','degraded','unknown')),
 observed_at timestamptz, details jsonb NOT NULL DEFAULT '{}', UNIQUE(tenant_id,name)
);
CREATE TABLE telemetry (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id),
 connector_id uuid NOT NULL REFERENCES connectors(id), component_id uuid REFERENCES components(id),
 metric text NOT NULL, value double precision NOT NULL, unit text NOT NULL,
 observed_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX telemetry_lookup ON telemetry(tenant_id,metric,observed_at DESC);
CREATE TABLE incidents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 title text NOT NULL, severity text NOT NULL, status text NOT NULL DEFAULT 'open',
 component_id uuid REFERENCES components(id), opened_at timestamptz NOT NULL DEFAULT now(),
 acknowledged_at timestamptz, resolved_at timestamptz, summary text NOT NULL DEFAULT ''
);
CREATE TABLE policies (
 tenant_id uuid NOT NULL REFERENCES tenants(id), action_type text NOT NULL, version int NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT false, risk text NOT NULL, approval_required boolean NOT NULL DEFAULT true,
 max_per_hour int NOT NULL DEFAULT 2, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,action_type)
);
CREATE TABLE action_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 target text NOT NULL, action_type text NOT NULL, purpose text NOT NULL, parameters jsonb NOT NULL DEFAULT '{}',
 impact text NOT NULL, preconditions text NOT NULL, rollback_plan text NOT NULL, verification_steps text NOT NULL,
 parameter_hash text NOT NULL, policy_version int NOT NULL, status text NOT NULL DEFAULT 'pending',
 requested_by text NOT NULL, approved_by text, approval_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE TABLE jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 kind text NOT NULL, payload jsonb NOT NULL, idempotency_key text NOT NULL,
 status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','dead')),
 attempts int NOT NULL DEFAULT 0, max_attempts int NOT NULL DEFAULT 5,
 run_at timestamptz NOT NULL DEFAULT now(), locked_until timestamptz,
 last_error text, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(tenant_id,kind,idempotency_key)
);
CREATE INDEX jobs_ready ON jobs(run_at) WHERE status='queued';
CREATE TABLE audit_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid REFERENCES tenants(id),
 actor text NOT NULL, event_type text NOT NULL, resource text NOT NULL,
 correlation_id text NOT NULL, outcome text NOT NULL, source_ip inet,
 detail jsonb NOT NULL DEFAULT '{}', occurred_at timestamptz NOT NULL DEFAULT now(),
 previous_hash text NOT NULL, event_hash text NOT NULL
);
CREATE INDEX audit_tenant_time ON audit_events(tenant_id,occurred_at DESC);
CREATE TABLE audit_chain_head (id boolean PRIMARY KEY DEFAULT true CHECK(id), event_hash text NOT NULL);
INSERT INTO audit_chain_head(id,event_hash) VALUES (true,repeat('0',64));
CREATE TABLE notification_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 audit_event_id uuid NOT NULL REFERENCES audit_events(id), recipient text NOT NULL,
 status text NOT NULL DEFAULT 'queued', attempts int NOT NULL DEFAULT 0,
 next_attempt_at timestamptz NOT NULL DEFAULT now(), provider_response text,
 delivered_at timestamptz, UNIQUE(audit_event_id,recipient)
);
CREATE TABLE knowledge_documents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid REFERENCES tenants(id),
 title text NOT NULL, source_url text NOT NULL, version text NOT NULL, review_status text NOT NULL,
 content text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE edge_settings (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), provider text NOT NULL DEFAULT 'none',
 status text NOT NULL DEFAULT 'unconfigured', origin_protected boolean NOT NULL DEFAULT false,
 last_checked_at timestamptz, detail jsonb NOT NULL DEFAULT '{}'
);
INSERT INTO edge_settings(id) VALUES(true);

REVOKE UPDATE, DELETE ON audit_events FROM PUBLIC;
REVOKE UPDATE, DELETE ON notification_outbox FROM PUBLIC;
