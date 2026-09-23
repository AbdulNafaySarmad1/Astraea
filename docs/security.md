# Threat model and security response

## Assets and adversaries

Assets include tenant configuration, infrastructure telemetry, policy, connector credentials, audit evidence, and customer notifications. Adversaries include a malicious authenticated user, a compromised connector, a hostile telemetry/log payload, a stolen provider credential, a misconfigured edge route, and a model output attempting to trigger an operation.

| Threat | Current control | Residual risk / gate |
| --- | --- | --- |
| Cross-tenant read or action | Keycloak JWT validation, membership checks, tenant predicates, tenant-bound connector secret | Add PostgreSQL RLS, dedicated DB roles, and integration tests across every route before production |
| Domain takeover or SSRF | ASCII hostname normalization, DNS TXT challenge, no server-side URL fetch from entered domains; connector CIDR pinning | Add public-suffix validation, periodic revalidation, and approved endpoint inventory before health-check URLs or custom routing |
| Prompt injection | Telemetry is structured input; mock gateway only; no model authority or shell | Production provider adapter needs prompt isolation, budget and retention controls, redaction, and adversarial evaluation |
| Unsafe operation | Disabled policy by default, exact parameter hash, separate approver for high-risk actions, expiry, tenant and global kill switches | Production typed execution adapter, connector attestation, maintenance windows, circuit breaker, and rollback verification are absent |
| Forged browser request | HttpOnly SameSite cookie, PKCE/state, exact Origin check on mutating API proxy requests | Add robust session refresh, Keycloak back-channel logout, and tenant-specific hostname validation |
| Lost or forged audit | Hash chain, append-only app behavior, customer-visible query, outbox link | DB owner can rewrite chain. Anchor heads externally, use separate write/read roles and retention/legal-hold controls |
| Notification outage | PostgreSQL outbox, retry, idempotency key, dead state, delivery view | Configure a real provider and alert on dead messages; no production mail is configured here |
| Edge bypass | App authentication and authorization remain mandatory without edge | Origin firewall, trusted proxy list, Cloudflare/Akamai status checks, and custom hostname routing are not provisioned |
| Secret leakage | No source/browser-storage credentials; environment-based server secrets; connector credential returned once | Production must inject through Infisical scoped identities, rotate credentials, and scan logs and artifacts |

## Turnstile

The public contact endpoint is disabled unless `TURNSTILE_SECRET` is configured. The Go server calls Cloudflare Siteverify with a five-second timeout, checks `success`, challenge age, expected action, and configured hostname, then stores the request. Siteverify enforces token single use. Use Cloudflare's official test keys only in development. Do not rely on widget rendering. Add rate limiting and abuse monitoring before exposing this endpoint publicly.

## Edge and custom domains

Edge status is `unconfigured` by default. Cloudflare and Akamai are deployment options, not implicit application trust. Configure a single provider per deployment, allow only its published proxy networks, deny direct public origin access, scope provider API credentials, and retain application authorization and rate limits. Verify certificate readiness and provider hostname status before routing a console or customer domain. A verified DNS TXT record alone does not activate TLS or routing. Never route an unrecognized Host header to a customer fallback. Cloudflare for SaaS and Akamai custom hostname services have account/plan prerequisites. Provider API integrations and status reconciliation are not implemented in this foundation.

## Data handling

The connector sends component name, kind, status, TCP probe latency, timestamps, and its identity to the control plane. It sends no business records, raw SQL, SSH output, or secret. The mock gateway reads recent structured signals and reviewed source URLs, scoped to the tenant. The email adapter sends actor, event category, resource reference, outcome, time, recipient, and authenticated audit link; it sends no raw diagnostic output. PostgreSQL contains operational metadata and audit; InfluxDB receives numeric telemetry only. Set retention by customer agreement and legal hold; production retention automation is a gate.

## Incident response

1. **Provider credential exposed:** Engage the global kill switch, revoke the Infisical machine identity and provider token, rotate dependent credentials, inspect audit/outbox and provider logs, then restore after verification.
2. **Connector compromise:** Engage the tenant kill switch, revoke the connector in the onboarding view, remove its overlay network ACL, rotate its credential and local probe credentials, examine telemetry and audit, then re-enroll a new identity.
3. **Tenant isolation failure:** Engage the global kill switch, block affected API routes at the origin, preserve database and edge logs, identify affected tenants by audit correlation, notify them under incident policy, fix and retest boundary checks before reopening.
4. **Misrouted custom domain:** Disable the hostname at the edge and origin, remove its verified binding, confirm certificate and DNS state, preserve routing logs, notify affected customers, and revalidate ownership before activation.
5. **Turnstile or edge outage:** Disable only the affected public form or route. Keep API auth and policy enforcement. Do not bypass Siteverify or trust client-side success. A provider outage must not expose the origin.

Do not call this system bulletproof or label an edge integration active until a server-side provider status check confirms it.
