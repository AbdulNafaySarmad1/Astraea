-- Forward upgrade for early development databases that applied 006 before
-- path-bound approvals and bounded archive retries were added. A fresh 006
-- already has these fields; each statement remains safe there.
ALTER TABLE connector_log_sources ADD COLUMN IF NOT EXISTS path_sha256 text;
UPDATE connector_log_sources SET path_sha256=repeat('0',64) WHERE path_sha256 IS NULL;
ALTER TABLE connector_log_sources ALTER COLUMN path_sha256 SET NOT NULL;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='connector_log_sources_path_scope_unique') THEN
    ALTER TABLE connector_log_sources ADD CONSTRAINT connector_log_sources_path_scope_unique
      UNIQUE (tenant_id,connector_id,name,kind,path_sha256);
  END IF;
END $$;

ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS path_sha256 text;
UPDATE log_batches SET path_sha256=repeat('0',64) WHERE path_sha256 IS NULL;
ALTER TABLE log_batches ALTER COLUMN path_sha256 SET NOT NULL;
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS archive_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS archive_retry_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS archive_last_error text;
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS cleanup_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS cleanup_retry_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE log_batches ADD COLUMN IF NOT EXISTS cleanup_last_error text;

ALTER TABLE connector_log_sources DROP CONSTRAINT IF EXISTS connector_log_sources_path_sha256_check;
ALTER TABLE connector_log_sources ADD CONSTRAINT connector_log_sources_path_sha256_check
  CHECK (path_sha256 ~ '^[0-9a-f]{64}$');
ALTER TABLE log_batches DROP CONSTRAINT IF EXISTS log_batches_path_sha256_check;
ALTER TABLE log_batches ADD CONSTRAINT log_batches_path_sha256_check
  CHECK (path_sha256 ~ '^[0-9a-f]{64}$');
ALTER TABLE log_batches DROP CONSTRAINT IF EXISTS log_batches_archive_attempts_check;
ALTER TABLE log_batches ADD CONSTRAINT log_batches_archive_attempts_check CHECK (archive_attempts >= 0);
ALTER TABLE log_batches DROP CONSTRAINT IF EXISTS log_batches_cleanup_attempts_check;
ALTER TABLE log_batches ADD CONSTRAINT log_batches_cleanup_attempts_check CHECK (cleanup_attempts >= 0);

ALTER TABLE log_batches DROP CONSTRAINT IF EXISTS log_batches_status_check;
ALTER TABLE log_batches ADD CONSTRAINT log_batches_status_check
  CHECK (status IN ('hot','archived','blocked'));

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='log_batches_path_scope_fk') THEN
    ALTER TABLE log_batches ADD CONSTRAINT log_batches_path_scope_fk
      FOREIGN KEY (tenant_id,connector_id,source_name,source_kind,path_sha256)
      REFERENCES connector_log_sources(tenant_id,connector_id,name,kind,path_sha256);
  END IF;
END $$;
