-- A connector retries a fixed batch ID after transport failure. The unique key
-- makes collection idempotent without mixing customer or connector identities.
CREATE TABLE connector_batches (
  tenant_id uuid NOT NULL,
  connector_id uuid NOT NULL,
  batch_id text NOT NULL CHECK (batch_id ~ '^[0-9a-f]{32}$'),
  received_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,connector_id,batch_id),
  FOREIGN KEY (tenant_id,connector_id) REFERENCES connectors(tenant_id,id)
);
CREATE INDEX connector_batches_time ON connector_batches(tenant_id,received_at DESC);
ALTER TABLE connector_batches ENABLE ROW LEVEL SECURITY;
