# Graph Report - knowledge-graph  (2026-09-23)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 319 nodes · 804 edges · 16 communities (10 shown, 6 thin omitted)
- Extraction: 91% EXTRACTED · 9% INFERRED · 0% AMBIGUOUS · INFERRED: 75 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

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
- Community 13
- Community 14
- Community 15

## God Nodes (most connected - your core abstractions)
1. `fail()` - 35 edges
2. `writeJSON()` - 34 edges
3. `Server` - 26 edges
4. `api()` - 18 edges
5. `actorFrom()` - 18 edges
6. `next` - 16 edges
7. `compilerOptions` - 16 edges
8. `decode()` - 12 edges
9. `PageHead()` - 11 edges
10. `DataState()` - 10 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/audit-verify/main.go → internal/platform/store.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/migrate/main.go → internal/platform/store.go
- `main()` --calls--> `LoadConfig()`  [EXTRACTED]
  cmd/control-plane/main.go → internal/platform/config.go
- `main()` --calls--> `Open()`  [EXTRACTED]
  cmd/control-plane/main.go → internal/platform/store.go
- `main()` --calls--> `LoadConfig()`  [EXTRACTED]
  cmd/worker/main.go → internal/platform/config.go

## Import Cycles
- None detected.

## Communities (16 total, 6 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.06
Nodes (58): main(), check(), main(), send(), TestProbeRequiresApprovedCIDR(), main(), main(), Config (+50 more)

### Community 1 - "Community 1"
Cohesion: 0.09
Nodes (35): lucide-react, next, react, ref_server_only, config, Approvals(), Audit(), Onboarding() (+27 more)

### Community 2 - "Community 2"
Cohesion: 0.06
Nodes (35): event, main(), go_pkg_crypto, go_pkg_crypto_rsa, go_pkg_encoding_base64, go_pkg_math_big, go_pkg_sync, context.Context (+27 more)

### Community 3 - "Community 3"
Cohesion: 0.18
Nodes (15): net/http.Handler, net/http.Request, net/http.ResponseWriter, actionHash(), Server, actorFrom(), requestIP(), decode() (+7 more)

### Community 4 - "Community 4"
Cohesion: 0.06
Nodes (30): dependencies, lucide-react, next, react, react-dom, devDependencies, eslint, eslint-config-next (+22 more)

### Community 5 - "Community 5"
Cohesion: 0.11
Nodes (18): compilerOptions, allowJs, esModuleInterop, incremental, isolatedModules, jsx, lib, module (+10 more)

### Community 6 - "Community 6"
Cohesion: 0.33
Nodes (5): components, $defs, operations, paths, webhooks

### Community 7 - "Community 7"
Cohesion: 0.40
Nodes (3): metadata, nav, web_src_app_styles

### Community 8 - "Community 8"
Cohesion: 0.50
Nodes (3): NOTE: This file should not be edited, web_next_types_root_params_d, web_next_types_routes_d

### Community 9 - "Community 9"
Cohesion: 0.83
Nodes (4): OnboardingControls(), addDomain(), load(), post()

## Knowledge Gaps
- **53 isolated node(s):** `$defs`, `operations`, `paths`, `webhooks`, `ModelGateway` (+48 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 95 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `next` connect `Community 1` to `Community 10`, `Community 4`, `Community 7`?**
  _High betweenness centrality (0.055) - this node is a cross-community bridge._
- **Why does `Server` connect `Community 3` to `Community 0`, `Community 2`?**
  _High betweenness centrality (0.031) - this node is a cross-community bridge._
- **Why does `actorFrom()` connect `Community 3` to `Community 2`?**
  _High betweenness centrality (0.028) - this node is a cross-community bridge._
- **Are the 15 inferred relationships involving `fail()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`fail()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **Are the 15 inferred relationships involving `writeJSON()` (e.g. with `.actions()` and `.approveAction()`) actually correct?**
  _`writeJSON()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **What connects `$defs`, `operations`, `paths` to the rest of the system?**
  _53 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.06468797564687975 - nodes in this community are weakly interconnected._