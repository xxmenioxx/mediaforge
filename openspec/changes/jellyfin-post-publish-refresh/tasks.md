## 1. Publisher dependency gate

- [ ] 1.1 Wait for #54 to merge, rebase `feat/56-jellyfin-refresh` onto current `main`, and inspect the final `AssetPublished` outbox contract; verify it supplies a stable event identity and authoritative MVForge Library ID, otherwise resolve that contract with #54 before implementing the canonical trigger.

## 2. Settings and bounded persistence

- [ ] 2.1 Add typed Jellyfin settings with disabled-by-default behavior, URL and refresh options, bounded debounce validation, and MVForge Library ID to Jellyfin Library ID mappings; verify defaults, invalid mappings, stable-ID behavior, and legacy settings compatibility with focused tests.
- [ ] 2.2 Add write-only credential update semantics and redact the credential from generic and typed Settings reads, responses, logs, History, diagnostics, and retry evidence; verify configured-state responses and secret redaction with focused handler tests.
- [ ] 2.3 Add the single Jellyfin delivery/outcome ledger and migration for deduplication, pending work, retry timing, and bounded outcomes; verify migration against an existing database shape and uniqueness for canonical and Test Encode source keys.

## 3. Jellyfin client and target refresh

- [ ] 3.1 Implement one bounded Jellyfin client for reachability, authentication, optional server-version retrieval, and media-library discovery; verify successful, unavailable-version, invalid-credential, and timeout behavior with a fake HTTP server.
- [ ] 3.2 Implement capability-aware scoped refresh for a mapped library and the explicitly broader server-wide scan fallback; verify discovered IDs are used exactly, stale IDs fail as configuration errors, fallback scope is exposed, and no paths or names are inferred.

## 4. Shared refresh coordinator

- [ ] 4.1 Implement persisted deduplication and per-effective-target debounce with a 10-second default, 60-second setting cap, and 60-second maximum wait; verify publication and Test Encode requests coalesce, distinct scoped targets debounce independently, duplicate observations do not scan repeatedly, and pending work resumes after restart.
- [ ] 4.2 Implement the shared bounded timeout, retry classification, backoff, and explicit retry behavior; verify transient errors retry within the five-attempt limit, permanent configuration errors stop automatic retries, and retry evidence contains only sanitized classifications.

## 5. Lifecycle trigger adapters

- [ ] 5.1 Subscribe only to the final #54 `AssetPublished` event after publication and archive outcome recording; verify Worker, Validation, staging, `PUBLISHING`, partial/failed publication, and legacy `PublishedAt` do not trigger, duplicate delivery deduplicates, and refresh failure leaves `PUBLISHED` and its evidence unchanged.
- [ ] 5.2 Notify the coordinator only after Test Encode media and required sidecars are activated, output probing/validation succeeds, and `READY` persistence succeeds; verify LibraryID mapping, no path/name inference, failed/canceled/incomplete cases do not trigger, Test Encode emits no `AssetPublished`, and Jellyfin failure preserves READY, Keep, and source state.
- [ ] 5.3 Notify removal only after successful physical removal of a previously READY output or registered sidecars from explicit delete, expiration, or housekeeping; verify Keep=true, dry-run, no-op cleanup, cleanup failure, and non-READY cleanup do not report a successful refresh.

## 6. Settings and lifecycle status UI

- [ ] 6.1 Add a compact Settings surface for enablement, URL, configured-credential state, test connection, stable-ID mappings, debounce, last bounded outcome, and optional manual refresh; verify discovery names remain presentation-only and the credential is never returned to the browser.
- [ ] 6.2 Show concise Jellyfin outcome status/time and eligible retry actions in Publisher/History and Test Encode views; verify Jellyfin status does not replace PUBLISHED, READY, Keep, Queue, or Validation state.

## 7. Focused integration verification

- [ ] 7.1 Add focused regression coverage for disabled and unmapped settings, separate reason values, scoped and broad refresh behavior, stale IDs, invalid credentials, transient outage, secret redaction, same-target cross-trigger coalescing, independent target debounce, and lifecycle failure isolation; verify all requested cases pass without real Jellyfin or media.
- [ ] 7.2 Run the canonical `./scripts/verify.sh --auto` after implementation and resolve only its reported failures; verify the output ends in `ALL CHECKS PASSED` before the implementation commit.
