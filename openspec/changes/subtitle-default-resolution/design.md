## Context

See proposal.md for the motivation. The existing backend already has an
authoritative Track Profile resolver and resolved track plan, Asset Defaults
for Tracks, Subtitle Settings for ASS fallback and styling, and Queue snapshots.
Asset Info extraction currently builds its own request plan and can receive
client-supplied OCR language/quality even when those values are only display
heuristics. The subtitles delta defines the observable contract.

## Goals / Non-Goals

**Goals:**

- Make the existing canonical Track Profile/subtitle plan the sole authority
  for effective format and per-stream OCR decisions across Asset Info,
  Preview, Test Encode, and Queue.
- Preserve field-level presence so a supplied value overrides only that field,
  while every omitted field continues to inherit.
- Reuse current Asset Defaults, Track Profiles, Subtitle Settings,
  ResolvedTrackPlan, styling policy, and OCR lifecycle.
- Make the effective settings snapshot inspectable in backend planning and
  operation state with the origin of each resolved field.

**Non-Goals:**

- Add Settings keys, profile fields, migrations, OCR modes, language inference,
  subtitle-selection policy, or styling features.
- Change #114 preferred-language matching, translation source/locale/provider
  ownership, OCR cache identity/evidence rules, or subtitle publication
  behavior.
- Redesign Asset Info controls or change AssetsPage.tsx in this planning phase.

## Decisions

### Extend the existing canonical resolution path

Asset Info will resolve the exact selected stream(s) and operation intent
through the same backend asset/profile resolution and canonical subtitle
planner used by Preview, Test Encode, and Queue. The planner will accept
operation overrides as presence-aware input and will return effective
per-stream settings alongside the existing resolved artifact decisions.

Do not add an Asset Info-only resolver or duplicate profile selection. Resolve
asset/path/logical-group/source-group assignments and Track Profile inheritance
first, then apply the existing global Settings sources, then built-in values.
The established scope precedence within asset/profile resolution remains
authoritative.

### Effective settings input and output

The resolver input consists of:

1. Existing source/stream facts and exact Asset Info operation intent.
2. The already-resolved asset/profile fields, preserving inherited,
   explicit-value, and explicit-none states.
3. Existing global Settings snapshots: assetDefaults.tracks (including a
   referenced Tracks Profile when selected) and subtitleSettings (SRT
   fallback and styling policy).
4. Presence-aware operation overrides for supported request fields.

The effective output consists of the existing canonical per-stream artifact
decisions plus a frozen effective-settings record: output format/mode, OCR
language and quality when applicable, whether SRT fallback added an output,
the applicable styling/positioning policy, and the source of each resolved
field. The plan remains responsible for stream eligibility, source codec,
artifact identity, stages, naming, validation, and publication.

Resolution order is field-by-field:

| Field | Asset/profile/global source | Built-in fallback |
| --- | --- | --- |
| Output format | Explicit operation; resolved Track Profile/asset decision; existing Tracks Asset Defaults format | SRT for converted requests; original extraction only when explicitly requested |
| OCR quality | Explicit operation; resolved stream rule/profile; existing Tracks Asset Defaults ocrMode | accurate |
| OCR language | Explicit operation; resolved stream rule/profile | Existing automatic/source-tag inference; retain bounded Japanese orientation fallback. No global OCR-language setting is added. |
| SRT fallback | Existing subtitleSettings.generateSRTFallbackForASS policy, after effective format selection | Disabled when missing, as today |
| ASS styling/positioning | Supported explicit restyle override gated by current policy; resolved styling snapshot; existing subtitleSettings.subtitleStyling | Existing built-in style policy, including disabled styling and current geometry behavior |

Track selection continues to apply #114 preferred-language rules through the
existing Track Profile authority. Those rules do not become an OCR-language
setting and do not change the source-language evidence used by OCR.

### Preserve optional request presence

At the request boundary, omitted fields remain absent. A supported supplied
field is an explicit operation override; auto is an explicit OCR-language
choice when the user selects it. An explicit boolean false must remain
distinguishable from an omitted boolean wherever a currently supported
operation exposes that choice. Existing validation remains responsible for
rejecting unsupported values.

Client display suggestions are not resolver input. The client may display
source-language or quality suggestions, but it must omit those fields until
the user changes the relevant control. This requires a later, narrowly scoped
request-construction update; the visual controls and their layout are outside
this change.

### Keep SRT fallback and generated styling downstream of format resolution

Resolve one requested format first, then apply the existing ASS-to-SRT fallback
eligibility and logical both normalization. An explicit SRT-only operation
remains SRT-only. Native ASS/SSA original extraction remains a distinct mode;
the existing eligible original-plus-SRT fallback remains separate from
converted logical both.

Pass the resolved styling policy to existing serializers only where styling
is supported. Keep SRT as canonical text/timing and preserve the existing ASS
geometry, role-aware positioning, source-style, and font behavior.

### Freeze effective inputs at existing lifecycle boundaries

Asset Info resolves and captures effective settings before extraction/OCR
starts. Its cancellation and failure states stay terminal; retry creates a
fresh operation identity from the same captured request intent and effective
settings. Reuse valid canonical OCR evidence only through the current
identity/provenance validation, and only when all existing cache identity
dimensions still match.

Queue resolves current Settings only before the existing immutable
PREPARING-to-PROCESSING freeze. Workers and retries consume the frozen plan.
Legacy snapshots that lack newly represented fields retain their stored
profile/plan semantics and are never silently re-resolved from current global
Settings.

Translation remains an independent subtitle operation owner. It continues to
resolve exact sources, locale, provider, canonical cues, and translated output
using its existing contract; effective extraction defaults do not rewrite
translation requests.

### Alternatives considered

- **Resolve defaults in the frontend:** rejected because it duplicates policy,
  turns display heuristics into explicit values, and cannot guarantee parity
  with Queue/Profile planning.
- **Add an Asset Info subtitle settings resolver:** rejected because it would
  create a second policy authority and keep the current parity gap.
- **Add OCR language/quality Settings:** rejected because the task forbids new
  settings and the existing profile plus automatic/source-language behavior
  already define those inputs.
- **Re-resolve legacy Queue jobs on retry:** rejected because it would let
  current settings relabel a frozen execution.

## Risks / Trade-offs

- [Existing clients send heuristic values as if explicit] → Update request
  construction to omit untouched suggestions; retain explicit user changes.
- [Asset Info format omission changes from its legacy endpoint fallback] →
  Keep original explicit and document the new omitted-field inheritance;
  preserve the existing converted SRT fallback when no configured format
  resolves.
- [Settings change between preview and execution] → Treat preview as current
  resolution and freeze Queue decisions at the established preparation
  boundary.
- [Cache mismatch after effective input resolution changes] → Continue to use
  canonical OCR request identities and evidence validation; do not migrate or
  bypass cached evidence.
- [Large Settings maps increase Queue snapshot size] → Snapshot only the
  resolved subtitle decisions and policy already needed for execution, not
  unrelated Settings.

## Migration Plan

No database migration or Settings rewrite is required. New Asset Info requests
use presence-aware optional fields and resolve omitted values from current
settings. Existing Track Profiles and Asset Defaults remain valid; missing
fields inherit through the defined precedence and then use their current
built-in behavior. Existing frozen Queue snapshots, including snapshots
without newly represented effective-settings metadata, remain authoritative.
Rollback is a code rollback: stored Settings and snapshots remain readable,
and no media or subtitle artifact is rewritten by migration.

## Open Questions

None. The supported settings sources, precedence, lifecycle boundaries, and
compatibility behavior are defined by the spec and this design.
