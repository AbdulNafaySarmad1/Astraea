# Graphify repository graph

The Graphify CLI generated this code knowledge graph after the service-log and archive implementation. It contains 469 nodes, 1,322 edges, and 16 communities.

- [Interactive graph](graphify-out/graph.html)
- [Graph JSON](graphify-out/graph.json)
- [Community report](graphify-out/GRAPH_REPORT.md)

Rebuild after changes with `graphify extract . --code-only --force --out docs/knowledge-graph` and `graphify cluster-only docs/knowledge-graph --no-label`. The local code-only extractor excludes Markdown prose. SQL schema relationships were not extracted because the optional `tree_sitter_sql` dependency is absent; use the migrations and architecture docs for those boundaries.
