# Feishu Sync Liveness Design

## Goal

Recover Feishu synchronization and staged ingestion automatically after a worker process or a single-attempt River job is lost, without weakening snapshot, transaction, or compare-and-swap guarantees.

## Durable Pending State

Migration `0010_document_pending_snapshot_payload.sql` adds nullable `pending_title`, `pending_bytes`, and `pending_metadata` columns. A changed snapshot is recoverable only when the existing content ref, checksum, and remote revision plus these payload fields are all present and valid.

Staging writes the complete pending set atomically with the existing claim-token and active/pending guards. Promotion reads the payload associated with that pending tuple, updates active fields, and clears every pending field in the same transaction. Unchanged completion also clears every pending field. Failure retains every pending field so a later reconciliation can retry. Replacement stages overwrite the complete pending set atomically.

The reconciler never substitutes active title, byte count, or metadata for missing pending data. Legacy or incomplete pending states enqueue a fresh guarded sync so the source is loaded and staged again. When that stale row is reclaimed, the claim query atomically clears the entire incomplete pending set; complete pending sets remain intact for exact reconciliation. Reconstructed payloads require a non-negative byte count and typed source metadata that passes the existing allowlisted domain decoder; unsafe or unknown metadata fields are rejected.

## River Reconciliation

`feishu_reconcile` is a singleton River job with `MaxAttempts: 1` and active-state uniqueness. Production registers it as a River periodic job with `RunOnStart` and a configured interval. No process-local ticker or goroutine owns scheduling.

The worker scans stale `syncing` Feishu documents with a PostgreSQL-clock cutoff. Results are ordered by `(updated_at, id)` and traversed with a keyset cursor. Both page size and pages per run are bounded.

For each stale document:

- No pending fields: enqueue `feishu_sync` with the exact active revision.
- Complete valid pending state: enqueue `ingestion` with the exact pending tuple, durable payload, and `updated_at` claim token.
- Partial or invalid pending state: enqueue a fresh `feishu_sync` with the exact active revision; the normal claim path reclaims and replaces the broken pending state.

Existing sync and ingestion active-state uniqueness makes insertion a no-op while the original job still exists. Discarded terminal jobs do not block recovery.

## Lease And Timeouts

`ClaimFeishuSync` and stale reconciliation both compare `updated_at` against PostgreSQL `now() - lease_seconds * interval '1 second'`. Claim creation already sets `updated_at = now()`, so creation and expiry share one clock. Application time is not used for SQL lease decisions.

Feishu sync and ingestion workers expose River `Timeout` methods backed by `FEISHU_SYNC_JOB_TIMEOUT` and `INGESTION_JOB_TIMEOUT`. Reconciliation has its own bounded timeout. Startup rejects non-positive durations, a reconciliation interval that is not shorter than the sync lease, a sync lease that is not longer than the sum of sync and ingestion timeouts, and `RIVER_RESCUE_STUCK_JOBS_AFTER` that is not longer than every individual worker timeout. The sum constraint accounts for ingestion retaining the original sync claim token. These relationships prevent normal work from becoming DB-stale or River-rescuable before its configured deadline.

## Cleanup Observability

Snapshot deletion remains best effort and never changes the main job result. A failed delete emits one structured warning through an injected `slog.Logger`. The warning includes only the document UUID, claim timestamp, and a short SHA-256 hash of the storage key. It excludes the key itself, token, URL, content, provider response, and raw error text.

## Tests

Tests cover discarded sync and ingestion recovery, active-state uniqueness configuration, incomplete pending fallback, metadata validation, keyset and batch bounds, periodic production registration, worker timeouts and invalid configuration combinations, DB-clock SQL contracts and sqlc provenance, pending-payload migration structure, and redacted delete warnings. Full verification includes sqlc generation/vet, focused and repository-wide Go tests, Go vet, gofmt, and diff checks. Live PostgreSQL/Goose execution remains an explicit residual gap in this environment.
