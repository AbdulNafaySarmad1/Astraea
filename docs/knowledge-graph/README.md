# Graphify repository graph

The Graphify CLI generated this code knowledge graph after the telemetry hardening slice. It contains 385 nodes, 1,015 edges, and 23 communities.

- [Interactive graph](graphify-out/graph.html)
- [Graph JSON](graphify-out/graph.json)
- [Community report](graphify-out/GRAPH_REPORT.md)

Rebuild after changes with `graphify extract . --code-only --force --out docs/knowledge-graph` and `graphify cluster-only docs/knowledge-graph --no-label`. The local code-only extractor excludes Markdown prose. SQL schema relationships were not extracted because the optional `tree_sitter_sql` dependency is absent; use the migrations and architecture docs for those boundaries.
