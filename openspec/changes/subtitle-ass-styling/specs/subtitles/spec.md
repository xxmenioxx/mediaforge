## ADDED Requirements

### Requirement: Subtitle style presets have stable identity and bounded definitions

The system SHALL expose a reusable `SubtitleStylePreset` concept with a stable
ID, display name, primary color, outline color, shadow/back color, opacity, font
family, font size or auto sizing, bold state, outline width, shadow depth,
alignment behavior, and built-in/custom provenance. Stable IDs SHALL be the
identity used by resolution and persistence; display names SHALL be labels only.
The built-in IDs SHALL be immutable and SHALL initially resolve as follows:

| ID | Initial appearance and intent |
| --- | --- |
| `MVF_Default` | white dialogue, normal weight, bottom alignment |
| `MVF_Top` | yellow upper-note appearance and top alignment |
| `MVF_Lyrics` | cyan lyrics appearance |
| `MVF_Sign` | light-green signs appearance |

Users MAY customize the appearance stored for a built-in ID, but SHALL NOT
rename, delete, or change the identity of that ID. Custom presets SHALL receive
a backend-generated collision-safe stable ID independent of their display name.

#### Scenario: Built-in identity survives an appearance edit

- **WHEN** a user changes the colors of `MVF_Default`
- **THEN** future resolution uses the edited appearance while the resolved style
  ID remains `MVF_Default` and references to that ID remain valid

#### Scenario: Custom names do not define identity

- **WHEN** two custom presets use the same display name
- **THEN** each remains independently addressable by its distinct stable custom
  ID and neither overwrites the other

### Requirement: Subtitle style settings use the existing settings contract

The system SHALL persist versioned subtitle styling settings through the
existing AppSetting infrastructure. The normalized `subtitleStyling` object
SHALL contain `version`, `enabled`, `applyToGeneratedASS`,
`preserveSourceStyles`, `overrideExistingASSStyles`, built-in role overrides,
custom presets, the preset palette, and bounded saved/recent color state.
`enabled` SHALL be the master gate for every #123 appearance mutation.
`applyToGeneratedASS` SHALL control only generated ASS when styling is enabled;
`preserveSourceStyles` SHALL control the native-document safety default; and
`overrideExistingASSStyles` SHALL be the explicit authorization for eligible
native base-event Style reassignment. Native restyle SHALL require the master
gate and explicit override authorization. Generated ASS SHALL require both the
master gate and `applyToGeneratedASS=true`.

When `enabled=false`, no #123 appearance mutation SHALL occur regardless of the
other stored flags, and existing generated ASS, native ASS/SSA, and SRT
behavior SHALL remain unchanged. When styling is enabled, the combination
`preserveSourceStyles=false` and `overrideExistingASSStyles=false` SHALL be
rejected as ambiguous; a native mutation requires explicit override. A missing
#123 configuration SHALL resolve to `enabled=false`,
`applyToGeneratedASS=true`, `preserveSourceStyles=true`, and
`overrideExistingASSStyles=false` without a noisy missing-record failure.
Built-in definitions MAY be represented by only their overrides; missing
overrides SHALL resolve to the documented built-in defaults. Missing #123 data
SHALL leave existing SRT, OCR, native ASS/SSA, translation, and fallback
behavior unchanged.

Backend validation SHALL normalize accepted values before storage and reject
invalid or structurally unsafe values. The setting version SHALL support
backward-compatible reading of installations that have only the existing
subtitle settings fields. Unknown future versions SHALL fail closed for the
new styling fields while retaining safe defaults for output.

#### Scenario: Existing installation has no styling setting

- **WHEN** settings are loaded from an installation without #123 data
- **THEN** the backend returns `enabled=false`,
  `applyToGeneratedASS=true`, `preserveSourceStyles=true`,
  `overrideExistingASSStyles=false`, the four documented built-ins, and empty
  bounded custom/palette state without a record-not-found error

#### Scenario: Styling is disabled

- **WHEN** a user saves `enabled=false` with any other styling flags present
- **THEN** generated ASS, native ASS/SSA, and SRT follow their pre-#123
  behavior, and no configured color, style, or position-aware mapping is
  applied

#### Scenario: Generated-only styling is explicit

- **WHEN** a user saves `enabled=true`, `applyToGeneratedASS=true`,
  `preserveSourceStyles=true`, and `overrideExistingASSStyles=false`
- **THEN** generated SRT-to-ASS, bitmap OCR-to-ASS, and other generated
  canonical ASS use the configured resolver while native ASS/SSA remains
  source-owned

#### Scenario: Native preservation is explicit

- **WHEN** styling is enabled but `applyToGeneratedASS=false`,
  `preserveSourceStyles=true`, and `overrideExistingASSStyles=false`
- **THEN** generated ASS keeps legacy appearance and native ASS/SSA preserves
  source styles and event assignments

#### Scenario: Explicit native override is gated

- **WHEN** a user saves `enabled=true` and `overrideExistingASSStyles=true`
  for an eligible native operation
- **THEN** only the explicitly authorized eligible native base Style assignments
  may use configured styles; protected source structure remains preserved

#### Scenario: Ambiguous native policy is rejected

- **WHEN** styling is enabled with `preserveSourceStyles=false` and
  `overrideExistingASSStyles=false`
- **THEN** backend validation rejects the setting instead of guessing whether
  native source styles should be preserved or mutated

#### Scenario: Invalid setting update is rejected atomically

- **WHEN** a setting update contains an invalid color, unsupported preset ID
  shape, out-of-range numeric field, duplicate custom ID, or excessive palette
  payload
- **THEN** the update is rejected and the previously stored setting remains
  effective

### Requirement: User colors have one normalized HEX contract and canonical ASS conversion

User-facing subtitle colors SHALL be represented as uppercase six-digit
`#RRGGBB` values after normalization. Three-digit HEX input SHALL be accepted
only when it is expanded deterministically (`#abc` becomes `#AABBCC`); malformed,
non-HEX, or out-of-range values SHALL be rejected rather than silently
interpreted. The backend SHALL own the only color conversion helper used for
ASS output and SHALL convert RGB plus the configured opacity to ASS/SSA
`&HAABBGGRR&` form. ASS alpha semantics SHALL be explicit: `00` is opaque and
`FF` is fully transparent. A style opacity of 0% SHALL serialize as `FF`; 100%
SHALL serialize as `00`; intermediate values SHALL use the documented bounded
rounding rule. The contract SHALL cover primary, outline, and shadow/back colors
and deterministic normalized round trips.

#### Scenario: HEX input normalizes before persistence

- **WHEN** a user enters lowercase `#abC123` or shorthand `#abc`
- **THEN** the stored and returned value is respectively `#ABC123` or
  `#AABBCC`, and ASS conversion uses the corresponding BGR byte order

#### Scenario: Invalid color does not reach a serializer

- **WHEN** a user enters `#12FG00`, `123456`, `#12345`, or a non-finite alpha
- **THEN** backend validation rejects the value with an actionable error and no
  malformed ASS color is emitted

#### Scenario: Alpha round trip is explicit

- **WHEN** a valid style is serialized at opaque, transparent, and intermediate
  opacity
- **THEN** the ASS alpha bytes follow `00=opaque` and `FF=transparent`, and
  parsing the canonical representation recovers the normalized RGB/opacity
  within the documented rounding precision

### Requirement: Palette state is bounded, normalized, deduplicated, and deterministic

The Settings UI SHALL provide the fixed initial palette entries White `#FFFFFF`,
Warm White `#FFF4D6`, Yellow `#FFFF00`, Gold `#FFD700`, Cyan `#00FFFF`, Sky Blue
`#7FDBFF`, Light Green `#90EE90`, Pink `#FF9ED2`, Orange `#FFA500`, and Lavender
`#C4A7E7`. Saved Colors and Recent Colors SHALL use canonical HEX values,
deduplicate case-insensitively after normalization, and remain bounded in the
persisted AppSetting. Saved Colors SHALL have a documented maximum of 32 values;
Recent Colors SHALL have a documented maximum of 12 values, with newest use
first and deterministic oldest eviction. Reordering or duplicate submissions
SHALL not grow the setting indefinitely. An eyedropper MAY be added later but
is not required for this capability.

#### Scenario: Repeated color use remains bounded

- **WHEN** a user saves or selects the same color in different casing repeatedly
- **THEN** only one canonical HEX entry remains and the recent list stays within
  its bound with deterministic newest-first ordering

#### Scenario: Palette overflow evicts only bounded history

- **WHEN** more than 32 saved colors or 12 recent colors are supplied
- **THEN** the backend retains only the documented bounded set according to its
  deterministic ordering rule and never creates an unbounded AppSetting value

### Requirement: Generated ASS style resolution is canonical and deterministic

When `enabled=true` and `applyToGeneratedASS=true`, the canonical resolver SHALL
resolve style selection in this order: an explicit operation style selection or
eligible canonical semantic role mapping, then a canonical layout/style hint
when one exists, then `MVF_Default`. When either gate is false, generated ASS
SHALL use its existing legacy appearance and SHALL NOT consult the #123 style
mapping. Existing canonical semantic-role authority SHALL remain the source of
role decisions: `signs` MAY select `MVF_Sign`, while `full`, `forced`,
`commentary`, `sdh`, and `unknown` SHALL use their configured mapping or fall
back to `MVF_Default`. A role of `unknown` SHALL NOT be silently reclassified
as a top note from geometry. `MVF_Top` SHALL be selectable by an explicit
canonical policy/layout hint, but #123 SHALL NOT introduce a second semantic
role classifier merely to infer it.

Geometry and position evidence SHALL affect generated alignment or exact
position only through the existing canonical layout evidence. It SHALL NOT
silently change semantic role or style identity. Resolver output SHALL include
style ID and resolved style definition separately from role, alignment, and
optional exact position.

#### Scenario: Known sign role selects the configured sign appearance

- **WHEN** a generated cue has canonical role `signs` and no explicit operation
  override
- **THEN** the resolver selects the configured `MVF_Sign` style while retaining
  the canonical role and any independent layout evidence

#### Scenario: Unknown role uses the safe default

- **WHEN** a generated cue has role `unknown` and upper-region geometry
- **THEN** style selection remains the configured `MVF_Default`; geometry may
  resolve alignment evidence but does not relabel the cue or select `MVF_Top`

#### Scenario: Semantic role and geometry disagree

- **WHEN** a canonical `signs` cue has lower-region geometry
- **THEN** semantic role remains authoritative for style selection and geometry
  remains independent layout evidence rather than reclassifying the cue

### Requirement: Styling and positioning remain separate

Canonical subtitle data SHALL represent role, selected style, alignment behavior,
and exact positioning as separate values. An event MAY have a lyrics role,
`MVF_Lyrics`, and an explicit `\pos(...)` at the same time. A style's alignment
fallback SHALL apply only when the event has no valid explicit `\pos`, `\move`,
or alignment override that the existing parser recognizes. Applying a configured
style SHALL NOT erase valid explicit position, movement, margins, layer, or
event-level positioning metadata.

#### Scenario: Explicit position wins over style alignment

- **WHEN** a generated or restyled event contains valid explicit `\pos(...)` and
  its selected style has top alignment
- **THEN** the explicit position remains in the event and the style alignment
  does not rewrite or remove it

#### Scenario: OCR geometry remains reusable layout evidence

- **WHEN** OCR produces a cue with retained #26 geometry
- **THEN** the existing coarse layout resolver supplies alignment/position
  evidence to ASS serialization without adding geometry to SRT or creating a
  second OCR/cue path

#### Scenario: Disabled generated styling preserves legacy output

- **WHEN** a generated ASS request runs with `enabled=false` or
  `applyToGeneratedASS=false`
- **THEN** the configured MVF colors and style mappings are not emitted and the
  existing generated ASS appearance remains authoritative

### Requirement: Native ASS and SSA preserve source styles by default

The default policy SHALL be `enabled=false`,
`preserveSourceStyles=true`, and `overrideExistingASSStyles=false`. Native
ASS/SSA processing SHALL preserve source style definitions, event style
assignments, karaoke timing/tokens, inline override tags, `\an`, `\pos`,
`\move`, margins, layers, drawing mode, text, timing, cue order, and
representation. Restyle SHALL reuse the generic canonical source-preserving
ASS/SSA document boundary and SHALL NOT rerun OCR or flatten a native document
into MVForge built-ins. SRT behavior SHALL remain unchanged.

#### Scenario: Styled native ASS round trip is unchanged by default

- **WHEN** a native ASS source contains multiple custom styles, custom colors,
  karaoke, inline override tags, and explicit positions and the user runs the
  default restyle operation
- **THEN** the output preserves those definitions, event references, tags,
  karaoke structure, positions, text, timing, order, and layers without mapping
  them to MVF styles

#### Scenario: Native SSA remains SSA

- **WHEN** a native SSA source is selected for default restyle
- **THEN** its SSA representation and source style/event structure remain
  preserved and no ASS-to-SSA or SSA-to-ASS conversion is introduced

### Requirement: ASS/SSA source preservation is a generic canonical boundary

The system SHALL provide one shared source-preserving ASS/SSA structural
parser/reconstructor for Translation, Styling, and future subtitle composition.
The boundary SHALL be independent of Translation-specific provider or
translatable-text models. It SHALL preserve representation, script/style
sections, style definitions, event Style assignment, layer, timing, margins,
effect, override tags, `\an`, `\pos`, `\move`, karaoke, drawing mode, line
breaks, and comments or unknown preserved lines where supported by the current
document behavior. The reconstruction SHALL retain source structure and SHALL
not flatten native documents into generated canonical-cue output.

Translation MAY use an adapter that exposes only protected translatable dialogue
text and SHALL remain limited to text mutation. Styling MAY append deterministic
generated style definitions and change eligible base event Style assignments
only under its explicit policy. Future #131 Composition MAY consume the same
boundary without importing Translation-specific concepts. Refactoring this
boundary SHALL preserve existing Translation behavior and tests.

#### Scenario: Shared parser preserves a styled native document

- **WHEN** a native ASS/SSA document with multiple styles, comments, unknown
  lines, karaoke, inline tags, explicit positioning, margins, layers, and
  drawing events passes through the shared boundary without an authorized
  mutation
- **THEN** representation and supported source structure are retained without
  flattening or style reinterpretation

#### Scenario: Translation adapter remains text-only

- **WHEN** Translation supplies replacement dialogue text through the generic
  document boundary
- **THEN** only the protected translatable text changes and timing, style,
  events, tags, karaoke, positioning, and other structural fields remain equal

#### Scenario: Styling uses the same boundary narrowly

- **WHEN** enabled native override is authorized for eligible events
- **THEN** Styling may append deterministic generated definitions and change the
  eligible base Style assignment while preserving source definitions and all
  protected event structure

#### Scenario: Generic extraction retains Translation behavior

- **WHEN** the current Translation ASS/SSA fixtures are run before and after
  structural behavior is extracted from Translation-specific naming
- **THEN** the structural output and protected-dialogue validation results are
  equivalent, with no Translation behavior change introduced by #123

### Requirement: Generated ASS rendering remains separate from native documents

Generated SRT-to-ASS and bitmap-OCR-to-ASS SHALL flow from canonical text or
bitmap cues through the existing/shared generated ASS renderer. Bitmap output
SHALL use one canonical OCR result plus retained #26 geometry. Native ASS/SSA
SHALL flow through the generic source-preserving document boundary and SHALL
not be flattened into the generated-cue renderer. Generated style serialization
helpers MAY be shared, but generated-cue rendering and native-document
reconstruction SHALL remain distinct authorities.

#### Scenario: SRT uses the generated-cue path

- **WHEN** a selected SRT is converted to styled ASS with both styling gates
  enabled
- **THEN** canonical text cues feed the shared generated ASS renderer and the
  source SRT remains text/timing-only and unchanged

#### Scenario: Bitmap uses the same generated style authority

- **WHEN** a bitmap subtitle is OCR-converted to ASS
- **THEN** OCR runs once, canonical bitmap cues and #26 geometry feed the same
  generated ASS style authority, and no native-document parser is used to
  flatten the OCR result

#### Scenario: Native ASS does not use generated flattening

- **WHEN** a native ASS/SSA source is preserved or explicitly restyled
- **THEN** the generic source-preserving document boundary retains native
  sections and structure instead of routing the document through the generated
  cue renderer

### Requirement: Explicit native override is opt-in and structurally conservative

When a user explicitly enables `enabled=true` and
`overrideExistingASSStyles=true`, the operation SHALL map only eligible
dialogue event style assignments to the selected MVForge style IDs or resolved
role mappings. `overrideExistingASSStyles=true` while the master gate is false
SHALL have no output effect. The operation SHALL preserve source style
definitions rather than silently redefining them, and SHALL preserve text,
timing, cue order, layer, margins, valid positioning/movement, karaoke
timing/tokens, drawing mode, and inline override tags. Inline reset/style tags
SHALL NOT be rewritten as an incidental consequence of changing the event style
field. The UI SHALL warn that native appearance can change and SHALL require
the explicit opt-in for this behavior. Comments, malformed events, and events
outside the operation's eligibility set SHALL remain untouched.

#### Scenario: Override changes assignment but not structure

- **WHEN** a user explicitly enables override for a native ASS with dialogue
  events using source styles and the master styling gate is enabled
- **THEN** eligible event assignments resolve to collision-safe generated MVF
  style names while source definitions and protected event structure remain
  available and unchanged

#### Scenario: Override is not implied by Settings edits

- **WHEN** a user customizes `MVF_Default` but leaves native override disabled
- **THEN** native ASS/SSA output continues to use its source style definitions
  and event references

#### Scenario: Native override cannot bypass the master gate

- **WHEN** `overrideExistingASSStyles=true` is stored while `enabled=false`
- **THEN** native ASS/SSA remains source-owned and no configured style is
  appended or assigned

### Requirement: Generated and native style names never collide semantically

The system SHALL never reinterpret a source style merely because its visible
name equals a configured MVForge ID such as `MVF_Default` or `MVF_Top`. Generated
or explicitly restyled definitions SHALL use deterministic MVForge namespaced
style names derived from stable style identity and the operation's collision
policy. Existing source style names and definitions SHALL remain source-owned.
When a generated output already contains a conflicting generated name, the
resolver SHALL choose a deterministic suffix from stable artifact/style
identity rather than order-dependent or random renaming.

#### Scenario: Source `MVF_Default` is not reinterpreted

- **WHEN** native ASS defines a style named `MVF_Default` with a red primary
  color while MVForge `MVF_Default` is configured as white
- **THEN** default processing keeps the source red style and its event
  references; it does not silently substitute the configured white style

#### Scenario: Restyled collision receives a safe generated name

- **WHEN** explicit override needs to add a configured style whose visible name
  collides with a source style
- **THEN** the generated definition receives a deterministic namespaced name,
  eligible events refer to that name, and source semantics are not corrupted

### Requirement: SRT remains text/timing-only and logical both shares canonical evidence

Styling settings SHALL never add styling syntax to SRT, alter an existing SRT,
or force an SRT user to select ASS. A logical `both` conversion SHALL use one
canonical normalized/OCR cue result, send it unchanged to the existing SRT
serializer, and send the same result plus resolved style/layout data to the
existing ASS serializer. Serializers SHALL NOT invoke OCR, recalculate semantic
roles, or perform independent cue discovery.

#### Scenario: Changing a style does not rewrite SRT

- **WHEN** a user changes a subtitle preset after an SRT sidecar exists
- **THEN** the SRT remains byte-for-byte untouched unless a separate explicit
  subtitle operation targets it, and no ASS requirement is introduced

#### Scenario: Bitmap both performs OCR once

- **WHEN** a bitmap subtitle requests both SRT and styled ASS
- **THEN** canonical OCR/geometry evidence is produced once, SRT remains neutral
  text/timing, and ASS receives style plus existing geometry without a second
  OCR pass

### Requirement: Settings preview is immediate but non-authoritative

Settings SHALL display an immediate browser preview using dark, light, and mixed
background samples and representative top-note, dialogue, lyrics, and sign
examples where useful. The preview SHALL use the normalized style contract as a
UI approximation and SHALL NOT invoke FFmpeg, OCR, a subtitle operation, or
publication. The UI SHALL identify that font availability, libass rendering,
exact outline metrics, and screen scaling can differ from final ASS output. The
backend canonical resolver and serializer SHALL remain authoritative.

#### Scenario: Color changes update preview without a job

- **WHEN** a user changes a color or opacity before saving Settings
- **THEN** the preview updates immediately in the browser and no FFmpeg/OCR job
  or sidecar is created

#### Scenario: Preview is not treated as output validation

- **WHEN** a configured font is unavailable in the browser or a preview metric
  differs from libass
- **THEN** the preview remains an approximation and output validation still uses
  the backend serializer/sidecar validation path

### Requirement: Asset Info restyle and regenerate target exact source identity

Asset Info SHALL expose a single-sidecar Restyle / Regenerate entry point that
captures and displays the exact selected source identity before work begins.
Identity SHALL include source path or artifact ID, source revision/fingerprint,
representation (`srt`, `ass`, or `ssa`), language, stream/artifact identity,
role/provenance, and the applicable style policy/version. The operation SHALL
not select another same-language file by filename guessing or broad directory
matching. A source revision mismatch before publication SHALL produce a stale or
conflict result and SHALL not overwrite any source or current published sidecar.

#### Scenario: Same-language sibling is not substituted

- **WHEN** the selected external source is one of two same-language sidecars
- **THEN** Restyle / Regenerate reads and verifies only the selected artifact
  identity and fails closed if that exact artifact is unavailable or changed

#### Scenario: Source changes during work

- **WHEN** the selected source revision changes after operation start but before
  publication
- **THEN** the operation ends stale/conflict, cleans its staging, publishes no
  new artifact, and preserves the changed source and prior published sidecar

### Requirement: Restyle and regenerate reuse canonical pipelines by mode

Restyle / Regenerate SHALL support these observable modes:

- Restyle existing ASS/SSA SHALL reuse existing text, timing, events,
  representation, positioning, and the canonical source-preserving
  parser/serializer. It SHALL not rerun OCR merely to change appearance.
- Generate styled ASS from SRT SHALL read the selected SRT through the existing
  canonical cue path, create an ASS sibling using the configured style resolver,
  and leave the SRT unchanged.
- Regenerate from a canonical embedded/bitmap source SHALL invoke the existing
  canonical extraction/OCR lifecycle only when source provenance permits
  recomputation, then style the resulting canonical cues and geometry through
  the existing ASS serializer.

No mode SHALL create a second OCR path, subtitle cue model, ASS serializer, or
sidecar publisher.

#### Scenario: ASS restyle avoids OCR

- **WHEN** a native ASS/SSA sidecar is restyled
- **THEN** the existing document is parsed and serialized without starting OCR
  or provider work, and protected structure is preserved under the selected
  policy

#### Scenario: SRT produces a styled sibling

- **WHEN** a selected SRT sidecar is converted through Asset Info to styled ASS
- **THEN** a validated ASS sibling is staged and published while the original
  SRT remains unchanged

#### Scenario: Bitmap regeneration reuses canonical OCR

- **WHEN** a bitmap-derived artifact has valid source provenance and regeneration
  is requested
- **THEN** the existing canonical OCR/geometry lifecycle supplies cues once and
  the operation does not implement a second regeneration/OCR flow

### Requirement: Restyle operations use existing lifecycle and non-destructive publication

Restyle / Regenerate SHALL use the existing subtitle operation model with
queued/running, completed, failed, and cancelled-equivalent terminal states,
operation cancellation, bounded cleanup, validation, staging, retry, and
provenance. Restyle of an existing ASS/SSA SHALL be lightweight and SHALL not
invoke OCR or translation/provider work unless the selected mode explicitly
requires canonical regeneration. New output SHALL be staged, validated, and
inspectable before publication. The default target SHALL be a sibling and SHALL
preserve the source and any current published sidecar. Replacing an existing
sidecar SHALL require explicit user opt-in and SHALL use the existing rollback
and atomic publication barrier. A cancellation or failure SHALL remove only
operation-owned staging and leave existing artifacts intact.

#### Scenario: Cancellation preserves current output

- **WHEN** a running Restyle / Regenerate operation is cancelled before its
  publication barrier
- **THEN** child work stops through the existing operation context, staging is
  cleaned, the operation is terminally cancelled, and source/current sidecars
  remain unchanged

#### Scenario: Replacement requires explicit opt-in

- **WHEN** a generated target path already exists and the user has not enabled
  replacement
- **THEN** the operation publishes a safe sibling or reports a deterministic
  conflict without overwriting that file

#### Scenario: Publication is atomic

- **WHEN** validation or publication of the styled artifact fails
- **THEN** no partial new sidecar is visible, rollback restores any explicitly
  replaced destination, and previously published artifacts remain available

### Requirement: Font selection respects existing attachment boundaries

Custom styles MAY reference a configured font family and SHALL permit `auto`
font sizing, but the system SHALL not assume arbitrary configured fonts exist at
playback time, download fonts, or invent a bundled font. Native ASS/SSA font
attachments and their existing extraction/publication behavior SHALL be
preserved. Validation or Settings MAY warn about runtime/browser availability
when evidence exists, but missing evidence SHALL not mutate the style or source
document. #123 SHALL not create a second font attachment manager.

#### Scenario: Native attachments remain associated

- **WHEN** a native ASS/SSA artifact with embedded font attachments is preserved
  or restyled
- **THEN** existing attachment handling remains authoritative and no attachment
  is dropped merely because the style setting was edited

#### Scenario: Missing configured font is explicit

- **WHEN** a custom style names a font unavailable to the known runtime
- **THEN** the UI/backend reports a warning where evidence is available, but
  does not download, substitute, or silently bundle another font

### Requirement: Style policy is frozen for running and queued execution

Every running Restyle / Regenerate operation and every Queue execution SHALL
capture a normalized style-policy snapshot at its applicable start/freeze
boundary, including resolved preset definitions, source-style policy, collision
policy, and a bounded configuration version/hash. Later Settings edits SHALL
affect only future operations or jobs that have not frozen their effective
configuration. Provenance SHALL record the style policy/version when practical
without storing unbounded settings payloads.

#### Scenario: Settings edit does not change running output

- **WHEN** a style setting changes while a subtitle operation is running
- **THEN** that operation completes with its captured style snapshot and the
  changed setting applies only to a later operation

#### Scenario: Queue freeze is authoritative

- **WHEN** Settings change after a Queue job freezes its resolved subtitle plan
- **THEN** the worker uses the frozen style policy and does not reread mutable
  Settings during processing

### Requirement: Styling primitives remain compatible with future subtitle composition

The style contract SHALL keep stable style IDs, canonical role-to-style
resolution, source-style preservation, and deterministic namespaced style-name
collision handling reusable by future #131 subtitle composition. #123 SHALL not
implement composition, multi-source cue merging, batch restyling, or a second
composition-specific styling layer. Track Profiles MAY reference stable style
IDs in a future change, but #123 SHALL not duplicate style definitions inside
Track Profiles.

#### Scenario: Future policy can reference a preset ID

- **WHEN** a later composition policy supplies a stable preset ID to the
  canonical resolver
- **THEN** it can reuse the same style definition and collision policy without
  defining another style schema

#### Scenario: Composition remains out of scope

- **WHEN** a user configures styles in #123
- **THEN** no multi-source subtitle composition or cue merge is performed by
  this capability
