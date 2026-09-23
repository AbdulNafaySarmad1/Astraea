-- Log payloads live in an encrypted file store. PostgreSQL holds only tenant
-- binding, approval, retention, integrity, and archive state.
ALTER TABLE enrollment_tokens
  ADD COLUMN approved_capabilities text[] NOT NULL DEFAULT ARRAY['tcp_health']::text[],
  ADD COLUMN approved_log_sources jsonb NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE connector_log_sources (
  tenant_id uuid NOT NULL,
  connector_id uuid NOT NULL,
  name text NOT NULL CHECK (name ~ '^[a-z][a-z0-9_-]{0,63}$'),
  kind text NOT NULL CHECK (kind IN ('postgres','valkey','vault','aegiscore')),
  path_sha256 text NOT NULL CHECK (path_sha256 ~ '^[0-9a-f]{64}$'),
  approved_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,connector_id,name),
  CONSTRAINT connector_log_sources_path_scope_unique UNIQUE (tenant_id,connector_id,name,kind,path_sha256),
  FOREIGN KEY (tenant_id,connector_id) REFERENCES connectors(tenant_id,id)
);

CREATE TABLE log_batches (
  tenant_id uuid NOT NULL,
  connector_id uuid NOT NULL,
  batch_id text NOT NULL CHECK (batch_id ~ '^[0-9a-f]{32}$'),
  source_name text NOT NULL,
  source_kind text NOT NULL,
  path_sha256 text NOT NULL CHECK (path_sha256 ~ '^[0-9a-f]{64}$'),
  collected_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  plaintext_sha256 text NOT NULL CHECK (plaintext_sha256 ~ '^[0-9a-f]{64}$'),
  line_count integer NOT NULL CHECK (line_count > 0 AND line_count <= 1024),
  status text NOT NULL DEFAULT 'hot' CHECK (status IN ('hot','archived','blocked')),
  archive_attempts integer NOT NULL DEFAULT 0 CHECK (archive_attempts >= 0),
  archive_retry_at timestamptz NOT NULL DEFAULT now(),
  archive_last_error text,
  archive_sha256 text,
  archived_at timestamptz,
  hot_deleted_at timestamptz,
  cleanup_attempts integer NOT NULL DEFAULT 0 CHECK (cleanup_attempts >= 0),
  cleanup_retry_at timestamptz NOT NULL DEFAULT now(),
  cleanup_last_error text,
  legal_hold boolean NOT NULL DEFAULT false,
  PRIMARY KEY (tenant_id,connector_id,batch_id),
  CONSTRAINT log_batches_path_scope_fk FOREIGN KEY (tenant_id,connector_id,source_name,source_kind,path_sha256)
    REFERENCES connector_log_sources(tenant_id,connector_id,name,kind,path_sha256)
);
CREATE INDEX log_batches_archive_due ON log_batches(received_at)
  WHERE status='hot';
CREATE INDEX log_batches_tenant_time ON log_batches(tenant_id,received_at DESC);
ALTER TABLE connector_log_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE log_batches ENABLE ROW LEVEL SECURITY;
