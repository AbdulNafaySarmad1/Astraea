-- Fictional, reserved-domain demo data. Run only in an isolated local database.
INSERT INTO tenants(id,name,slug,status,authorization_record,notification_recipients,monitoring_approved,operations_approved)
VALUES
 ('11111111-1111-4111-8111-111111111111','Meridian Vale University','meridian-vale','active','Fictional demo authorization','{"recipient@example.invalid"}',true,true),
 ('22222222-2222-4222-8222-222222222222','Cedar Ridge Institute','cedar-ridge','active','Fictional demo authorization','{"recipient@example.invalid"}',true,true),
 ('33333333-3333-4333-8333-333333333333','Lumen Coast University','lumen-coast','onboarding','Fictional demo authorization','{"recipient@example.invalid"}',false,false),
 ('44444444-4444-4444-8444-444444444444','Arborfield College','arborfield','active','Fictional demo authorization','{"recipient@example.invalid"}',true,true)
ON CONFLICT(slug) DO NOTHING;
INSERT INTO domains(tenant_id,hostname,surface,challenge_hash,status,verified_at,expires_at)
SELECT id,slug||'.example','customer','demo-unusable','verified',now(),now()+interval '1 year' FROM tenants
ON CONFLICT(hostname) DO NOTHING;
INSERT INTO connectors(id,tenant_id,name,version,capabilities,credential_hash,last_seen_at,last_telemetry_at)
SELECT ('a'||substr(replace(id::text,'-',''),2,7)||'-aaaa-4aaa-8aaa-'||substr(replace(id::text,'-',''),1,12))::uuid,id,'Demo telemetry agent','0.1.0',ARRAY['tcp_health'],'demo-unusable-'||slug,now(),now() FROM tenants WHERE status='active'
ON CONFLICT(credential_hash) DO NOTHING;
INSERT INTO components(tenant_id,connector_id,name,kind,status,observed_at)
SELECT t.id,c.id,n.name,n.kind,CASE WHEN t.slug='cedar-ridge' AND n.name='PostgreSQL primary' THEN 'degraded' ELSE 'healthy' END,now()
FROM tenants t JOIN connectors c ON c.tenant_id=t.id
CROSS JOIN (VALUES('AegisCore API','application'),('PostgreSQL primary','database'),('Object storage','storage')) n(name,kind)
ON CONFLICT(tenant_id,name) DO NOTHING;
INSERT INTO telemetry(tenant_id,connector_id,metric,value,unit,observed_at)
SELECT t.id,c.id,'probe_latency_ms',(18+g.n%7+CASE WHEN t.slug='cedar-ridge' THEN 12 ELSE 0 END)::double precision,'ms',now()-g.n*interval '10 minutes'
FROM tenants t JOIN connectors c ON c.tenant_id=t.id CROSS JOIN generate_series(0,24) g(n);
INSERT INTO incidents(tenant_id,title,severity,status,summary)
SELECT id,'Database replication lag','degraded','open','Replication delay exceeded the fictional demo threshold' FROM tenants WHERE slug='cedar-ridge'
AND NOT EXISTS(SELECT 1 FROM incidents WHERE tenant_id=tenants.id AND title='Database replication lag');
INSERT INTO policies(tenant_id,action_type,enabled,risk,approval_required,max_per_hour)
SELECT id,'diagnostic_snapshot',true,'read_only',false,6 FROM tenants WHERE status='active'
ON CONFLICT DO NOTHING;
