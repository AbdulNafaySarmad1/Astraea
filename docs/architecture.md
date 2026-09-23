# Architecture and data flows

## Trust boundaries

1. **Browser to console:** OIDC authorization code with PKCE. Access tokens remain in an HttpOnly, SameSite cookie and are proxied server-side to the API. Mutating proxy requests require a matching Origin. The Keycloak realm supplies SSO and MFA policy.
2. **Console to API:** Bearer JWT validation uses the realm discovery document, JWKS, issuer, audience, signature, and expiry. The API enforces role and membership per route. A client-provided tenant path never grants membership.
3. **API to PostgreSQL:** Customer-owned tables carry `tenant_id`; queries include it. The database stores tenant registry, enrollment state, action policy, approval, durable jobs, audit metadata, and notification delivery. Separate database roles and row-level security are required for production hardening.
4. **Connector to API:** A connector enrolls with a one-time, 15-minute token; the server returns a unique credential once. The connector initiates HTTPS heartbeats. It is tied to one tenant, capabilities, version, and revocation state. Deploy one connector per customer boundary. Its local probe config controls which IP networks it can reach.
5. **Worker to external services:** Jobs are claimed with PostgreSQL `FOR UPDATE SKIP LOCKED`, leased, retried with bounded backoff, and dead-lettered. Per-tenant running-job exclusion bounds concurrency. InfluxDB receives sanitized numeric latency metrics. The email adapter receives a minimal event record. The mock model gateway only receives recent structured component signals and reviewed source links.

## Onboarding sequence

Customer record with authorization reference and recipients → DNS TXT challenge → domain verification → short-lived enrollment token → tenant-bound connector enrollment → administrator approval for monitoring → separate approval and disabled-by-default action policies for operations. A domain alone never allows probes.

## Health semantics

Component status is `healthy`, `degraded`, or `unknown` only while its observation is younger than five minutes. Older observations render as `stale`. A connector is `connected` only when it has reported within five minutes and is not revoked. No signal is interpreted as healthy. AegisCore scaling remains independent.

## Operations

An action request contains tenant, target, action type, purpose, parameters, impact, preconditions, rollback, and verification. The API checks target inventory, customer enablement, policy version, exact parameter hash, approval expiry, recent connector presence, and action volume before queueing. The worker rechecks policy before its demo simulation. There is no production command executor; a reviewed typed-tool protocol and connector attestation must be added before production operations.

## Audit and notifications

Each audit event has a stable ID, UTC timestamp, actor, tenant, resource, correlation ID, outcome, optional source IP, previous hash, and event hash. Events that require customer notice insert outbox rows transactionally with the audit event. Delivery uses retries and an idempotency key. Audit access itself is audited. External hash anchoring and append-only database roles remain deployment tasks.

## Isolation modes

`shared` uses a control plane with server-side tenant scoping. `dedicated` is a deployment flag for a separate stack, database, workers, secrets scope, and connector network. The repository does not automatically provision dedicated infrastructure.
