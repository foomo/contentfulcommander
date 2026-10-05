# Changelog

## Unreleased (suggested: v0.8.0)

### Added

- **Reverse-reference index.** `GetParents`, `GetReferrerPath` and the new reverse queries are answered
  from an index built on the first reverse query and maintained incrementally. Lookups on a 100k-entity
  cache drop from ~180 ms to ~2 µs. `LoadSpaceModel` is unaffected, and clients that never query
  referrers pay nothing.
- `EntryEntity.GetParentsViaFields(fieldNames, contentTypes)` and
  `AssetEntity.GetParentsViaFields(fieldNames, contentTypes)`: parents restricted to referencing fields.
- `MigrationClient.GetReferrers(targetID, ReferrerFilter)` returning `[]Referrer` (entity, field, locale):
  every edge pointing at an ID, also for IDs that are not cached.
- `MigrationClient.ReindexEntity(entity)` and `MigrationClient.RebuildReferenceIndex()`: escape hatches
  after direct edits of cached fields that bypass `SetFieldValue`.
- `MigrationClient.RefreshEntityOutcome(ctx, id)` returning `RefreshOutcomeUpdated` or
  `RefreshOutcomeRemoved`.
- `ErrEntityNotFound` and `EntityNotFoundError`.
- `StatusArchived`, `EntryEntity.IsArchived()`, `AssetEntity.IsArchived()` and the nil-safe package helper
  `IsArchived(Entity)`.

### Changed: `GetPublishingStatus` reports `StatusArchived`

Archived entries and assets (`Sys.ArchivedVersion > 0`) now report `"archived"`, taking precedence over
the other statuses. Before, they reported `"draft"` (or `"changed"`). This affects
`GroupByPublishingStatus`, `CountByPublishingStatus` and `GetStats().PublishingStatusCounts`.

*Migration:* code that switches on the status should handle `StatusArchived`. To keep treating archived
entities as drafts, check `IsArchived(entity)` first. Which entities are cached is unchanged: archived
entities were and remain part of the cache.

### Changed: `RefreshEntity` removes entities Contentful no longer has

When both CMA lookups (entry, then asset) return not found, `RefreshEntity` now removes the stale entity
from the cache before returning an error. The error is now an `*EntityNotFoundError` matching
`errors.Is(err, ErrEntityNotFound)`, and its message (`entity <id> not found`) is unchanged. Any other
failure (network, 5xx, 429, auth) now leaves the cache untouched and returns the underlying error,
which is wrapped, instead of `entity <id> not found`.

The asset lookup now runs only after the entry lookup returned not found. Before, any entry lookup
failure fell through to the asset lookup. Now an entry lookup failing with 5xx, 429, auth or a network
error returns that error, even when the ID is an asset that would have been found.

*Migration:* callers that treat the error as "missing" keep working; prefer `errors.Is(err,
ErrEntityNotFound)` over string matching. Use `RefreshEntityOutcome` to treat a removal as success.

### Changed: `GetParents` order is deterministic

`GetParents` returns parents sorted by entity ID; before, the order was random (map iteration).

*Migration:* none needed. Code that sorted the result itself can stop doing so.

### Changed: `EntryEntity.SetFieldValue` takes the client's cache lock

For entries with a client, `SetFieldValue` now mutates the fields under the cache write lock, so it is
race-free against reverse queries, and it re-extracts the entry's references when the entry is the
cached one.

*Migration:* none needed. Do not call it from code that holds the client's lock (the library never
calls back into user code while holding it).

### Changed: CMA writes are applied to the entity under the cache lock

`SaveDraft`, `Publish` and the `MigrationExecutor` operations now send a deep copy of the entity to the
CMA and apply the response (version, sys, fields, metadata) to the entity under the client's cache lock,
re-extracting its references in the same step. Before, the SDK decoded the response straight into the
(possibly cached) entity, racing with concurrent readers. The entity's `Sys` struct and field map keep
their identity, and only values that changed are written. Identity data (ID, type, content type) is
never rewritten, so reading it concurrently with a save is safe. Reading values a save changes (version,
publishing status, fields) concurrently with saving the same entity remains the caller's race, as before. One difference: when a version-conflict retry fails, the entity no longer keeps the
version refreshed from the server; it is left as it was before the write.

*Migration:* none needed.

### Fixed

- `GetParents` and `GetReferrerPath` iterated the cache without holding its lock (a data race with
  concurrent cache writers).
- `UpdateSpaceModel` no longer carries a stale CDA view over to an entity that became archived, and no
  path attaches a CDA view to an archived entity. `RefreshEntity` does not query the CDA for archived
  entities.

### Documented

- `UpdateSpaceModel` cannot observe deletions (the CMA has no deletion feed): call `RefreshEntity` /
  `RefreshEntityOutcome` for entity events, or reload.
- Which mutation paths the reverse-reference index tracks, and which it does not (direct writes to
  `Entry.Fields` or the maps returned by `GetFields` / `GetFieldValue`).
