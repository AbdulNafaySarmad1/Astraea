# Service log ingestion and 365-day archive

This implementation accepts **only named local infrastructure service log files** configured on a tenant-bound connector. Supported labels are `postgres`, `valkey`, `vault`, and `aegiscore`. The label does not grant access to a process, database, device, object store, or remote endpoint. R2 API log retrieval, Docker log drivers, journald, and general host or user-device log collection are not implemented. Keep academic, payroll, fee, student, SQL statement, and credential content out of the approved source files.

## Approve sources and enroll

An authorized tenant manager issues a 15-minute enrollment token through `POST /v1/tenants/{tenant}/enrollment-tokens`. The request binds capabilities, exact log source names/kinds, and hashes of their approved absolute paths to the token:

```json
{
  "capabilities": ["tcp_health", "service_logs"],
  "log_sources": [
    {"name": "primary_db", "kind": "postgres", "path_sha256": "<sha256-of-exact-absolute-path>"},
    {"name": "cache", "kind": "valkey", "path_sha256": "<sha256-of-exact-absolute-path>"}
  ]
}
```

Calculate each lowercase SHA-256 from the exact absolute path string in the customer connector config (for example, `printf %s /var/log/aegisops-approved/postgres.log | sha256sum` on Linux). The server records that scope when the connector enrolls; the connector cannot add capabilities or substitute a different path by claiming them in its enrollment or log request. A compromised connector can still lie about local file contents. The console's current one-click token issues `tcp_health` only. Use the authenticated API for additional approved capabilities and review the source scope with the customer before monitoring approval. Existing enrolled connectors need a new scoped enrollment to collect logs.

Configure the customer connector with exact **absolute paths** to the locally approved, pre-filtered service files. Paths never come from the control plane:

```json
{
  "log_sources": [
    {"name": "primary_db", "kind": "postgres", "path": "/var/log/aegisops-approved/postgres.log"},
    {"name": "cache", "kind": "valkey", "path": "/var/log/aegisops-approved/valkey.log"}
  ]
}
```

Add these fields to the normal connector config in [connector telemetry](connector-telemetry.md). Run the connector on Linux with read-only access to these regular files, no symlink in the approved path, and a persistent private `spool_dir`. It starts at the end of each file on first use, so historical contents are never backfilled automatically. Every collection interval it reads at most 256 KiB per source and queues up to 1,024 filtered lines per batch. The log spool holds at most 3,000 batches. A full spool, unreadable file, oversized unbroken line, or delivery rejection requires operator intervention; cursors do not advance after a queue failure. **Rapid rotation can still remove unread lines**, so retain rotated files until a rotation-aware collector is implemented and watch source lag. This is a production gate.

The connector and API both filter obvious credentials, bearer tokens, email addresses, SQL statements, bind parameters, binary lines, and lines over 8 KiB. This is defense in depth, **not a guarantee that logs contain no PII or secrets**. Configure the source service to omit statement text, parameters, request bodies, and business records before the connector can read its files. Validate a sample in a non-customer environment.

## Storage and retention

Configure `LOG_HOT_DIR`, `LOG_HOT_MARKER`, and `LOG_KEY_B64` on the API. `LOG_KEY_B64` is a base64-encoded 32-byte AES key supplied by a scoped secrets manager, never a checked-in file. Provision the hot volume before startup with a private root directory and a `.aegisops-store-id` file containing the exact marker string. Hot batches are stored as AES-256-GCM encrypted files. The API commits tenant/source/checksum metadata and an audit event in PostgreSQL after the encrypted file is durable. A retry with the same batch ID and content is acknowledged without creating another record. No log content is stored in PostgreSQL or InfluxDB.

Configure the same hot directory, marker, and key, plus `LOG_ARCHIVE_DIR` and `LOG_ARCHIVE_MARKER`, on the separately deployed `cmd/log-archive` service (`go run ./cmd/log-archive` for development). The API and archive worker must see the same persistent hot filesystem. Mount `LOG_ARCHIVE_DIR` on the separately administered Storage Box or another approved off-host archive filesystem. Create its private `.aegisops-store-id` file **on the mounted filesystem**, not on the underlying local mountpoint. Use different marker strings for hot and archive. The worker checks both identities before each archive cycle and again before completion; a missing marker prevents hot-copy deletion. The application verifies file writes and readback, but **cannot itself prove that a filesystem path is off-host or backed up**. Keep both directories private to their service identities, with sufficient capacity and monitoring. Do not use the demo PostgreSQL volume for log contents.

After **365 days from server receipt**, the archive worker reads a hot batch, verifies its plaintext SHA-256, gzip-compresses it, encrypts it with AES-256-GCM, writes an archive file and JSON manifest, then decrypts and decompresses the archive to verify exact content. It records the archive checksum and an audit event before a separate locked step can delete the hot copy. The deletion step checks the archive and manifest again and skips batches with `legal_hold=true`. Failed verification leaves the hot copy in place. Archive failures back off; after eight attempts the batch becomes `blocked`, allowing other tenants to continue. A manager can repair the cause and call `POST /v1/tenants/{tenant}/log-batches/{connector}/{batch}/retry-archive`; that reset is audited and customer-notified. Failed hot-copy cleanup also backs off while leaving the copy intact. Archived batches are **not automatically deleted**; define an archive expiry and legal-hold policy before production use.

`GET /v1/tenants/{tenant}/log-batches` returns tenant-scoped **metadata only** (up to 100 records per request), and that access is audited and queued for customer notification. A tenant manager can set or release a hold with `POST /v1/tenants/{tenant}/log-batches/{connector}/{batch}/legal-hold` and `{"engaged":true}` or `false`; the change and customer notice are committed together. A hold set after a hot copy was removed protects the archive but cannot restore that hot copy.

## Recovery and operational checks

1. Monitor connector log spool count, source lag, API 4xx/5xx delivery, `log_batches` rows older than 365 days with `status='hot'` or `blocked`, archive and cleanup attempt counts, and hot copies pending deletion. There is no built-in alert router yet.
2. If the worker or archive mount is unavailable, leave hot copies intact and repair storage before restarting archival. Never manually delete a hot file based only on its age.
3. Restore a sampled archive by checking the manifest checksum, decrypting with the protected key, decompressing, and comparing the plaintext hash with PostgreSQL metadata. Run a scheduled restore drill before customer onboarding.
4. Preserve old encryption keys when rotating them. This version has one active key and no keyring or re-encryption workflow; key rotation and recovery need implementation before production deployment.
5. For a suspected leak, revoke connector access, stop ingestion, preserve audit and archive evidence, rotate credentials and keys with a reviewed recovery plan, and notify affected customers under the incident procedure.

The current archive is **per batch** rather than a daily aggregate. At high log volume, file count and filesystem overhead may be significant. Capacity projections, archive key rotation, rotation-aware tailing, alert routing, a retrieval workflow, and integration tests against the selected Storage Box mount remain open.
