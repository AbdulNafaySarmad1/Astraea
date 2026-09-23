# Graph Report - knowledge-graph  (2026-09-23)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 385 nodes · 1015 edges · 23 communities (17 shown, 6 thin omitted)
- Extraction: 91% EXTRACTED · 9% INFERRED · 0% AMBIGUOUS · INFERRED: 91 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `e5b84d6c`
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
- Community 20
- Community 21
- Community 22

## God Nodes (most connected - your core abstractions)
1. `fail()` - 36 edges
2. `writeJSON()` - 34 edges
3. `Server` - 26 edges
4. `api()` - 18 edges
5. `actorFrom()` - 18 edges
6. `next` - 16 edges
7. `compilerOptions` - 16 edges
8. `decode()` - 12 edges
9. `PageHead()` - 11 edges
10. `Store` - 11 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/migrate/main.go → internal/platform/store.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/audit-verify/main.go → internal/platform/store.go
- `TestProxyHeaderOnlyFromTrustedNetwork()` --calls--> `requestIP()`  [INFERRED]
  internal/platform/security_test.go → internal/platform/auth.go
- `actionHash()` --calls--> `digest()`  [INFERRED]
  internal/platform/actions.go → internal/platform/store.go
- `main()` --calls--> `LoadConfig()`  [EXTRACTED]
  cmd/control-plane/main.go → internal/platform/config.go

## Import Cycles
- None detected.

## Communities (23 total, 6 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.08
Nodes (39): lucide-react, next, react, ref_server_only, config, Approvals(), Audit(), Onboarding() (+31 more)

### Community 1 - "Community 1"
Cohesion: 0.18
Nodes (16): net/http.Handler, net/http.Request, net/http.ResponseWriter, Server, actorFrom(), requestIP(), decode(), fail() (+8 more)

### Community 2 - "Community 2"
Cohesion: 0.13
Nodes (29): collectPostgres(), collectProbe(), collectValkey(), loopbackHost(), respCommand(), approvedDial(), check(), collectAndQueue() (+21 more)

### Community 3 - "Community 3"
Cohesion: 0.11
Nodes (14): collectHost(), context.Context, github.com/jackc/pgx/v5/pgxpool.Pool, sync.Mutex, Worker, urlQuery(), pgx.Tx, Config (+6 more)

### Community 4 - "Community 4"
Cohesion: 0.06
Nodes (30): dependencies, lucide-react, next, react, react-dom, devDependencies, eslint, eslint-config-next (+22 more)

### Community 5 - "Community 5"
Cohesion: 0.12
Nodes (17): event, database/sql.NullTime, encoding/json.RawMessage, time.Time, validateTurnstileResult(), Worker, TestTurnstileResultFailsClosed(), nullTime() (+9 more)

### Community 6 - "Community 6"
Cohesion: 0.17
Nodes (16): TestControlPlaneTransportRequiresHybridKeyExchange(), TestProbeRejectsBroadNetworkAndPlaintextRemoteValkey(), TestProbeRequiresApprovedCIDR(), go_pkg_crypto_x509, go_pkg_math, go_pkg_net_http_httptest, go_pkg_testing, testing.T (+8 more)

### Community 7 - "Community 7"
Cohesion: 0.11
Nodes (18): compilerOptions, allowJs, esModuleInterop, incremental, isolatedModules, jsx, lib, module (+10 more)

### Community 8 - "Community 8"
Cohesion: 0.18
Nodes (11): go_pkg_bufio, go_pkg_crypto_subtle, go_pkg_crypto_tls, go_pkg_errors, go_pkg_github_com_jackc_pgx_v5_pgconn, go_pkg_io, go_pkg_net_http, go_pkg_regexp (+3 more)

### Community 9 - "Community 9"
Cohesion: 0.18
Nodes (13): go_pkg_crypto_rand, go_pkg_crypto_sha256, go_pkg_database_sql, go_pkg_encoding_hex, go_pkg_encoding_json, go_pkg_fmt, go_pkg_github_com_jackc_pgx_v5, go_pkg_github_com_jackc_pgx_v5_pgxpool (+5 more)

### Community 10 - "Community 10"
Cohesion: 0.25
Nodes (10): main(), go_pkg_context, go_pkg_log, go_pkg_nocturn_example_aegis_operations_internal_platform, go_pkg_os, go_pkg_os_signal, go_pkg_path_filepath, go_pkg_sort (+2 more)

### Community 11 - "Community 11"
Cohesion: 0.19
Nodes (13): main(), main(), main(), go_pkg_net, NewOIDCVerifier(), env(), LoadConfig(), Config (+5 more)

### Community 12 - "Community 12"
Cohesion: 0.25
Nodes (8): go_pkg_crypto, go_pkg_crypto_rsa, go_pkg_encoding_base64, go_pkg_math_big, go_pkg_sync, actorKey, jwk, keySet

### Community 13 - "Community 13"
Cohesion: 0.60
Nodes (5): collectHost(), cpuCounters(), memoryUsedPercent(), networkCounters(), cpuCount

### Community 14 - "Community 14"
Cohesion: 0.33
Nodes (5): components, $defs, operations, paths, webhooks

### Community 15 - "Community 15"
Cohesion: 0.50
Nodes (3): NOTE: This file should not be edited, web_next_types_root_params_d, web_next_types_routes_d

### Community 16 - "Community 16"
Cohesion: 0.83
Nodes (4): OnboardingControls(), addDomain(), load(), post()

## Knowledge Gaps
- **55 isolated node(s):** `$defs`, `operations`, `paths`, `webhooks`, `config` (+50 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 101 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `next` connect `Community 0` to `Community 17`, `Community 4`?**
  _High betweenness centrality (0.038) - this node is a cross-community bridge._
- **Why does `actorFrom()` connect `Community 1` to `Community 3`, `Community 12`?**
  _High betweenness centrality (0.033) - this node is a cross-community bridge._
- **Why does `Server` connect `Community 1` to `Community 8`, `Community 3`, `Community 11`?**
  _High betweenness centrality (0.031) - this node is a cross-community bridge._
- **Are the 16 inferred relationships involving `fail()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`fail()` has 16 INFERRED edges - model-reasoned connections that need verification._
- **Are the 16 inferred relationships involving `writeJSON()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`writeJSON()` has 16 INFERRED edges - model-reasoned connections that need verification._
- **What connects `$defs`, `operations`, `paths` to the rest of the system?**
  _55 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.07785602503912363 - nodes in this community are weakly interconnected._