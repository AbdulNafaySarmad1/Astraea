# Connector telemetry slice

The customer-side Go connector now collects only the sources explicitly listed in its local configuration. It initiates outbound HTTPS requests to the control plane. The server binds its credential, batch ID, capabilities, and resulting records to one tenant. No target or command is accepted from a model or control-plane response.

## Local configuration

The connector reads `-config <path>` (default `connector.json`). This example uses reserved addresses and names; replace them only with customer-approved endpoints. Do not put passwords in JSON.

```json
{
  "control_plane_url": "https://ops.example.com",
  "tenant_id": "<enrolled-tenant-uuid>",
  "connector_id": "<enrolled-connector-uuid>",
  "version": "<deployed-version>",
  "spool_dir": "/var/lib/aegisops-connector/spool",
  "collection_interval_seconds": 900,
  "host_metrics": true,
  "probes": [
    {"name":"AegisCore API","kind":"tcp","host":"192.0.2.10","port":443,"allowed_cidrs":["192.0.2.10/32"]},
    {"name":"PostgreSQL primary","kind":"postgres","host":"db.example.com","port":5432,"allowed_cidrs":["192.0.2.20/32"],"credential_env":"CUSTOMER_POSTGRES_MONITOR_DSN","min_replicas":0},
    {"name":"Valkey primary","kind":"valkey","host":"cache.example.com","port":6379,"allowed_cidrs":["192.0.2.30/32"],"credential_env":"CUSTOMER_VALKEY_MONITOR_PASSWORD","username":"monitor","tls":true}
  ]
}
```

Set `CONNECTOR_CREDENTIAL` to the one-time enrollment result using a customer-specific secret scope. `CUSTOMER_POSTGRES_MONITOR_DSN` must be a PostgreSQL URL whose host and port match the probe. Outside loopback, use verified TLS (`sslmode=verify-full`) and a restricted monitoring role with access to the required `pg_stat_*` views. `CUSTOMER_VALKEY_MONITOR_PASSWORD` belongs to a Valkey ACL identity permitted to run `AUTH`, `PING`, and the four `INFO` sections (`memory`, `clients`, `stats`, `replication`). Remote Valkey probes require TLS. Assign each connector only the approved `tcp_health`, `host_metrics`, `postgres_health`, and/or `valkey_health` capabilities at enrollment; the API rejects signals outside the recorded capabilities.

The `host_metrics` switch reports CPU, available memory, root filesystem use, and network byte counters for the machine or namespace where the connector itself runs. A container does not automatically see the host's full filesystem or network; deploy it with a reviewed host monitoring arrangement if host-wide data is required. The PostgreSQL probe reports connections, transaction rollbacks, primary replica count, and WAL replay backlog; it never reads application tables or query text. The Valkey probe reports memory, clients, evictions, and replica link state; it never reads keys or values. TCP checks report reachability and latency only. R2 metrics, application-specific checks, and service logs remain separate integrations.

The connector collects at most every 15 minutes by default and sends a lightweight heartbeat every 30 seconds. Its private spool holds up to 672 batches (seven days at the default interval); it writes each batch before network delivery, retries oldest first, and removes it only after a 200 acknowledgement. The control plane deduplicates batch IDs. A full or corrupt spool requires operator intervention and is logged locally. Install the spool on a persistent, access-restricted filesystem; the current implementation does not encrypt this local spool.

The connector's HTTPS client requires TLS 1.3 with `X25519MLKEM768` hybrid key exchange and normal certificate validation. If a reverse proxy or edge does not negotiate that group, telemetry delivery fails closed. This covers only the connector-to-control-plane TLS hop. The control-plane certificate signature, database, storage, identity, and other service connections have not been validated for post-quantum cryptography. Check edge support before deployment; do not silently downgrade the connector.

The API validates metric names, units, count, timestamps, capabilities, and tenant-bound connector identity. Numeric samples and an audit record commit in one PostgreSQL transaction. Replayed older samples can add history without replacing a newer component state. A health worker opens one incident per degraded, unknown, or stale component signal and closes degraded/unknown incidents after two consecutive healthy samples. It does not execute any infrastructure operation. Component telemetry is stale after 30 minutes; connector connectivity is stale after five minutes.

## Remaining production work

- Rotate connector credentials and replace the long-lived bearer token with approved workload identity or mutual authentication.
- Add customer-approved AegisCore inventory discovery, R2 telemetry, application checks, backup/replication verification, and safe service-log collection.
- Add off-host, encrypted log archiving with manifests, retention/legal holds, verified restore, and deletion only after archive confirmation.
- Add configurable per-customer anomaly rules, alert routing, maintenance windows, and incident acknowledgement.
- Verify post-quantum transport and key management across the other service connections, certificate signatures, database, storage, and identity provider. Add a deployment check that confirms the configured edge negotiates the required hybrid group.
