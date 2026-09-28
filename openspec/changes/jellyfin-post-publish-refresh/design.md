## Context

See `proposal.md` for motivation and `specs/jellyfin-post-publish-refresh/spec.md` for the behavior contract.

The current Test Encode worker writes its temporary media into the selected Library, activates the media and generated subtitle sidecars, checks and probes the output, builds its validation report, then persists `READY`. `TestEncode.LibraryID` is already persisted. Explicit deletion and housekeeping both call the shared Test Encode file-removal helper; housekeeping excludes `Keep=true` and only expires non-running records after its cleanup succeeds. These are the integration boundaries: READY notification follows a successful persisted transition, and removal notification follows actual successful cleanup or records bounded reconciliation evidence if cleanup partially mutated Library content before failing.

The generic Settings list currently returns stored `AppSetting` values. A Jellyfin credential therefore needs a sanitized Settings read projection; adding a credential to a value returned unchanged by that API would expose plaintext to the frontend. Existing typed provider settings normalization and persistence can be followed without introducing authentication infrastructure.

After rebasing onto the merged #54 contract, `AssetPublishedOutbox` supplies a stable `EventID` (`asset-published-<PublicationID>`), `PublicationID`, and authoritative `LibraryID` in the event payload. The payload also contains `DestinationPath`, but the subscriber uses only the stable event and publication identities plus `LibraryID`; it never infers Library identity from that path. The outbox is created only after the publication and archive outcome are durably recorded and retries delivery with the same event identity.

The current Jellyfin TypeScript SDK exposes `GET /Library/MediaFolders` for library discovery and `GET /System/Info/Public` for public server information. It documents `POST /Items/{itemId}/Refresh` as refreshing item metadata and `POST /Library/Refresh` as starting a library scan. The item operation does not establish that a mapped library root refresh detects descendant additions and removals, so this implementation uses the documented server-wide scan and discloses that broader scope. It does not guess support from a Jellyfin minor version or a mapped-item 404. See the [Jellyfin Library API](https://github.com/jellyfin/jellyfin-sdk-typescript/blob/master/src/generated-client/api/library-api.ts) and [Jellyfin System API](https://github.com/jellyfin/jellyfin-sdk-typescript/blob/master/src/generated-client/api/system-api.ts).

## Goals / Non-Goals

**Goals:**

- Consume canonical publication only through the final #54 `AssetPublished` contract.
- Notify one integration coordinator from that subscriber and the distinct Test Encode READY/removal boundaries.
- Persist deduplicated, bounded delivery state that supports UI, retry, coalescing, and restart recovery.
- Bind MVForge Library IDs to discovered Jellyfin Library IDs and keep credentials backend-owned.
- Prefer scoped refresh where it is proven to detect the Library changes in scope; disclose and bound any server-wide fallback.

**Non-Goals:**

- Changing Publisher, Validation, Queue, or Test Encode lifecycle authority.
- Replaying historical publications or Test Encodes after enabling the integration.
- Adding a generic event bus, scheduler, integration framework, retry service, or credential-authentication subsystem.
- Managing Jellyfin metadata, images, posters, databases, or files; adding Plex; or implementing #55 metrics and dashboards.

## Decisions

### Keep trigger authorities separate

The canonical subscriber accepts only `AssetPublished` from the merged #54 outbox. It requires the event's stable `EventID`, `PublicationID`, and numeric `LibraryID`, and hands the event identity plus the authoritative Library ID to the coordinator. It ignores `DestinationPath` for target resolution. It does not call or modify Publisher, and it does not subscribe to worker, validation, staging, or generic job completion signals.

Test Encode sends `test_encode_ready` only after the media and required sidecars have been activated, output probing/validation has succeeded, and the database update has persisted `READY` successfully. It resolves the target only from the persisted `TestEncode.LibraryID`. Test Encode never creates publication evidence, emits `AssetPublished`, or participates in #53 authorization.

Explicit deletion and expiration/housekeeping send `test_encode_removed` when cleanup physically mutates a previously `READY` record's Library output or registered sidecars. Make the shared cleanup boundary return bounded mutation evidence alongside any error; if an early artifact is removed and a later removal fails, request reconciliation for the changed Library while preserving the caller's existing failed cleanup/deletion semantics. Do not report the Test Encode as successfully deleted. Do not hook failure cleanup, cancellation, startup recovery, or dry-run housekeeping. A cleanup that finds no owned output or sidecar to remove is not a removal trigger, and no filesystem scan is added to infer mutation afterward.

### Use one settings record, client, coordinator, and delivery ledger

Add one typed Jellyfin provider setting through existing Settings normalization and persistence. Store mappings in that same configuration as `MVForge Library ID -> Jellyfin Library ID`; retain display names only as refreshed presentation data. IDs from Jellyfin discovery are copied exactly and are never inferred from names or paths.

Use one Jellyfin HTTP client with a finite request timeout for connection tests, discovery, and refresh calls. Connection testing first obtains reachability and public server information when available, then authenticates and discovers libraries. The current upstream SDK documents `/System/Info/Public`, `/Library/MediaFolders`, and token-backed authenticated calls in its [generated clients and usage example](https://github.com/jellyfin/jellyfin-sdk-typescript).

Use one Jellyfin delivery/outcome ledger for pending work, deduplication, attempts, and UI evidence. It is specific to this integration and is not an event bus or generic scheduler. A unique source key prevents duplicate `AssetPublished` delivery and duplicate Test Encode readiness/removal observations. Manual requests receive their own request key. The same stored row moves through bounded pending, success, skipped, retryable-failure, or permanent-configuration-error outcomes; no second work queue or evidence store is introduced.

### Redact the credential at the Settings boundary

Keep the API key/token in the existing backend Settings persistence, but ensure generic Settings reads and the typed Jellyfin response replace it with `credentialConfigured`. An omitted credential on update preserves the current value; explicit replacement or clearing is represented by a deliberate write action. Neither update responses nor diagnostics return the secret. Use silent SQL logging for credential writes and fixed error responses so a failed Settings write cannot echo its SQL value. Redact the authorization header and any credential-bearing URL before logging, persisting, or displaying errors. Retry evidence stores only a bounded failure classification, never raw Jellyfin response text.

This adds no new authentication or encryption subsystem. The required guarantee is that the secret never leaves the backend through Settings reads, History, diagnostics, retry evidence, logs, or metrics labels.

### Resolve and refresh the effective Jellyfin target

Connection testing discovers libraries from Jellyfin's documented media-folder listing and returns their stable IDs plus display names. Settings map those IDs to existing MVForge Library IDs. A stale or unknown Jellyfin ID is a permanent configuration error; the integration never substitutes an ID by name.

Use a mapped Jellyfin Library refresh only when the connected server's supported contract establishes that the operation detects descendant additions and removals. Do not select behavior from one hard-coded Jellyfin minor version. The documented item refresh contract covers metadata refresh but does not establish descendant addition/removal detection, so this implementation uses the documented `POST /Library/Refresh` server-wide scan and records that broader effective scope for each attempted request. It validates the exact mapped ID against `/Library/MediaFolders` before scanning; it does not use a mapped-item GET/404 to infer scoped-refresh support. The upstream SDK distinguishes item metadata refresh from starting a library scan in its [Library API contract](https://github.com/jellyfin/jellyfin-sdk-typescript/blob/master/src/generated-client/api/library-api.ts).

### Persist debounce and retry state in the coordinator

Default the debounce window to 10 seconds and allow configuration from 0 through 60 seconds. Flush no later than 60 seconds after the first pending trigger for an effective target, even if new triggers keep arriving. For a proven scoped refresh, debounce identity is `(Jellyfin configuration revision, Jellyfin Library ID)`. With the current server-wide scan fallback, identity is `(Jellyfin configuration revision, server-wide)`, so triggers mapped to different Jellyfin Libraries intentionally coalesce because one scan covers all Libraries. A URL or credential is never used as a metric label.

Persist each source's deduplication identity, reason, effective target, first/due timestamps, bounded state, attempt count, next-attempt time, and sanitized failure class. Pending requests for the same effective target coalesce and share one scan result. Different proven scoped Library IDs would debounce independently; the current server-wide fallback coalesces all mapped IDs. Terminal rows are retained for at most 180 days and at most 2,000 rows; pending rows are never trimmed. On restart, the coordinator reloads pending rows and restores their due/backoff timing; already-due work is dispatched once. It uses integration-specific timers and persisted rows, not another scheduler.

Apply one shared retry policy to all trigger families: a 10-second request deadline, at most five attempts total, and bounded retry delays of 1 second, 5 seconds, 30 seconds, and 2 minutes (a Jellyfin `Retry-After` value may shorten or lengthen a delay only up to the same 2-minute cap). Network errors, timeouts, rate limits, and server errors are retryable. Invalid credentials, invalid configuration, and stale mappings are permanent until corrected; after a settings revision corrects the configuration and the target is mapped, an explicit retry may reuse the same coordinator. New same-target requests join a pending retry window. Once automatic attempts are exhausted, retain `retryable_failure` for an explicit retry through the same coordinator. A repeated refresh asks Jellyfin to reconcile current Library state; it does not mutate media or MVForge lifecycle state. Persist attempt intent before the network call and serialize calls per effective target to bound duplicate work after restart.

The Jellyfin refresh endpoint acknowledges a scan request rather than proving that scanning has completed. Record `success` as request acceptance and show the UI wording as “Requested” or “Refresh accepted,” not as proof that Jellyfin finished indexing.

### Keep integration evidence separate and bounded

Expose the bounded per-source outcome to Publisher/History and Test Encode views, including the last status and time and an eligible retry action. Keep its reason values limited to `asset_published`, `test_encode_ready`, `test_encode_removed`, and `manual_refresh`; these are integration context only.

The outcome contract for #55 contains bounded status, duration/timestamps, attempt count, and a sanitized failure class. It does not add Prometheus implementation here and never makes asset paths, filenames, record IDs, event IDs, Jellyfin URLs, credentials, or raw error text metric labels.

## Risks / Trade-offs

- [A publication event could be malformed] → Require its stable EventID, PublicationID, and numeric LibraryID; never infer the Library from DestinationPath.
- [Scoped item refresh may not discover filesystem additions/removals on every server contract] → Require capability evidence for scoped recursive Library scanning; otherwise use the documented, explicitly broader server-wide scan.
- [A timeout can occur after Jellyfin accepted a request] → Persist attempt intent, serialize one request per target, cap retries, and rely on refresh being a convergent scan request; report bounded request acceptance rather than scan completion.
- [Generic Settings currently exposes stored values] → Redact the Jellyfin credential in both generic and typed Settings reads and ensure tests cover the HTTP response and serialized error paths.
- [A mapping can become stale after Jellyfin library reconfiguration] → Treat the returned ID as configuration error, surface the exact discovered IDs for correction, and never guess a replacement.
- [Server-wide fallback scans more media and can consume NAS resources] → Use it only when scoped scanning is not supported or established, coalesce all such requests per server, and disclose its scope.

## Migration Plan

1. Rebase after #54 is merged, then consume its stable EventID, PublicationID, and authoritative payload LibraryID contract.
2. Add the Jellyfin setting with integration disabled by default, no mappings, and no credential. Add the bounded delivery/outcome persistence with a database migration compatible with existing installations.
3. Do not backfill old `PublishedAt` records or historical/READY Test Encodes. Only new canonical events and qualifying Test Encode transitions/removals create outcomes.
4. Enable the integration only after the user configures a reachable server and explicit ID mappings. Rollback is to disable the integration; retain lifecycle records and bounded integration evidence without reversing publication, restoring deleted Test Encode output, or changing source media.
