## 1. Runtime state model and migration

- [ ] 1.1 Add the dedicated singleton `RuntimeState` model/table with explicit logical identity, current detection fields, stable fingerprint/version, and indexes; verify a temporary SQLite database can create exactly one host state and cannot create a second singleton.
- [ ] 1.2 Add immutable snapshot capture metadata needed for stable fingerprint, capture reason, and provenance without mutating legacy rows; verify existing `RuntimeSnapshot`, `ExecutionPlan.RuntimeSnapshotID`, and `TestEncode.RuntimeSnapshotID` schemas remain readable.
- [ ] 1.3 Add bounded legacy initialization that copies the newest existing snapshot into `RuntimeState` ordered by detection time then ID, computes fingerprint v1, and creates no synthetic audit row; verify referenced legacy IDs and all legacy rows survive migration.

## 2. Detection and stable fingerprint

- [ ] 2.1 Refactor detector output into current-state detection plus atomic singleton upsert; verify first detection creates one current row and repeated detection updates that row without appending snapshots.
- [ ] 2.2 Implement canonical `RuntimeStableFingerprint` v1 with normalized stable OS/host, memory, GPU, encoder, profile/policy/override, and disk-role payloads; verify identical values with different JSON/map ordering produce identical hashes and timestamps/IDs are excluded.
- [ ] 2.3 Add volatile-field regressions for CPU load, available RAM, GPU utilization, battery/power state, and free disk bytes; verify current telemetry changes while the stable fingerprint and immutable snapshot count remain unchanged.
- [ ] 2.4 Add stable-transition handling inside the state write transaction; verify encoder, GPU, runtime-policy, profile, and disk role/type changes create exactly one `stable_transition` snapshot per transition, equivalent writes do not duplicate it, and A → B → A is supported.
- [ ] 2.5 Migrate startup, periodic detector, runtime-policy update, and capability-cache refresh paths to the new detection/upsert boundary; verify same-fingerprint restart/startup and policy writes do not append history.

## 3. Explicit runtime APIs and current-state consumers

- [ ] 3.1 Introduce explicit current-state, latest-audit, and audit-capture service boundaries and remove ambiguous live/history use of `runtimeinfo.Latest`; verify every known current-state caller reads `RuntimeState` and only audit workflows read immutable snapshots.
- [ ] 3.2 Update GET `/api/system/runtime` and POST `/api/system/runtime/refresh` to return live state with existing telemetry field meanings plus explicit state kind, detection time, fingerprint, and optional audit provenance; verify GET and repeated refresh do not create snapshots and refresh failures leave no partial state.
- [ ] 3.3 Preserve API client compatibility for existing telemetry consumers while making current-state technical identity distinct from optional audit snapshot ID; verify JSON decoding of legacy fields and omission/nullability behavior for live provenance.
- [ ] 3.4 Update Dashboard and Settings runtime labels and refresh success/error messages; verify live refresh shows detection time without a fabricated `Host snapshot #` or “snapshot recorded” claim, while explicit immutable evidence can still display its snapshot ID.

## 4. ExecutionPlan provenance and resource reevaluation

- [ ] 4.1 Add the material plan-freeze transaction that captures `execution_plan_freeze` evidence and binds `ExecutionPlan.RuntimeSnapshotID` atomically with the frozen runtime profile/configuration; verify a plan reference remains valid after current state changes.
- [ ] 4.2 Change resource/waiting reevaluation to use current-state dynamic telemetry without changing frozen runtime profile or `RuntimeSnapshotID`; verify repeated WAITING reevaluations do not create unbounded snapshots or replace plan provenance.
- [ ] 4.3 Preserve bounded final dynamic resource evidence in the plan's structured decision data when it materially affects dispatch; verify free RAM/free disk/battery/worker changes remain explainable without replacing the stable snapshot ID.
- [ ] 4.4 Add a compatibility path for pre-existing plans with no runtime snapshot reference that binds only at their next explicit material freeze; verify ordinary reevaluation alone does not become a capture boundary.

## 5. Test Encode provenance

- [ ] 5.1 Capture `test_encode_freeze` evidence during Test Encode preparation after effective configuration resolution and bind it atomically before FFmpeg starts; verify the Test Encode never stores the mutable current-state row ID as `RuntimeSnapshotID`.
- [ ] 5.2 Fail Test Encode preparation safely when immutable capture/binding fails; verify no encoding process starts without a valid immutable reference and retry/new Test Encode captures independently.
- [ ] 5.3 Verify current-state refreshes after Test Encode preparation do not change its runtime snapshot reference and that completed/retained Test Encodes expose explainable immutable provenance.

## 6. Reference discovery and retention

- [ ] 6.1 Implement one canonical referenced-ID discovery boundary covering all Execution Plan and Test Encode references and an extension point for future provenance owners; verify references are found regardless of lifecycle status and without ad-hoc deletion queries.
- [ ] 6.2 Implement deterministic retention resolution for always-referenced rows, newest immutable row, seven-day recent unreferenced rows, 256-row soft pressure, and oldest-first ordering; verify referenced, newest, recent, and old-unreferenced cases match the policy.
- [ ] 6.3 Implement bounded deletion batches of at most 100 eligible rows and recheck references/latest-row protection inside the deletion transaction; verify a reference added after preview prevents deletion and large legacy history is processed over bounded runs.
- [ ] 6.4 Add SQLite transaction/concurrency regressions for capture plus plan/Test Encode binding racing housekeeping; verify no newly referenced snapshot is deleted and no provenance row is left pointing at a deleted snapshot.

## 7. Housekeeping and reporting

- [ ] 7.1 Integrate runtime snapshot pruning into existing `RunHousekeeping` and `StartAutoHousekeeper` ownership without adding a scheduler or request-triggered cleanup; verify the existing workspace and Test Encode cleanup behavior remains intact.
- [ ] 7.2 Extend the housekeeping report backward-compatibly with structured runtime rows-considered, referenced-kept, latest-kept, recent-kept, candidates, deleted, and error counts; verify database rows are not fake filesystem candidates and `recoveredBytes` changes only for actual files.
- [ ] 7.3 Verify dry-run preview resolves and reports advisory runtime candidates but deletes no rows, while a normal run deletes only the bounded transactionally rechecked batch.

## 8. Focused regression and repository validation

- [ ] 8.1 Add SQLite/fake-detector coverage for first/repeated detection, Dashboard-style 30-second refresh, startup restart, volatile telemetry, deterministic fingerprinting, stable transitions, and no automatic `VACUUM` from detection/API/startup/housekeeping.
- [ ] 8.2 Add plan/Test Encode coverage for one-time immutable capture, unchanged provenance after current-state changes, resource reevaluation behavior, and retention survival of both reference types.
- [ ] 8.3 Add migration, retention, bounded-batch, dry-run, report, API compatibility, and Dashboard/Settings label coverage using temporary databases and no NAS/hardware dependency.
- [ ] 8.4 Run focused backend/frontend checks for the changed surfaces, then run `openspec validate runtime-state-snapshot-retention --type change --strict` and `./scripts/verify.sh --auto`; mark the OpenSpec tasks complete only after all required checks pass.
