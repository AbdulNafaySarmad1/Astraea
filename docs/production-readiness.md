# Production readiness gates

This repository is a working development foundation, not a production deployment. The following items require implementation and verification before connecting a real university:

1. **Execution:** reviewed typed connector tools, exact target attestation, short-lived workload identity or mTLS, precondition and verification protocol, maintenance windows, circuit breakers, rollback, and no broad shell. Current worker simulations must never be interpreted as completed operations.
2. **Tenant defense:** PostgreSQL RLS and distinct roles, full cross-tenant integration suite, queue and model-context isolation checks, dedicated deployment automation, and external security review.
3. **Notifications:** configure and validate a real email delivery gateway, provider webhooks/status, dead-letter alerts, customer notification-policy controls with audited changes, and failure drills.
4. **Identity:** Keycloak realm and MFA deployment, token refresh and logout propagation, break-glass workflow, user/role administration, and session/access audit completeness.
5. **AI:** an approved provider adapter with Infisical credentials, allowlists, budgets, rate limits, timeouts, data minimization, retention controls, prompt-injection tests, source review, and incident-memory boundaries. The current gateway is mock only.
6. **Edge/custom domains:** Cloudflare and Akamai provider adapters or a selected approved provider, certificate lifecycle/status reconciliation, verified server-side hostname mapping, origin ACLs, proxy-header tests, and rollback. The current edge setting is informational and defaults to unconfigured.
7. **Observability:** OpenTelemetry traces, alert routing for stale connectors, failed notices, queue backlog, policy errors and unusual operation volume, and long-range InfluxDB query/retention strategy.
8. **Audit and continuity:** external audit hash anchoring, immutable storage/roles, retention/legal holds, automated encrypted backups, restore drills, incident exercises, and signed connector rollout.
9. **Verification:** end-to-end testing with real Keycloak, Infisical, PostgreSQL, InfluxDB, email sandbox, Turnstile test keys, edge staging account, and tenant-separated connector environments. No real customer infrastructure should be used for this validation.

The console labels a mock result `simulated`, a missing edge `unconfigured`, and telemetry older than five minutes `stale`. Keep these distinctions when replacing adapters.
