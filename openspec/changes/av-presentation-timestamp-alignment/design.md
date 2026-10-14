## Context

See `proposal.md` for the reported false warning. The current persisted Validation path parses stream metadata from Queue source evidence and the canonical Result output probe. Its A/V calculation can fall back to `stream.start_time`, while the finding stores those input objects and omits the already computed per-track comparison report. A separate helper can collect packet PTS and presented-frame timestamps with an initial two-second ffprobe read interval, but persisted Validation does not currently use it.

Constraints: retain the frozen stream plan as the audio identity map; source and output evidence must belong to the same canonical Queue execution and Result; ValidationRuns remain immutable; Publisher keeps its existing outcome and eligibility rules; probing must stay read-only and bounded for NAS workloads.

## Goals / Non-Goals

**Goals:**

- Compare the same presentation anchor semantics on both sides: the first presented video frame and a presented audio timestamp or audio packet PTS.
- Keep stream-level start times as diagnostic evidence and fail closed as `UNVERIFIED` when presentation evidence cannot be compared.
- Persist an explanatory per-track report without changing the existing expected/actual evidence shape or Result identity.
- Reuse stored evidence and limit fallback probing to the initial two seconds of only the input whose evidence is missing.

**Non-Goals:**

- Change encoder or FFmpeg conversion arguments, QSV, GOP, B-frame behavior, Validation thresholds, or global policy.
- Prove full-file synchronization or ongoing drift; the finding measures initial presentation alignment only.
- Change `video.properties`, chapter findings, Publisher policy, historical ValidationRuns, or media files.

## Decisions

1. **Use presentation anchors only in the introduced-offset calculation.** Video uses the first presented frame timestamp, preferring `best_effort_timestamp_time`; audio uses its first presented timestamp or packet `pts_time`. The evidence records the exact timestamp field and whether it came from stored Queue/Result evidence or bounded ffprobe fallback. `stream.start_time` remains visible in the raw evidence but is never a fallback for either anchor. If a timestamp cannot be found, omit that offset and report `UNVERIFIED`.

2. **Preserve frozen identity pairing and current policy boundaries.** Continue selecting planned source/output audio tracks through the resolved frozen stream plan. Compute source and output audio-relative-to-video offsets, then subtract them to obtain the introduced offset. Keep configured pass/warning frame thresholds and the existing bounded time-base quantization allowance for the mismatch boundary; do not widen the pass boundary or policy.

3. **Reuse evidence and keep fallback work bounded.** Parse presentation fields if they are already present in the Queue source probe or Result output probe. For a side missing required anchors, run at most one read-only timing probe over the initial two-second interval, using the existing narrow stream, packet, and frame fields, with a five-second process timeout. Only probe a source with matching frozen Queue source evidence and an output whose bytes still match the canonical Result revision. Never rescan the full file, encode, or probe from Review Center/History list requests. A failed or incomplete fallback remains `UNVERIFIED`.

4. **Persist comparison details additively.** Keep `validationFinding.expected` as source evidence and `actual` as output evidence for compatibility. Add an optional details object only to the A/V alignment finding. It carries per-track source/output relative offsets, introduced milliseconds and frame equivalent, pass/warning and quantization tolerances, evidence provenance, and `validated`/`warning`/`mismatch`/`unverified` decision. The same report can remain in the aggregate Validation report. Existing finding severity and Publisher eligibility continue to follow the canonical rule state.

## Risks / Trade-offs

- **[Risk] Some files do not expose a decodable presented frame inside the bounded initial interval.** → Record `UNVERIFIED`; do not substitute stream metadata or fabricate a value.
- **[Risk] A fallback probe adds I/O and ffprobe work during Validation.** → Reuse existing evidence first, probe only sides missing anchors, inspect at most two seconds with a five-second timeout per probe, and never probe from list rendering.
- **[Risk] Consumers may have stored older finding shapes.** → Keep existing expected/actual values and add only an optional details field; missing details on historical runs remain valid.
- **[Risk] Initial alignment can be mistaken for full-file synchronization.** → Preserve `driftStatus: not_measured` and describe the scope as initial presentation alignment.

## Migration Plan

No database migration or artifact rewrite is needed. New Validation runs use the updated evidence and details. Completed ValidationRuns, their canonical Result bindings, and Publisher decisions remain immutable; rerunning Validation creates a new run against the same Result identity when that identity is still valid. Rollback consists of reverting the code while retaining already persisted findings, which remain readable because the added details are optional.
