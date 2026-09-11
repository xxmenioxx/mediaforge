# Codex Prompt — Queue Claim Freeze

Implement:

```text
docs/specs/003-queue-claim-freeze/spec.md
```

Use `plan.md` and `tasks.md`. Treat `spec.md` as source of truth.

Key constraints:

```text
QUEUE = lightweight intent
CLAIM/PREPARING = current config + required analysis + freeze
PROCESSING = frozen execution

- no deep preparation in commit:false/commit:true;
- claim must be atomic and short;
- expensive work starts after claim transaction ends;
- reuse valid ScanResult;
- Track Profile/ResolvedTrackPlan remain per asset;
- post-freeze execution is immutable;
- legacy frozen queued jobs remain compatible;
- stay on Assets/Library after queue;
- no per-asset scope dependency for Queue Selected;
- avoid Queue redesign if a scoped refactor is enough.
```

First inspect only these anchors:

```text
QueueSelectedAssets
resolveSelectedAssetsQueue
prepareBatchQueueJob
captureSupplementalProfiles
resolveTrackPlan
worker claim / next runnable job
Queue Selected frontend success/navigation
```

Before editing report:

```text
Current gap:
Smallest safe change:
Touch:
```

Then execute `tasks.md` in order. Follow `AGENTS.md`. Add focused tests only.

Validate:

```bash
./scripts/verify.sh --auto
```

Final report:

```text
Changed:
Behavior:
Validation:
Limitations:
```
