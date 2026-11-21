# Contributing to Astraea

Thanks for your interest. Astraea is a secure, multi-tenant infrastructure control plane. It is under active development and has **not reached production release status** (see the [README](README.md) and [production readiness](docs/production-readiness.md)). Careful contributions are very welcome.

## License and CLA

- Astraea is licensed under the [GNU Affero General Public License v3.0](LICENSE) (`AGPL-3.0-only`).
- Before we can merge a pull request you must sign the [Contributor License Agreement](CLA.md). You keep your copyright; the CLA lets the project keep the freedom to evolve its licensing. To sign, comment `I have read the CLA Document and I hereby sign the CLA` on your first pull request.
- By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
- **Security problems:** do not open public issues. Follow [SECURITY.md](SECURITY.md).

## Ways to help

- Pick an issue labelled `good first issue` or `help wanted`.
- Add tests, especially for tenant isolation, authorization, and failure paths.
- Improve docs: deployment, operations, connector telemetry, service logs.
- Build or review provider and connector integrations.
- If you want to work on something big, **open an issue first** so we can agree on the approach before you invest time.

## Repository map

| Path | Contents |
| --- | --- |
| `cmd/control-plane`, `cmd/worker`, `cmd/connector` | Go entry points: API, durable job worker, tenant-bound connector |
| `cmd/migrate`, `cmd/audit-verify`, `cmd/log-archive` | Migrations, audit-chain verification, log archival |
| `internal` | Core Go packages |
| `db` | PostgreSQL schema and migrations |
| `contracts` | OpenAPI contract |
| `web` | Next.js management console |
| `deploy`, `compose.demo.yaml` | Deployment assets and the local demo stack |
| `docs` | Architecture, security model, operations, deployment |

## Development setup

You need the Go version in [`go.mod`](go.mod), Node.js 24, and Docker (for PostgreSQL in the demo stack).

```sh
cp .env.example .env     # then fill in local, non-production values
npm ci                   # web console dependencies
npm run dev              # console on 127.0.0.1
```

The demo database is defined in `compose.demo.yaml` and requires you to set a local password:

```sh
DEMO_DB_PASSWORD='choose-a-local-password' docker compose -f compose.demo.yaml up -d
```

Never commit real secrets, tokens, or customer data. `.env` files other than `.env.example` must stay out of git.

## Checks to run before opening a pull request

These mirror CI.

```sh
go test ./cmd/... ./internal/...
go build ./cmd/... ./internal/...
npm run api:types && git diff --exit-code -- web/src/lib/api.generated.ts
npm run lint
npm run types
npm run build
```

If you change `contracts/openapi.yaml`, regenerate the types with `npm run api:types` and commit the result.

## Pull request process

1. Fork the repository and create a branch from `main` (for example `fix/connector-reconnect`).
2. Keep each pull request focused on one change, and add or update tests.
3. Write clear commit messages in the imperative mood ("Reject cross-tenant connector token"). Explain *why*, not just *what*.
4. Fill in the pull request template, including the CLA checkbox.
5. CI must pass. A maintainer will review; please respond to feedback, and expect a few rounds. Review comments are about the code, not about you.

## Safety rules specific to this project

- **Tenant isolation is non-negotiable.** Every query and route that touches tenant data needs a tenant predicate and a test that proves cross-tenant access fails.
- **Privileged operations are controlled transactions.** Do not add paths that execute infrastructure actions outside the policy, approval, and audit flow described in the [security docs](docs/security.md).
- **No secrets in code, logs, telemetry, browsers, or AI context.**
- **Security-relevant changes** (identity, authorization, approvals, audit, connectors) need tests for the failure paths and a note in the pull request on how the threat model in `docs/security.md` is affected.

## Reporting bugs and requesting features

Use the issue templates. Include the commit you tested, steps to reproduce, and remove secrets, hostnames, and customer data from logs.
