## MODIFIED Requirements

### Requirement: Explicit Asset Info subtitle translation

Asset Info SHALL expose an explicit `Translate...` operation for an eligible
embedded text subtitle, embedded PGS/VobSub subtitle, or supported external
SRT/ASS/SSA subtitle artifact. V1 SHALL support source language `Auto`,
`English`, or `Japanese`, target locale `es-419` or `es-ES`, and provider
`Ollama`. Translation SHALL be opt-in from Asset Info and SHALL NOT be
introduced into Queue, Track Profiles, Profile Lab, or automatic processing
policy.

For an owned subtitle on a current published Library asset with converted,
published-as-is, or accepted provenance, Asset Info SHALL expose the action
when no active source-operation guard blocks it. Translation SHALL remain
independent of video conversion and publication lifecycle.

#### Scenario: Embedded English text subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded English text subtitle
  with target `es-419` and an enabled, ready Ollama configuration
- **THEN** one subtitle translation operation is created and the resulting
  Spanish artifact is produced without changing the embedded source or its
  source classification

#### Scenario: Embedded Japanese bitmap subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded Japanese PGS or
  VobSub subtitle with target `es-ES` and an enabled, ready Ollama
  configuration
- **THEN** the existing canonical OCR path supplies the source cues to one
  translation operation and a separate Spanish artifact is planned

#### Scenario: External sidecar can be translated

- **WHEN** Asset Info selects an eligible external SRT, ASS, or SSA artifact
  and starts translation
- **THEN** the operation uses that exact artifact and its ownership/revision
  identity rather than guessing another same-language sidecar

#### Scenario: Published lifecycle sidecar remains available

- **WHEN** an eligible owned external SRT, ASS, or SSA sidecar belongs to a
  current published Library asset with converted, published-as-is, or accepted
  provenance and no active conflicting operation
- **THEN** Asset Info leaves `Translate...` available regardless of conversion
  or profile editing locks and opens the dialog for that exact sidecar

#### Scenario: Translation is not automatic Queue behavior

- **WHEN** a Queue job, Track Profile, or Profile Lab request is processed
  without an explicit Asset Info translation operation
- **THEN** no translation provider is invoked and existing subtitle planning
  and publication behavior remains unchanged

### Requirement: Asset Info translation controls reflect backend capabilities

The Asset Info `Translate...` dialog SHALL expose source language (`Auto`,
`English`, `Japanese`), target locale (`Spanish (Latin America) / es-419`,
`Spanish (Spain) / es-ES`), and provider. It SHALL expose a model control only
when the selected provider reports model selection as supported. The backend
response SHALL provide informational media-context status and source, and the
dialog SHALL display them when available. Missing context SHALL remain
informational and SHALL NOT block translation.

The backend SHALL remain authoritative for canonical valid output formats. The
frontend SHALL show an output-format selector only when the backend reports
more than one valid format; with one valid format it SHALL use that format
without requiring a choice. The frontend SHALL NOT invent, infer, add, or
suppress output-format eligibility independently. For a source that is safe to
inspect, the outer `Translate...` action SHALL open the dialog and the dialog
SHALL query the capability for the exact selected source. Provider enablement,
configuration, readiness, and model availability SHALL be shown as actionable
backend reasons inside the dialog; `Start translation` SHALL remain disabled
when the backend reports the source unavailable or the provider not ready. The
frontend SHALL not probe Ollama or infer provider readiness from endpoint/model
fields. When the canonical asset lifecycle action guard blocks translation,
Asset Info SHALL keep the action disabled and expose the specific blocker.

#### Scenario: Dialog displays available media context

- **WHEN** the selected subtitle has backend-reported media context with a
  status and source
- **THEN** the dialog displays that context status and source as informational
  fields alongside the source language, target locale, provider, and supported
  model controls

#### Scenario: Missing media context does not block translation

- **WHEN** the backend reports that media context is unavailable or empty for
  an otherwise eligible subtitle
- **THEN** the dialog indicates that context is unavailable, leaves the
  translation action enabled, and does not invent replacement context

#### Scenario: Multiple backend-authorized formats show a selector

- **WHEN** the backend reports more than one canonical valid output format for
  the exact subtitle entry
- **THEN** the dialog shows an output-format selector containing only those
  backend-reported formats

#### Scenario: One backend-authorized format needs no selector

- **WHEN** the backend reports exactly one canonical valid output format
- **THEN** the dialog does not require an output-format choice and submits the
  single backend-authorized format

#### Scenario: Frontend does not decide output eligibility

- **WHEN** a source, filename, client capability, or provider state could
  suggest an output format that is absent from the backend capability response
- **THEN** the dialog does not add or enable that format and relies on the
  backend-authorized set

#### Scenario: Disabled Translate remains discoverable

- **WHEN** an eligible subtitle is selected while translation is disabled,
  unconfigured, untested, unavailable, or missing its configured model
- **THEN** `Translate...` opens the dialog, the dialog displays the backend's
  bounded actionable reason, and `Start translation` remains disabled without
  probing or enabling the provider in the frontend

#### Scenario: Active Queue or maintenance work blocks translation

- **WHEN** the canonical asset lifecycle action guard reports active Queue or
  maintenance work for the selected asset
- **THEN** Asset Info disables translation with a specific active-work reason,
  and the backend capability and start operations reject the source while that
  work remains active

#### Scenario: Missing, unsupported, or archived sources remain blocked

- **WHEN** the exact source is missing, unsupported, outside owned media roots,
  belongs to an archive original, or has a stale revision
- **THEN** the backend reports a concrete source blocker, no sibling sidecar is
  substituted, and no translation output is published
