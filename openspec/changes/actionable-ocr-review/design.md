## Context

See [proposal.md](proposal.md) for motivation and scope. The current
`reviewTranslationOCR` handler takes `canonicalOcrArtifactPath` from subtitle
eligibility, closes Translation, and starts the existing Analyze OCR operation.
The report is rendered under the sidecar list; the current integration test
checks that analysis is called for a path but does not assert a visible review.

The backend eligibility path currently picks one published OCR artifact by
media path and stream index, with format/path ordering. The frontend then
matches that artifact by path. Translation preparation has a stronger
canonical request check: source revision, stream index/ID/codec, requested
language, OCR mode, preparation scope, prepared mapping/representation, and
pipeline provenance. These two selection paths can therefore disagree about
which SRT/ASS sidecar is canonical for a blocked Translation request.

The existing OCR review flow already has published snapshots, sidecar
revision hashes, correction provenance, cue-only edits, operation status, and
an atomic publication barrier. The merged `Feat/262 ocr dictionary (#271)`
change is present in this checkout: Translation preparation already applies
validated contextual dictionary corrections before selective OCR and review.
This design keeps that behavior intact.

## Goals / Non-Goals

**Goals:**

- Make review visibly reachable from the exact blocked Translation operation.
- Preserve source selection and Translation settings through review and retry.
- Persist narrow approvals that remain valid only for the exact cue text and
  canonical source/artifact revisions the user reviewed.
- Reuse existing OCR snapshots, correction operations, serializers, and
  publication safeguards.

**Non-Goals:**

- Rebuild or change the #262 OCR dictionary, OCR engine, or Translation
  provider.
- Generate or apply AI corrections from this workflow. An AI suggestion may
  be displayed only when it is already supplied to the review report; #267
  remains separate.
- Add a second OCR review pipeline, job model, or publication path.

## Decisions

### Resolve review from the blocked Translation request

The failed Translation operation is the authority for the source selection,
requested source language, target locale, and output representation. Extend
its bounded OCR-review metadata with a target state and the exact canonical
request identity plus the matching published artifact identity, revision set,
format members, and logical SRT/ASS transform. The backend derives this from
the same canonical request used by Translation; the frontend does not rebuild
identity from a filename or select a sidecar from a nearby list row.

The shared resolver must match the current media revision, embedded stream
index/ID/codec, requested OCR language, mode, preparation scope and mapping,
prepared representation evidence, pipeline provenance, and current sidecar
revision. It may group SRT and ASS only when they share one logical transform
and equivalent canonical cues/provenance. Distinct transforms, stale members,
or conflicting evidence produce an explicit ambiguous/stale result instead
of format-order selection. Analyze and Correct revalidate the descriptor and
all revisions at request time and again before committing.

Source and artifact nanosecond revisions remain exact at the browser boundary:
the review API emits these `int64` fields as decimal strings, and the frontend
keeps them as strings when submitting the captured target. Canonical snapshot
storage retains its existing numeric representation. This prevents JavaScript
number rounding from changing an otherwise exact revision during review.

Use the existing Translation operation response and OCR analysis/correction
operations rather than adding a new OCR job or matching endpoint. The Review
OCR action remains available for an OCR-review blocker even when the target
state is missing or conflicting, so the visible review surface can explain
the problem and preserve Translation context. A stale descriptor requires a
refresh/re-analysis; it cannot silently retarget another sidecar.

**Alternative considered:** continue using `SubtitleEligibility`'s first
artifact and pass its path to Analyze OCR. This remains dependent on broad
path/stream matching and cannot distinguish OCR mode, requested language,
preparation scope, or multiple logical transforms, so it is rejected.

### Keep review visible and reuse the sidecar review experience

Open a dedicated OCR review dialog from the blocked Translation dialog. Keep
the selected source and submitted Translation options in the parent workflow;
returning from review restores the blocked Translation dialog and its retry
action. Reuse the existing cue report and correction controls inside the
dialog rather than adding a second inline report below Sidecars.

Key report state by the exact review target and baseline revision. Show
loading, no-findings, failure, cancellation, stale, and conflict states in the
dialog with a usable return or retry action. A successful report shows cue ID,
reason, original text, deterministic suggestion, an already-available AI
suggestion when present, an editable text value, and read-only timing and ASS
layout/position details. AI suggestions are display-only in this change.

**Alternative considered:** scroll to the Sidecars section and expand the
report there. This can leave the user with no visible feedback while analysis
runs and makes the selected logical sibling less apparent, so a dedicated
surface is preferred.

### Store human decisions in canonical OCR snapshot provenance

Extend the existing canonical OCR decision provenance with an explicit human
approval action. An approval records the canonical OCR identity/source
revision, published artifact revision set, logical transform, cue ID, exact
SHA-256 cue-text hash, the reviewed analyzer/repair contract versions, and
operation provenance. Do not create a second approval table or dictionary.
Existing snapshots without approval data remain valid and simply have no
human approvals.

Correct applies an already-validated suggestion or the user's text edit and
records the before/after text hashes. For changed text, commit the approval
against the new sidecar revision only after all requested serializers validate
and the existing atomic SRT/ASS publication succeeds. Accept as correct
records equal before/after hashes against the current revision without
rewriting sidecar bytes. Leave pending sends no decision. An approval-only
commit still locks and rechecks the complete current revision set before
updating the internal snapshot. Mixed decisions and text corrections use one
operation and one consistent canonical cue set.

Bound one decision request to the existing correction-provenance limit and
show the affected cue IDs/actions for confirmation before a batch. The
existing serializers remain responsible for SRT/ASS output: only cue text may
change; timing, cue order, ASS tags/styles, geometry, and positioning stay
from canonical evidence. A logical `both` text correction uses the current
all-output publication barrier.

**Alternative considered:** treat Accept as correct as the current `keep`
action. `keep` does not persist provenance, so Translation would rediscover
the same finding on retry. A separate approval store would duplicate snapshot
ownership and revision rules, so the decision belongs in canonical snapshot
provenance.

### Honor approvals inside canonical Translation preparation

Keep #262 dictionary corrections first and unchanged. After those corrections,
Translation preparation checks exact approvals before running selective OCR
for each remaining review finding. A match resolves only that cue for this
request; a missing or stale match follows the current selective-correction and
fail-closed review behavior. Final provider work starts only after every cue
is clean, safely corrected, or explicitly approved.

The match includes canonical source identity and revision, stream, requested
OCR language/mode, preparation scope, logical transform, current sidecar
revision, cue ID, text hash, and reviewed quality-contract versions. Do not
rewrite the analyzer result as clean or suppress future findings globally;
retain the original finding and attach its approval status. A source edit,
stream change, different OCR request, logical sibling conflict, changed cue
text, sidecar revision change, or analyzer/repair contract change requires
review again.

**Alternative considered:** skip the quality gate for an approved source or
stream. That would let one cue's approval hide different cues or later source
revisions, so approval is matched per cue and per exact revision.

## Risks / Trade-offs

- **[Risk]** Existing eligibility and Translation selection can disagree about
  SRT/ASS siblings. **Mitigation**: share one exact resolver and fail closed
  when revisions, logical transforms, or cue evidence conflict.
- **[Risk]** A human can approve text that the heuristic still flags.
  **Mitigation**: approval is explicit, cue-local, revision-bound, and does
  not suppress other findings or later analyzer versions.
- **[Risk]** Older internal snapshots have no human approval provenance.
  **Mitigation**: treat the missing field as no approval and request review;
  no database migration or source-sidecar rewrite is needed.
- **[Risk]** Approval-only and text-changing decisions could diverge from the
  published SRT/ASS members under concurrent edits. **Mitigation**: check the
  full revision set before committing and use the existing publication barrier
  for text changes.
- **[Risk]** Resolving an OCR blocker can trigger extra media/OCR work on a NAS.
  **Mitigation**: review reads the bounded published snapshot and never starts
  full OCR; retries reuse current canonical evidence and matching approvals.

## Migration Plan

1. Add optional decision fields to the existing internal snapshot format and
   extend the existing operation/report contracts. Older snapshots decode
   without approvals and remain reviewable.
2. Deploy backend and frontend together. If a server cannot provide a matching
   review target, the UI displays an actionable unavailable/stale state and
   keeps Translation context; it does not fall back to path guessing.
3. Rollback requires no media or database migration. An older backend cannot
   honor the new approval action, so users may need to review again after
   rollback; existing source and published sidecar bytes remain protected.

## Open Questions

None. The exact source/revision matching and the meaning of approval-only
publication are resolved by this design.
