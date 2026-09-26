## MODIFIED Requirements

### Requirement: Canonical converted subtitle formats

The canonical subtitle planner SHALL resolve an operation's effective
converted format in this order: explicit operation format, asset/profile
resolved format, canonical global Settings, then the documented built-in SRT
fallback. A configured asset/profile format SHALL count as resolved even when
the operation omits its format. If no format is resolved after applying that
precedence, converted output SHALL default to SRT. An explicit `original`
request SHALL retain original extraction semantics and SHALL NOT be rewritten
as converted output. A logical `both` request SHALL remain one subtitle
transform that performs text normalization or bitmap OCR once, then renders
both artifacts from the same normalized cue result. This requirement SHALL
introduce no new OCR semantics, sidecar discovery or matching behavior, or
parallel subtitle pipeline.

#### Scenario: Explicit converted format wins

- **WHEN** an operation explicitly requests converted SRT or ASS while a
  profile or global default selects another format
- **THEN** the canonical plan SHALL use the explicit format and apply the
  existing SRT fallback policy afterward

#### Scenario: Converted SRT

- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted SRT
- **THEN** the canonical pipeline SHALL produce a .srt artifact with
  normalized cues

#### Scenario: Converted ASS

- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted ASS
- **THEN** the canonical pipeline SHALL produce a .ass artifact with the
  converted cues

#### Scenario: Inherited converted format is resolved

- **WHEN** an operation omits its format and an asset/profile or canonical
  global setting supplies one
- **THEN** the canonical plan SHALL use that resolved format

#### Scenario: Built-in converted format fallback

- **WHEN** a converted request has no explicit, asset/profile, or global format
- **THEN** the canonical plan SHALL select SRT

#### Scenario: Default format

- **WHEN** a converted subtitle request has no explicit or inherited output
  format
- **THEN** the pipeline SHALL select SRT and preserve existing SRT behavior

#### Scenario: Original extraction remains explicit

- **WHEN** an operation explicitly requests original extraction for a source
  that supports it
- **THEN** the canonical plan SHALL preserve its original representation,
  independently of converted-format defaults

#### Scenario: Both converted formats share one cue result

- **WHEN** the effective converted format is both SRT and ASS
- **THEN** the canonical pipeline SHALL normalize text or perform bitmap OCR
  once and produce both artifacts from that shared cue result

## ADDED Requirements

### Requirement: Effective subtitle settings use one precedence contract

The system SHALL resolve subtitle operation settings using the precedence
`explicit operation override > asset/profile resolved setting > canonical
global Settings > documented built-in fallback`. Asset/profile resolution
SHALL retain the existing Asset, Path, LogicalGroup, SourceGroup, and Track
Profile authority and its distinction between inherited, explicit, and
explicitly disabled values. The effective decisions SHALL cover output format,
OCR language, OCR quality, SRT fallback, and styling/positioning wherever the
existing pipeline supports those settings. The resolver SHALL use existing
`assetDefaults.tracks` and `subtitleSettings` values and SHALL NOT add a second
settings store or new user-configurable setting.

#### Scenario: Explicit operation fields override inherited settings

- **WHEN** an Asset Info operation supplies a supported format, OCR language,
  or OCR quality value and a more general setting supplies a different value
- **THEN** the effective plan SHALL use the explicit operation value for that
  field only, preserving inheritance for omitted fields

#### Scenario: Asset and profile values precede global defaults

- **WHEN** an operation omits a field and the resolved asset or Track Profile
  supplies that field while global Settings also supplies it
- **THEN** the effective plan SHALL use the asset/profile value

#### Scenario: Global defaults precede documented fallbacks

- **WHEN** an operation and resolved asset/profile omit a supported field and
  canonical global Settings define it
- **THEN** the effective plan SHALL use the global value before using a
  built-in fallback

#### Scenario: OCR language uses existing language evidence

- **WHEN** OCR language is absent from the operation and resolved profile and
  no canonical global OCR-language setting exists
- **THEN** the canonical OCR path SHALL retain its existing automatic/source
  language inference and Japanese orientation fallback without deriving OCR
  language from or changing #114 preferred-language track selection

#### Scenario: OCR quality uses existing configured modes

- **WHEN** an OCR operation omits quality and the resolved asset/profile or
  global Tracks defaults define an existing OCR mode
- **THEN** the canonical OCR request SHALL receive that mode; if none is
  defined, it SHALL use the existing `accurate` built-in fallback

#### Scenario: SRT fallback remains an independent global policy

- **WHEN** effective format resolution selects ASS and the existing
  `generateSRTFallbackForASS` setting is enabled for an eligible source
- **THEN** the existing canonical fallback rules SHALL add SRT without
  changing the explicit or inherited ASS choice; an explicit SRT-only request
  SHALL remain SRT-only

#### Scenario: Styling and positioning use existing policy

- **WHEN** an operation produces generated ASS and omits supported style
  overrides
- **THEN** it SHALL use the resolved existing subtitle styling policy and
  preserve existing geometry/positioning behavior; SRT timing and text SHALL
  remain unaffected by ASS styling

### Requirement: Optional subtitle request fields distinguish overrides from inheritance

Asset Info subtitle requests SHALL preserve the difference between an absent
optional setting and a supplied supported value. An absent value SHALL inherit
through the effective settings precedence. A supplied value, including a
supported explicit automatic choice such as `auto`, SHALL be treated as an
operation override. Invalid explicit values SHALL retain existing validation
failure behavior. Frontend presentation heuristics SHALL NOT be serialized as
operation overrides unless the user explicitly selects them.

#### Scenario: Omitted request field inherits

- **WHEN** an Asset Info request omits output format, OCR language, or OCR
  quality
- **THEN** the backend SHALL resolve each omitted field from the asset/profile,
  canonical global Settings, and built-in fallback in order

#### Scenario: Supplied request field is explicit

- **WHEN** an Asset Info request supplies a supported value for one optional
  field
- **THEN** that field SHALL override its inherited value while other absent
  fields continue to inherit

#### Scenario: Display heuristic is not an operation override

- **WHEN** the client displays a suggested OCR language or quality before the
  user changes that control
- **THEN** it SHALL omit that suggestion from the operation request so backend
  settings remain authoritative

#### Scenario: Explicit automatic language is retained

- **WHEN** the user explicitly chooses the supported `auto` OCR language
- **THEN** the request SHALL preserve that explicit choice and the canonical
  OCR path SHALL perform its existing source-language inference

### Requirement: Effective subtitle settings are shared and frozen at operation boundaries

Asset Info extraction/conversion, Track Profile planning, Preview, Test Encode,
and Queue SHALL expose compatible effective subtitle decisions for equivalent
source and resolved settings. Effective decisions SHALL include the resolved
format/mode, OCR inputs when applicable, fallback outputs, and applicable ASS
style policy. Queue SHALL freeze the effective settings and canonical plan
before processing; workers SHALL consume that snapshot without re-reading
current Settings. Asset Info SHALL capture one effective settings snapshot for
each operation before extraction or OCR begins.

#### Scenario: Asset Info and Queue/Profile planning agree

- **WHEN** Asset Info, Preview, Test Encode, and Queue receive equivalent
  source facts and resolved settings with no different operation override
- **THEN** they SHALL select matching format, OCR inputs, fallback outputs,
  and supported style policy

#### Scenario: Settings change before Queue freeze

- **WHEN** a Queue job has not frozen its effective subtitle plan and a
  relevant Settings value changes
- **THEN** the next preparation attempt SHALL resolve the current value before
  freezing the plan

#### Scenario: Settings change after Queue freeze

- **WHEN** a Queue job has frozen its effective subtitle plan and Settings
  later change
- **THEN** execution and retry SHALL continue to consume that frozen plan

#### Scenario: Asset Info retry keeps operation identity boundaries

- **WHEN** an Asset Info conversion is cancelled or fails and is retried
- **THEN** the terminal operation SHALL remain unchanged, the retry SHALL use
  a new operation identity with the captured effective settings and request
  intent, and existing valid artifacts SHALL remain eligible for canonical
  satisfaction

#### Scenario: Legacy Queue snapshot remains authoritative

- **WHEN** a pre-existing Queue snapshot lacks newly represented effective
  settings
- **THEN** the system SHALL preserve that snapshot's existing resolved profile
  and plan behavior and SHALL NOT silently re-resolve it from current global
  Settings

#### Scenario: Translation retains its own request authority

- **WHEN** subtitle translation resolves a source and requested translated
  representation
- **THEN** translation SHALL retain its existing source, locale, provider,
  cue, and output ownership and SHALL NOT reinterpret translation choices as
  subtitle extraction defaults

#### Scenario: OCR lifecycle and evidence identity are unchanged

- **WHEN** effective OCR language and quality are passed to PGS or VobSub OCR
- **THEN** the existing canonical OCR lifecycle, evidence validation/cache,
  Japanese fallback, cancellation cleanup, and retry behavior SHALL remain
  authoritative
