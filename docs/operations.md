# Operations runbooks

## Daily checks

- Check `/health/ready`, database replication and backups, worker queue lag, dead jobs, outbox dead records, connector freshness, and incident volume.
- Review edge status in the console and provider dashboard. `unconfigured` means no provider protection is confirmed by this app.
- Review audit hash verification and external chain-head anchor once deployed.

## Emergency stop

Use `POST /v1/platform/kill-switch` with an authenticated `platform_admin` token and `{"engaged":true}`. For one customer use `POST /v1/tenants/{id}/kill-switch`. These prevent new action dispatch and model investigations; revoke the connector separately to stop telemetry. Existing AegisCore scaling continues independently. For a suspected compromised worker, stop its deployment and revoke its Infisical identity too. Confirm no queued operation is executing before release.

## Connector revocation and network removal

Revoke the connector via customer onboarding or `POST /v1/tenants/{id}/connectors/{connector}/revoke`. Rotate its credential in Infisical, remove customer-specific Tailscale or equivalent ACL grants, deny its egress at the API if needed, and review the audit trail. Access control should specify tenant, connector identity, destination host, port, and purpose with deny-by-default rules.

## Notification delivery failure

Inspect `/v1/tenants/{id}/notifications` and the `notification_outbox` status. `queued` means not delivered; `simulated` means a mock adapter was used; `dead` means retries were exhausted. Verify provider credential and HTTPS endpoint, repair the adapter, and requeue only after checking the idempotency key. Inform affected customers through an approved alternate channel if immediate access notices were delayed.

## Backup and restore

Take encrypted PostgreSQL base backups and WAL archives at an interval matching customer agreements. Back up Infisical separately and use the provider's supported recovery process. To restore, isolate the deployment, restore PostgreSQL to a point in time, run migrations, verify `audit_chain_head` with `cmd/audit-verify`, reconcile outbox messages using provider idempotency keys, and compare connector identities and revocation state. Test this procedure in an isolated environment before production. The repository does not ship an automated backup scheduler.

## Audit retention and legal hold

Set retention schedules per customer agreement, preserve legal-hold records, and export with tenant-scoped authorization and an audit event. Ordinary application users cannot modify the history through the API. The current schema has no retention job or legal-hold workflow; add them before production use.

## Policy or provider failure

If the policy database or Keycloak validation is unavailable, deny privileged requests. If the model provider fails, retain telemetry and queued investigations; do not authorize an operation from a model response. If InfluxDB fails, queue bounded writes and keep current health labels based on PostgreSQL freshness. If an edge provider fails, preserve origin ACLs and app authentication; disable public forms that depend on Turnstile rather than accepting unverified submissions.
