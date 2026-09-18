## Context

The existing subtitle contract keeps stream index, codec, language, disposition,
and subtitle actions in the canonical track plan. `resolveTrackPlan` is already
the shared resolver used by profile previews, Asset Info-related planning, and
Queue preparation; Queue stores a serialized resolved plan for Worker execution.
The current resolved subtitle track and sidecar records do not carry a semantic
role, provenance, or confidence. Existing canonical text/OCR conversion,
geometry-aware ASS rendering, native ASS/SSA extraction, operation lifecycle,
and publication barriers are already authoritative and remain unchanged.

## Goals / Non-Goals

**Goals:**

- Add a deterministic, player-agnostic role result and evidence contract for
  every subtitle stream.
- Keep stream index authoritative while making roles independently observable,
  overridable, filterable, and frozen with Queue execution decisions.
- Make classification bounded and NAS-safe, with explicit provenance and
  conservative ambiguity handling.
- Carry the role through canonical planning and publishing without creating a
  second subtitle resolver or changing subtitle conversion semantics.

**Non-Goals:**

- Direct Play policy, translation, OCR quality repair, regional language
  inference, a new OCR/planner/job system, or the Subtitles dashboard.
- Reclassifying a stream during Worker execution or changing an already frozen
  Queue plan.
- Treating a player-specific sidecar/container convention as the canonical role
  model.

## Decisions

### 1. Use a singular canonical role with explicit evidence

Represent the role as a closed value set: `full`, `forced`, `signs`,
`commentary`, `sdh`, and `unknown`. Keep it separate from language, codec,
stream index, and the original boolean dispositions. Add role provenance and a
confidence value to the resolved stream and relevant artifact metadata. The
provenance vocabulary should distinguish at least `manual`, `metadata`,
`disposition`, `heuristic`, and `ambiguous/unknown`; confidence should be a
small stable vocabulary such as `high`, `medium`, and `low` rather than an
unstable model score.

Use this precedence order:

1. valid per-asset/per-stream manual override;
2. explicit semantic role metadata already present in the stream snapshot;
3. explicit dispositions that map unambiguously to one supported role;
4. bounded title/order/codec and already-available track-size, cue-count, or
   active-duration evidence;
5. `unknown` when the evidence conflicts, is insufficient, or cannot be
   validated.

Multiple same-level signals that assert incompatible singular roles are
ambiguous and resolve to `unknown`; they are not resolved by arbitrary stream
order. `forced` and `sdh` derive from their respective explicit signals, while
`signs` requires signs-specific evidence and never inherits `forced`. A
`full` result requires explicit ordinary/full evidence; absence of a special
disposition alone is not enough to manufacture certainty. This makes the
classifier conservative while still allowing an explicit full-role marker.

The classifier consumes the existing scan/profile snapshot only. It must not
extract subtitles, invoke OCR, or perform an unbounded media read merely to
decide a role. If a bounded evidence source is unavailable, the result is
`unknown` with a reason rather than a guessed role.

### 2. Extend the existing canonical plan and subtitle rule model, not the
planner topology

Add role, provenance, confidence, and manual-override state to the existing
resolved subtitle stream shape and to sidecar artifact metadata. Keep
`resolveTrackPlan` as the only place that combines stream evidence, profile
rules, role overrides, and role matching. Existing language-only selection
remains the default path.

Extend each existing `subtitleRules` entry rather than adding a parallel global
role filter. A rule may contain `language?`, `streamIndex?`, `roles?`, and its
existing `action`, `sidecarFormats`, `ocrLanguage`, and `ocrMode` fields. An
absent `roles` selector is a wildcard. The role predicate is evaluated together
with the existing language and stream predicates, so a profile can express
policies such as `spa + full -> keep`, `spa + forced/signs -> keep`,
`spa + commentary -> remove`, and `eng + full -> keep`.

Preserve the existing rule precedence: a matching stream-specific rule wins
over language/path-safe rules, and role matching only narrows whether a rule
matches; it must not reorder rules or merge same-language streams. A configured
role list containing `unknown` is distinct from an absent role selector and an
empty/invalid selector fails validation rather than becoming a wildcard. Older
profile payloads without `roles` remain valid and retain their current
language-only behavior.

### 3. Keep overrides scoped to the source snapshot and stream identity

Store manual role overrides under the existing per-asset configuration boundary,
keyed by authoritative stream index plus the source/asset snapshot identity and
a stable signature of the relevant stream metadata. The binding should include
the source snapshot/fingerprint and normalized stream identity evidence such as
index, codec, language, title, order, and dispositions. Do not key an override
only by language, title, ordinal position, or stream index. If the source
snapshot or relevant stream signature changes, the override is stale and MUST
fail closed: it is ignored, never moved to another stream, and automatic role
classification is used instead. Asset Info displays automatic, manual, or
stale/ignored provenance and the reset-to-auto action.

Reset deletes only the selected stream override and invokes the same canonical
resolver against the current snapshot. It does not mutate source media,
existing sidecars, or unrelated stream overrides. A stale override may be
retained for review/audit, but it cannot affect selection or publishing until a
new override is explicitly bound to the current snapshot and stream signature.

### 4. Freeze role decisions at the existing Queue boundary

During Queue preparation/freeze, serialize the complete resolved role decision
with the existing `ResolvedTrackPlan` snapshot, including stream index,
override state, provenance, confidence, and the evidence version used for
classification. Workers deserialize and consume that plan for selection,
artifact metadata, and publishing. They do not re-read mutable profiles or
rerun role heuristics. A job prepared before a later metadata/profile change
therefore retains the role it was frozen with, consistent with other Queue
decisions.

### 5. Propagate role metadata through artifacts and required publication
adapters

The canonical artifact records retain the source stream index and resolved role
for original and converted sidecars. For supported external subtitle sidecars,
publication MUST insert `.forced` for role `forced` and `.sdh` for role `sdh` in
the established sidecar naming position before the subtitle extension. No
`.signs` suffix is invented by this change. For supported muxed/container
outputs, publication MUST map `forced` to the forced disposition,
`commentary` to the commentary disposition, and `sdh` to the hearing-impaired
disposition when those target dispositions are available. Original source
disposition flags remain preserved independently of the canonical role.

The adapter records the canonical role as metadata and uses no target
convention to reclassify it. Targets without these supported conventions keep
existing naming/metadata behavior while retaining the canonical role in the
internal artifact/provenance record.

Role metadata is descriptive to the conversion pipeline: it does not alter OCR language, OCR lifecycle,
subtitle text/timing, bitmap geometry, ASS alignment, native ASS/SSA stream
copy, cancellation, atomic `both` publication, or sidecar identity unless an
explicit role-aware publication mapping is configured.

### 6. Keep classification deterministic and cheap on NAS storage

Use normalized snapshot fields and bounded strings/integers already available
from probing. If the implementation needs cue count, active duration, or track
size, it may consume a bounded cached snapshot value or a bounded subtitle
representation already produced by the existing scan path; it must not hash a
complete MKV/BD asset or repeatedly invoke external tools per cue, serializer,
or Worker. Include a classifier contract/version in resolved metadata so
heuristic changes make the evidence auditable and can invalidate stale role
results safely.

## Risks / Trade-offs

- [Conservative classification] Many real-world tracks omit role metadata →
  return `unknown` with evidence instead of silently selecting the wrong track;
  allow explicit overrides and explicit `unknown` filters.
- [Conflicting dispositions] A stream can carry multiple relevant flags →
  preserve all original flags and return `unknown` when they cannot represent
  one role deterministically.
- [Profile compatibility] Adding an optional filter can accidentally change
  old profiles → treat absent role configuration as no role predicate and add
  legacy payload regression tests.
- [Stale role metadata] Snapshot changes can invalidate automatic decisions →
  bind automatic provenance to the snapshot/evidence version and recompute
  only through the canonical resolver; keep manual overrides stream-indexed.
- [Publishing convention drift] Player-specific forced/SDH conventions differ
  → require the canonical `.forced`/`.sdh` sidecar and supported container
  mappings only where the destination contract supports them, and preserve the
  canonical role when no supported mapping exists.
- [NAS cost] Extra probes could duplicate media work → prefer existing snapshot
  evidence, bounded reads, and cached classification inputs; never invoke OCR
  for role resolution.

## Migration Plan

1. Add optional role fields and role-filter/override payloads with backward-
   compatible defaults; existing profiles resolve exactly as before.
2. Persist role decisions in newly resolved plans and Queue snapshots while
   accepting older snapshots without role fields as `unknown`/legacy automatic
   metadata until their normal refresh boundary.
3. Enable Asset Info display and manual reset/override controls after backend
   resolution is available.
4. Add only supported publication mappings, leaving existing sidecar names and
   artifact identity unchanged where no explicit mapping is configured.

Rollback is safe by disabling role filtering and publication mappings. Existing
language-only profile data and source media remain valid; role fields may be
ignored by older readers without changing subtitle conversion behavior.
