## 1. Persisted candidate lifecycle

- [x] 1.1 Add durable active/completed/discarded status to QueueBatchPreflight and verify existing-row migration defaults with migration tests.
- [x] 1.2 Add revision-fenced transactional Remove and Discard APIs; reject terminal mutations and complete acceptance transitions without changing existing Edit/Resolve semantics.
- [x] 1.3 Add handler regressions for blocked and final-item removal, full and partial discard, stale/double submissions, restart persistence, new candidate creation, and unchanged QueueJobs, ExecutionPlans, reservations, and files.

## 2. Explicit frontend recovery and actions

- [x] 2.1 Extend preflight API methods and types for lifecycle status, Remove, and Discard; verify request payloads and response status handling.
- [x] 2.2 Separate saved candidate identity from dialog visibility in Assets; add explicit Resume and prevent stale GET responses/events from reopening or clearing a newer candidate.
- [x] 2.3 Add distinct Close, Discard candidate, and Discard edits controls plus Remove from candidate for each unqueued status; verify removal/discard feedback and revision-conflict recovery in dialog tests.
- [x] 2.4 Add frontend regressions for close/navigation/reload, explicit resume, partial acceptance, remove/discard, stale events and GET responses, and fresh Preflight after discard.

## 3. Architecture and validation

- [x] 3.1 Update the Queue Batch Preflight architecture guide to document persisted lifecycle states, explicit recovery, and accepted-work preservation.
- [x] 3.2 Run `openspec validate batch-preflight-candidate-lifecycle --strict` and `./scripts/verify.sh --auto`; resolve reported failures and confirm `ALL CHECKS PASSED`.
  - Validation (2026-10-10): OpenSpec strict reports `Change 'batch-preflight-candidate-lifecycle' is valid`; `./scripts/verify.sh --auto` reports `Mode: auto -> full`, `Checks: 6/6`, `Duration: 179s`, `ALL CHECKS PASSED`.
