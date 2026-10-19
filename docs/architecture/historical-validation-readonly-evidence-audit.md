# Historical Validation — bounded read-only evidence audit

Status: **proposal only**. Related to the remaining historical recovery scope of closed issue #368. Not an authorization to access the NAS.

## Scope and guardrails

Target exactly three completed, unpublished Queue jobs previously identified privately as **#2214, #2237 and #2238**. Identifiers and media paths stay in private/operator notes; never paste original filenames, database contents, media fingerprints, credentials or private paths into the public repository, PR, issues, or CI logs.

This audit is **read-only** and requires an explicit separate approval before any NAS access. Do not deploy the new preview endpoint simply to run it; production deployment is independent. Do not copy the production DB, write temp tables, modify Queue/Result/Validation/Publisher, revalidate, repair sidecars, hash entire media outputs, re-encode, publish or start maintenance tasks. Stop if any required record, identity or archival evidence is absent. Use a dedicated read-only SQLite connection if/when local operator provides the canonical DB file path; do not guess the path.

No scripts in this document should be run remotely until approved. Any implementation must inspect the current SQLite schema via read-only introspection, parameterize values, bound reads to these jobs, cap outputs, sanitize exported diagnostics and **never place a query result in GitHub**.

## Audit hypotheses (not findings)

1. **Accepted QueueBatchPreflight candidate.** `ExecutionPlan.Evaluation["batchPreflightCandidateId"]` may identify a persisted `QueueBatchPreflight` record. That record stores JSON `State` and selected per-asset `Input`/`ResolvedJob`. Its accepted revision and original input might predate the contaminated freeze. However, the same preflight resolver may already have mutated shared ProfileData, and the record's final `State` is mutable across accept/update/discard. A matching candidate ID is *correlation*, not proof of historical user intent, unless a separately preserved immutable pre-mutation record and fingerprint exist.
2. **Historical per-asset explicit override.** An independently timestamped/versioned asset track selection or external operator request might prove specific keep/remove/extract intentions. Current overrides, current Asset Defaults, profile key alone and current scans are not immutable execution-time proof.
3. **Execution and result trail.** ResultIdentity, ExecutionBinding, active plan fingerprint, ResultArtifact and ValidationRun can establish that the same output/execution was evaluated; they do not establish what tracks were originally requested. AS-IS StreamPlan and Result.AssetConversion may derive from the contaminated frozen Queue snapshot.
4. **Sidecar readiness.** Result manifest artifact IDs and current sidecar existence/type can refute claims of completeness, but do not prove requiredness or original per-asset selections. The schema does not version sidecar content; no sidecar repair is authorized.
5. **Independent video FPS.** The third historical case has `video.properties UNVERIFIED`. Even with a provably corrected track contract, this remains a separate blocking investigation.

## Preflight inspection checklist (local operator; requires approval)

1. Establish exact read-only DB connection, NAS host, audit window, maximum query budget, and operator. Require application-native SQLite URI `mode=ro`, `PRAGMA query_only=ON`; **no `immutable=1` shortcut** on an actively used WAL database, as it may hide live WAL changes.
2. Confirm schemas of `queue_jobs`, `execution_plans`, `queue_batch_preflights`, `validation_runs` and related provenance tables with SELECT-only introspection. Do not assume table names on production; derive through model migrations/schema.
3. For each of three explicit job IDs, fetch only a sanitized **structural projection**: JobID, completion/published state, SourceAssetRecordID, active plan ID, execution number, active ValidationRun ID, profile/track snapshot provenance, and *presence* of relevant JSON fields. Do not emit raw paths or full snapshots to terminal logs.
4. Correlate plan ID/version/fingerprint and candidate ID (if any) to preflight record/revision and per-asset input state. Distinguish *original user input* from `ResolvedJob`/resolved candidate and from downstream worker/Result evidence; reject equality by path alone. Confirm timestamps/revisions and whether the proposed record could have been mutated after acceptance.
5. Locate canonical `kind=result` and AS-IS artifact references through existing backend report configuration, not by recursive NAS scans. Inspect just expected JSON metadata identities: artifact ID, generation token/version, output revision, frozen Queue fingerprint, ExecutionPlan ID/version/fingerprint, and current active ValidationRun binding. No media reads or rehashing.
6. Summarize previous ValidationRun findings for audio, embedded subtitles, attachments, sidecars/manifest, `video.properties` and independent failures. Record diagnostic required-vs-produced counts/IDs only after local redaction; never assume manifest entries prove original requiredness.
7. For each intended audio/subtitle/sidecar field, assign **PROVEN_INDEPENDENT**, **CANDIDATE_UNVERIFIED**, **NEEDS_EVIDENCE**, or **BLOCKED_IDENTITY**, citing the origin and whether it was immutable before Queue freeze. No inferred corrected contract; no SAFE without all relevant fields independently proved.
8. Stop immediately on result generation mismatch, changed output revision, inconsistent execution/plan binding, missing immutable intent, or ambiguous preflight record. Do not promote status, create ValidationRun or authorize Publisher.

## Synthetic/local query template — do not paste production results into GitHub

Use **only an offline synthetic fixture** for query development; real database path and job IDs must be provided by operator after separate approval. Avoid wide `SELECT *`, `json_extract` returning raw sensitive payloads, unconstrained folder globs, or recursive filesystem operations. When the schema is confirmed, use a prepared equivalent of:

```sql
SELECT id, status, stage, source_asset_record_id,
       execution_number, active_execution_plan_id,
       validation_run_id, published_at
FROM queue_jobs
WHERE id IN (?, ?, ?)
LIMIT 3;
```

Further selects may inspect `execution_plans.evaluation` **only as in-process JSON** for `batchPreflightCandidateId`, and select a single corresponding candidate `queue_batch_preflights` record by that ID, if present. Aggregate only sanitized provenance summaries; never output raw `state` containing file paths. Retrieve at most the bounded active and previous ValidationRun records for each job; preserve all audit originals.

## Per-job outcome matrix

| Historical job | Suspicious derived expectation | Independent evidence necessary | Current classification |
| --- | --- | --- | --- |
| #2214 | Audio membership mismatch from another asset's frozen snapshot | Original per-asset audio stream IDs/actions plus versioned source-selection provenance | NEEDS_EVIDENCE |
| #2237 | Crossed audio/subtitle actions and inflated sidecar/manifest expectations | Independently preserved per-asset audio and subtitle stream actions, output formats and required artifact IDs | NEEDS_EVIDENCE |
| #2238 | Crossed subtitle/sidecar expectations; separate FPS uncertainty | Per-asset subtitle/sidecar contract **and** independent cadence/FPS evidence for video.properties | NEEDS_EVIDENCE |

## Exit criteria / next decision

Produce a **private** redacted matrix of field-level proof with 3 statuses: independently proven, candidate-only, unavailable. Do not leak paths or file hashes. If no independent pre-freeze intent exists, recovery by historical Validation reconciliation is **not safe**; discuss a separately authorized fresh execution or a product-level provenance-capture improvement for future jobs. If sufficient independent evidence exists, design an append-only operator-approved correction and a Publisher-compatible read-only preview in a separate phase; ask for separate commit and deploy approval. No silent write to production.
