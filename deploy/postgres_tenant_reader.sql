-- Run as a database administrator after migrations. Grant this NOLOGIN role
-- only to a dedicated, non-owner read connection identity. Never grant it to
-- the migration owner or a browser-facing user.
BEGIN;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nocturn_tenant_reader') THEN
    CREATE ROLE nocturn_tenant_reader NOLOGIN;
  END IF;
END $$;

GRANT USAGE ON SCHEMA public TO nocturn_tenant_reader;
REVOKE ALL PRIVILEGES ON tenants,memberships,domains,connectors,enrollment_tokens,
  components,telemetry,incidents,policies,action_requests,jobs,audit_events,
  notification_outbox,knowledge_documents,investigations,connector_batches,
  connector_log_sources,log_batches FROM nocturn_tenant_reader;
GRANT SELECT(id,name,slug,status,isolation_mode,monitoring_approved,
  operations_approved,kill_switch,notification_recipients,created_at)
  ON tenants TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,hostname,surface,status,certificate_status,
  verified_at,expires_at,created_at) ON domains TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,name,version,capabilities,revoked_at,last_seen_at,
  last_telemetry_at,created_at) ON connectors TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,connector_id,name,kind,status,observed_at)
  ON components TO nocturn_tenant_reader;
GRANT SELECT(tenant_id,metric,value,unit,observed_at)
  ON telemetry TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,title,severity,status,component_id,opened_at,
  acknowledged_at,resolved_at,summary) ON incidents TO nocturn_tenant_reader;
GRANT SELECT(tenant_id,action_type,version,enabled,risk,approval_required,
  max_per_hour,updated_at) ON policies TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,target,action_type,purpose,impact,status,
  requested_by,approved_by,created_at,approval_expires_at)
  ON action_requests TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,actor,event_type,resource,correlation_id,outcome,
  occurred_at,detail) ON audit_events TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,audit_event_id,recipient,status,attempts,delivered_at)
  ON notification_outbox TO nocturn_tenant_reader;
GRANT SELECT(id,tenant_id,requested_by,status,summary,uncertainty,evidence,
  sources,provider,created_at,completed_at)
  ON investigations TO nocturn_tenant_reader;
GRANT SELECT(tenant_id,connector_id,batch_id,source_name,source_kind,collected_at,
  received_at,line_count,status,archive_attempts,archive_retry_at,cleanup_attempts,cleanup_retry_at,
  archived_at,hot_deleted_at,legal_hold)
  ON log_batches TO nocturn_tenant_reader;

-- No tenant context means zero customer rows. Malformed UUIDs fail the query.
-- Only reviewed shared knowledge is visible outside a tenant record.
DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'tenants','memberships','domains','connectors','enrollment_tokens',
    'components','telemetry','incidents','policies','action_requests','jobs',
    'audit_events','notification_outbox','knowledge_documents','investigations',
    'connector_batches','connector_log_sources','log_batches'
  ] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',table_name);
    EXECUTE format('DROP POLICY IF EXISTS tenant_reader_scope ON %I',table_name);
    IF table_name = 'tenants' THEN
      EXECUTE format('CREATE POLICY tenant_reader_scope ON %I FOR SELECT TO nocturn_tenant_reader USING (id = NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid)',table_name);
    ELSIF table_name = 'knowledge_documents' THEN
      EXECUTE format('CREATE POLICY tenant_reader_scope ON %I FOR SELECT TO nocturn_tenant_reader USING (tenant_id = NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid OR (tenant_id IS NULL AND review_status = ''approved''))',table_name);
    ELSE
      EXECUTE format('CREATE POLICY tenant_reader_scope ON %I FOR SELECT TO nocturn_tenant_reader USING (tenant_id = NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid)',table_name);
    END IF;
  END LOOP;
END $$;
COMMIT;
