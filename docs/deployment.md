# Deployment and configuration

## Version choices

The source targets Go 1.27 (current supported release at implementation time), Next.js 16 App Router, Node.js 24, PostgreSQL 17 for the local demo, and InfluxDB 3 Core HTTP write API. Pin patch releases and review advisories in deployment lockfiles. References: [Go releases](https://go.dev/doc/devel/release), [Next.js installation](https://nextjs.org/docs/app/getting-started/installation), [Keycloak OIDC endpoints](https://www.keycloak.org/securing-apps/oidc-layers), [PostgreSQL queue locking](https://www.postgresql.org/docs/current/sql-select.html), [InfluxDB writes](https://docs.influxdata.com/influxdb3/core/api/write-data/), [Cloudflare Turnstile validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/).

## Independent services

Build and deploy `cmd/control-plane`, `cmd/worker`, and `cmd/connector` as separate binaries and workload identities. Deploy the Next.js console as a Node server. The control plane and worker use separate PostgreSQL credentials in production. The connector runs within or adjacent to one customer's network and receives only its tenant credential. A dedicated tenant may have a separate database, workers, InfluxDB bucket, secret scope, and control plane.

## Keycloak

Create separate clients for the browser and API audience. Configure the browser client for authorization code with PKCE, registered HTTPS redirect URI `/auth/callback`, valid post-logout redirect URI `/auth/login`, and the intended console hostname. Configure realm roles `platform_admin`, `ops_engineer`, `read_only_operator`, `audit_reviewer`, and `customer_viewer`; use the `memberships` table for resource scope. Enforce MFA and session policy in Keycloak. Set `OIDC_ISSUER`, `OIDC_AUDIENCE`, `OIDC_CLIENT_ID`, and canonical `PUBLIC_URL`. The API validates Keycloak JWKS and issuer on every token. Sign out clears the local cookie and redirects through Keycloak's OIDC logout endpoint.

## Secrets

Use self-hosted Infisical machine identities with narrowly scoped projects and environments. Inject `DATABASE_URL`, `INFLUX_TOKEN`, `EMAIL_PROVIDER_TOKEN`, `TURNSTILE_SECRET`, and connector credentials into service processes at startup through an Infisical agent or workload identity integration. Use distinct identities for API, worker, notification adapter, and each customer connector. Do not share one token across tenants. Rotate at the secret manager and restart or re-enroll workloads as appropriate. No production secret belongs in `.env`, source, browser storage, model prompts, or email.

## Database and queue

Apply migrations with `go run ./cmd/migrate` using a migration-only role. The jobs and outbox tables are durable. Workers claim rows with `SKIP LOCKED`, lease them, retry with backoff, and mark exhausted jobs `dead`. Monitor queue lag, dead records, and per-tenant backlog. Back up PostgreSQL and test restores. Give runtime roles only the required DML; do not allow ordinary users or web query paths to update/delete audit events.

After migrations, a database administrator runs `deploy/postgres_tenant_reader.sql`. It creates a NOLOGIN read role, grants only the columns needed by tenant detail pages, and enables row-level policies on customer tables. Provision a separate non-owner, non-`BYPASSRLS` login, grant it membership in `nocturn_tenant_reader`, and inject its connection string as `TENANT_READ_DATABASE_URL` into the control plane. The control plane refuses to start in production without that URL or if its login owns `tenants`, is a superuser, or can bypass row security. It uses `SET LOCAL ROLE` and `set_config('app.tenant_id', ..., true)` inside each read transaction; a missing tenant context returns no customer rows. Keep the migration credential and worker credential separate from this reader. Never give the reader credential to the browser or connector.

The API still uses its primary connection for writes, authorization lookups, connector authentication, and fleet listing. These paths require further database-role separation and cross-tenant testing before production onboarding. Application-set row context is defense in depth and does not replace the route authorization checks. Test the database boundary in an isolated database with `TENANT_RLS_INTEGRATION=1 go test -vet=off ./internal/platform -run TestTenantDatabaseBoundaries` after applying the read-role script.

## Metrics

The control plane saves numeric probe latency in PostgreSQL and enqueues an InfluxDB write. Configure `INFLUX_URL`, `INFLUX_DATABASE`, and `INFLUX_TOKEN` to enable InfluxDB 3 writes. Treat a missing InfluxDB as degraded metrics delivery; never widen access or authorize operations because metrics are unavailable. The current console reads tenant-scoped PostgreSQL samples; a production retention/rollup path should move long-range graph queries to InfluxDB.

## Email

`EMAIL_MODE=disabled` leaves outbox rows queued. `mock` marks them simulated. `http` posts minimal JSON to `EMAIL_PROVIDER_URL` over HTTPS with a bearer token and `Idempotency-Key`. The delivery gateway is responsible for provider-specific payload translation and should return a 2xx only when accepted. Dead records need an alert and operator review. Set `PUBLIC_URL` to the authenticated console URL used in audit links. No real email is sent in the demo.

## Edge

Cloudflare or Akamai can be placed in front of the console and API. Configure TLS, WAF, rate limits, bot and DDoS settings, and origin ACLs at that provider. Populate `TRUSTED_PROXY_CIDRS` only with validated proxy ranges; the API otherwise uses the direct peer IP. The current repository stores edge status but does not call provider APIs or provision custom hostnames. Do not mark hostnames active until DNS control, provider routing, certificate readiness, and origin mapping have all been checked. Other approved edge providers may be used with the same application authorization boundary.

## Connector enrollment

Create a customer with authorization reference and recipients. Publish the DNS challenge, verify it, issue a 15-minute enrollment token, and submit it to `/v1/connectors/enroll` with connector name, version, and capabilities. Store the returned credential in that customer's Infisical scope; it appears only once. Use a local connector config with approved probe hosts, ports, and CIDRs. The connector requires HTTPS and resolves each host, then connects only to an IP inside a configured CIDR. Review local config with the customer. Approve monitoring only after the domain and connector are ready. The demo fixture is pre-approved solely for UI and API validation.

## Release controls

Stage connector releases by tenant, compare versions and capabilities, monitor heartbeats and stale telemetry, and roll back by deploying the previous signed image. Revoke a compromised connector immediately. Backward-compatible API contracts and explicit capability negotiation are required before a production rollout. No generic public SSH listener is provided.
