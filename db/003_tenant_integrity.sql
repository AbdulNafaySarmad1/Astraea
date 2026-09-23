-- Reject references to another tenant even when application validation fails.
-- The additional unique keys make tenant_id part of each referenced identity.
ALTER TABLE components ADD CONSTRAINT components_tenant_identity UNIQUE (tenant_id,id);
ALTER TABLE audit_events ADD CONSTRAINT audit_events_tenant_identity UNIQUE (tenant_id,id);

ALTER TABLE components ADD CONSTRAINT components_connector_same_tenant
  FOREIGN KEY (tenant_id,connector_id) REFERENCES connectors(tenant_id,id);
ALTER TABLE telemetry ADD CONSTRAINT telemetry_connector_same_tenant
  FOREIGN KEY (tenant_id,connector_id) REFERENCES connectors(tenant_id,id);
ALTER TABLE telemetry ADD CONSTRAINT telemetry_component_same_tenant
  FOREIGN KEY (tenant_id,component_id) REFERENCES components(tenant_id,id);
ALTER TABLE incidents ADD CONSTRAINT incidents_component_same_tenant
  FOREIGN KEY (tenant_id,component_id) REFERENCES components(tenant_id,id);
ALTER TABLE notification_outbox ADD CONSTRAINT outbox_event_same_tenant
  FOREIGN KEY (tenant_id,audit_event_id) REFERENCES audit_events(tenant_id,id);
