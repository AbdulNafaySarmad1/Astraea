# Graph Report - knowledge-graph  (2026-09-23)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 327 nodes · 832 edges · 24 communities (18 shown, 6 thin omitted)
- Extraction: 91% EXTRACTED · 9% INFERRED · 0% AMBIGUOUS · INFERRED: 78 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `0def4560`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Community 0
- Community 1
- Community 2
- Community 3
- Community 4
- Community 5
- Community 6
- Community 7
- Community 8
- Community 9
- Community 10
- Community 11
- Community 12
- Community 13
- Community 14
- Community 15
- Community 16
- Community 17
- Community 18
- Community 21
- Community 22
- Community 23

## God Nodes (most connected - your core abstractions)
1. `fail()` - 36 edges
2. `writeJSON()` - 34 edges
3. `Server` - 27 edges
4. `api()` - 18 edges
5. `actorFrom()` - 18 edges
6. `next` - 16 edges
7. `compilerOptions` - 16 edges
8. `decode()` - 12 edges
9. `PageHead()` - 11 edges
10. `DataState()` - 10 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `LoadConfig()`  [EXTRACTED]
  cmd/control-plane/main.go → internal/platform/config.go
- `main()` --calls--> `LoadConfig()`  [EXTRACTED]
  cmd/worker/main.go → internal/platform/config.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/audit-verify/main.go → internal/platform/store.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/migrate/main.go → internal/platform/store.go
- `TestProxyHeaderOnlyFromTrustedNetwork()` --calls--> `requestIP()`  [INFERRED]
  internal/platform/security_test.go → internal/platform/auth.go

## Import Cycles
- None detected.

## Communities (24 total, 6 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.09
Nodes (35): lucide-react, next, react, ref_server_only, config, Approvals(), Audit(), Onboarding() (+27 more)

### Community 1 - "Community 1"
Cohesion: 0.18
Nodes (15): net/http.Handler, net/http.Request, net/http.ResponseWriter, Server, actorFrom(), requestIP(), decode(), fail() (+7 more)

### Community 2 - "Community 2"
Cohesion: 0.06
Nodes (30): dependencies, lucide-react, next, react, react-dom, devDependencies, eslint, eslint-config-next (+22 more)

### Community 3 - "Community 3"
Cohesion: 0.12
Nodes (12): context.Context, github.com/jackc/pgx/v5/pgxpool.Pool, net/http.Client, sync.Mutex, urlQuery(), Worker, pgx.Tx, Config (+4 more)

### Community 4 - "Community 4"
Cohesion: 0.19
Nodes (16): check(), main(), send(), TestProbeRequiresApprovedCIDR(), Config, Probe, go_pkg_context, go_pkg_flag (+8 more)

### Community 5 - "Community 5"
Cohesion: 0.11
Nodes (18): compilerOptions, allowJs, esModuleInterop, incremental, isolatedModules, jsx, lib, module (+10 more)

### Community 6 - "Community 6"
Cohesion: 0.20
Nodes (12): event, go_pkg_crypto_rand, go_pkg_crypto_sha256, go_pkg_database_sql, go_pkg_encoding_hex, go_pkg_encoding_json, go_pkg_fmt, go_pkg_github_com_jackc_pgx_v5 (+4 more)

### Community 7 - "Community 7"
Cohesion: 0.18
Nodes (12): go_pkg_crypto_subtle, go_pkg_net_http, go_pkg_regexp, go_pkg_strconv, database/sql.NullTime, encoding/json.RawMessage, time.Time, normalizeHost() (+4 more)

### Community 8 - "Community 8"
Cohesion: 0.22
Nodes (10): go_pkg_errors, go_pkg_github_com_jackc_pgx_v5_pgconn, go_pkg_net, go_pkg_net_http_httptest, go_pkg_strings, go_pkg_testing, go_pkg_time, env() (+2 more)

### Community 9 - "Community 9"
Cohesion: 0.20
Nodes (10): main(), main(), main(), main(), NewOIDCVerifier(), Config, NewServer(), Open() (+2 more)

### Community 10 - "Community 10"
Cohesion: 0.36
Nodes (9): testing.T, TestActionHashBindsTenantTargetParametersAndPolicy(), TestAuditOutboxIntegration(), TestDemoModeCannotBindPublicly(), TestHostNormalizationRejectsNetworkTargets(), TestOIDCVerifierChecksSignatureAudienceAndExpiry(), TestProxyHeaderOnlyFromTrustedNetwork(), TestTenantMembershipIsolation() (+1 more)

### Community 11 - "Community 11"
Cohesion: 0.25
Nodes (8): go_pkg_crypto, go_pkg_crypto_rsa, go_pkg_encoding_base64, go_pkg_math_big, go_pkg_sync, actorKey, jwk, keySet

### Community 12 - "Community 12"
Cohesion: 0.33
Nodes (4): go_pkg_bytes, go_pkg_io, go_pkg_math, truncate()

### Community 13 - "Community 13"
Cohesion: 0.53
Nodes (4): Diagnosis, MockGateway, ModelGateway, Signal

### Community 14 - "Community 14"
Cohesion: 0.33
Nodes (5): components, $defs, operations, paths, webhooks

### Community 15 - "Community 15"
Cohesion: 0.40
Nodes (3): metadata, nav, web_src_app_styles

### Community 16 - "Community 16"
Cohesion: 0.50
Nodes (3): NOTE: This file should not be edited, web_next_types_root_params_d, web_next_types_routes_d

### Community 17 - "Community 17"
Cohesion: 0.83
Nodes (4): OnboardingControls(), addDomain(), load(), post()

## Knowledge Gaps
- **53 isolated node(s):** `$defs`, `operations`, `paths`, `webhooks`, `config` (+48 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 97 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `next` connect `Community 0` to `Community 2`, `Community 18`, `Community 15`?**
  _High betweenness centrality (0.053) - this node is a cross-community bridge._
- **Why does `Server` connect `Community 1` to `Community 9`, `Community 3`, `Community 7`?**
  _High betweenness centrality (0.035) - this node is a cross-community bridge._
- **Why does `actorFrom()` connect `Community 1` to `Community 3`, `Community 11`?**
  _High betweenness centrality (0.029) - this node is a cross-community bridge._
- **Are the 15 inferred relationships involving `fail()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`fail()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **Are the 15 inferred relationships involving `writeJSON()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`writeJSON()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **What connects `$defs`, `operations`, `paths` to the rest of the system?**
  _53 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.08951048951048951 - nodes in this community are weakly interconnected._