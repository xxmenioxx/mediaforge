## Why

MVForge can publish media or place temporary Test Encode outputs in a configured Library without asking Jellyfin to refresh its catalog, so newly available media and removed test outputs can remain stale in Jellyfin. This change adds an optional integration while keeping canonical publication and Test Encode as separate lifecycle authorities.

## What Changes

- Add Jellyfin connection settings, stable MVForge Library ID to Jellyfin Library ID mappings, bounded library discovery and connection testing, and a small status surface.
- Subscribe to the canonical `AssetPublished` event after #54 is merged; never infer publication from Worker, Validation, staging, partial publication, or legacy timestamps.
- Notify the same integration coordinator only after Test Encode output and required sidecars are activated, output validation succeeds, and the Test Encode reaches `READY`; notify removal only after successful physical cleanup of a previously `READY` output.
- Debounce and retry refresh requests by effective Jellyfin library target, persisting bounded outcomes without changing publication or Test Encode lifecycle evidence.
- Keep Test Encode outside Publisher, `AssetPublished`, and #53 ValidationRun publication authorization.

## Capabilities

### New Capabilities

- `jellyfin-post-publish-refresh`: Optional Jellyfin refresh integration for canonical publication and consumable Test Encode additions/removals in configured MVForge Libraries.

### Modified Capabilities

None.

## Impact

- Backend: Settings/provider conventions, Jellyfin client and shared refresh coordinator, bounded integration outcome persistence, the post-#54 `AssetPublished` subscriber, and Test Encode READY/removal hooks.
- Frontend: a compact Jellyfin settings panel and concise Publisher/History and Test Encode integration status.
- Dependency: canonical publication work cannot be implemented until #54 is merged and the final `AssetPublished` outbox/subscriber contract is adopted.
- #55 retains ownership of Prometheus/Grafana implementation; this change exposes bounded outcomes only.
- Out of scope: metadata/poster management, Jellyfin database or file changes, Plex, source-media changes, new authentication infrastructure, and treating Test Encode as publication.
