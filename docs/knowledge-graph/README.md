# Graphify repository graph

The Graphify CLI generated this code knowledge graph after the application implementation. It contains 319 nodes, 804 edges, and 16 communities as of the initial build.

- [Interactive graph](graphify-out/graph.html)
- [Graph JSON](graphify-out/graph.json)
- [Community report](graphify-out/GRAPH_REPORT.md)

Rebuild after changes with `graphify extract . --code-only --force --out docs/knowledge-graph` and `graphify cluster-only docs/knowledge-graph --no-label`. The local code-only extractor does not include Markdown prose or SQL schema relationships, so use the source files and architecture docs for those boundaries.
