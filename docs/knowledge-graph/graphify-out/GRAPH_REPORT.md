# Graph Report - knowledge-graph  (2026-09-23)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 469 nodes · 1322 edges · 16 communities (12 shown, 4 thin omitted)
- Extraction: 90% EXTRACTED · 10% INFERRED · 0% AMBIGUOUS · INFERRED: 136 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `f7632ac6`
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
- Community 13
- Community 14
- Community 15

## God Nodes (most connected - your core abstractions)
1. `fail()` - 40 edges
2. `writeJSON()` - 38 edges
3. `Server` - 26 edges
4. `actorFrom()` - 20 edges
5. `api()` - 18 edges
6. `next` - 16 edges
7. `decode()` - 16 edges
8. `compilerOptions` - 16 edges
9. `logStorage` - 14 edges
10. `Open()` - 12 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/audit-verify/main.go → internal/platform/store.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/migrate/main.go → internal/platform/store.go
- `actionHash()` --calls--> `digest()`  [INFERRED]
  internal/platform/actions.go → internal/platform/store.go
- `TestProxyHeaderOnlyFromTrustedNetwork()` --calls--> `requestIP()`  [INFERRED]
  internal/platform/security_test.go → internal/platform/auth.go
- `TestJSONDecoderRejectsTrailingAndOversizedInput()` --calls--> `decode()`  [INFERRED]
  internal/platform/log_request_test.go → internal/platform/http.go

## Import Cycles
- None detected.

## Communities (16 total, 4 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.06
Nodes (49): lucide-react, next, ref_node_crypto, react, ref_server_only, config, Approvals(), Audit() (+41 more)

### Community 1 - "Community 1"
Cohesion: 0.08
Nodes (45): collectHost(), cpuCounters(), memoryUsedPercent(), networkCounters(), logFileIdentity(), logFileIdentity(), cpuCount, go_pkg_bytes (+37 more)

### Community 2 - "Community 2"
Cohesion: 0.16
Nodes (18): net/http.Handler, net/http.Request, net/http.ResponseWriter, Server, actorFrom(), requestIP(), decode(), fail() (+10 more)

### Community 3 - "Community 3"
Cohesion: 0.08
Nodes (34): event, go_pkg_compress_gzip, go_pkg_crypto_aes, go_pkg_crypto_cipher, go_pkg_crypto_rand, go_pkg_io, go_pkg_runtime, database/sql.NullTime (+26 more)

### Community 4 - "Community 4"
Cohesion: 0.07
Nodes (26): collectHost(), go_pkg_crypto, go_pkg_crypto_rsa, go_pkg_math_big, go_pkg_sync, context.Context, github.com/jackc/pgx/v5/pgxpool.Pool, sync.Mutex (+18 more)

### Community 5 - "Community 5"
Cohesion: 0.10
Nodes (35): collectPostgres(), collectProbe(), collectValkey(), loopbackHost(), respCommand(), approvedDial(), check(), collectAndQueue() (+27 more)

### Community 6 - "Community 6"
Cohesion: 0.06
Nodes (30): dependencies, lucide-react, next, react, react-dom, devDependencies, eslint, eslint-config-next (+22 more)

### Community 7 - "Community 7"
Cohesion: 0.13
Nodes (22): main(), main(), main(), main(), main(), testing.T, env(), LoadConfig() (+14 more)

### Community 8 - "Community 8"
Cohesion: 0.18
Nodes (15): atomicPrivateFile(), collectLogs(), collectLogSource(), flushLogs(), Config, openLogSpool(), TestServiceLogTailStartsAtEndAndFiltersSensitiveLines(), validateLogSource() (+7 more)

### Community 9 - "Community 9"
Cohesion: 0.11
Nodes (18): compilerOptions, allowJs, esModuleInterop, incremental, isolatedModules, jsx, lib, module (+10 more)

### Community 10 - "Community 10"
Cohesion: 0.36
Nodes (5): Worker, Diagnosis, MockGateway, ModelGateway, Signal

### Community 11 - "Community 11"
Cohesion: 0.50
Nodes (3): NOTE: This file should not be edited, web_next_types_root_params_d, web_next_types_routes_d

## Knowledge Gaps
- **56 isolated node(s):** `$defs`, `operations`, `paths`, `webhooks`, `config` (+51 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 103 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **4 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `actorFrom()` connect `Community 2` to `Community 4`?**
  _High betweenness centrality (0.032) - this node is a cross-community bridge._
- **Why does `Server` connect `Community 2` to `Community 1`, `Community 4`, `Community 7`?**
  _High betweenness centrality (0.026) - this node is a cross-community bridge._
- **Why does `next` connect `Community 0` to `Community 6`?**
  _High betweenness centrality (0.026) - this node is a cross-community bridge._
- **Are the 20 inferred relationships involving `fail()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`fail()` has 20 INFERRED edges - model-reasoned connections that need verification._
- **Are the 20 inferred relationships involving `writeJSON()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`writeJSON()` has 20 INFERRED edges - model-reasoned connections that need verification._
- **What connects `$defs`, `operations`, `paths` to the rest of the system?**
  _56 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.06050420168067227 - nodes in this community are weakly interconnected._