# #368 Phase A — safe preview only

- `GET /api/validation/jobs/:id/reconciliation-preview` reads existing job and canonical Result evidence.
- BLOCKED for unbound Results, non-completed/published jobs and normally bound same-asset snapshots.
- NEEDS_EVIDENCE for crossed snapshots, missing historical original intent, and missing Result. This endpoint never reports SAFE because independent original per-asset intent is not established by the currently frozen schema.
- OutputProbe counts and frozen snapshot counts are diagnostics ONLY. Never infer expected audio/subtitle/sidecar decisions from observed output.
- No database writes, ffprobe, encoding, sidecar repairs, active ValidationRun changes or Publisher handoff.
- No full-file identity hashing on GET; **not a replacement** for the future operator-approved fully revalidated commit.
- The historical patterns for wrong audio counts, wrong subtitle counts and inflated sidecar/manifest counts remain NEEDS_EVIDENCE; unrelated video.properties UNVERIFIED is not cleared.
- End-to-end producer/sidecar evidence and code-supported SAFE must be designed separately, following the OpenSpec authority audit.
