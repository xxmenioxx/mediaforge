## ADDED Requirements

### Requirement: OCR AI has independent disabled-by-default Settings

The system MUST persist OCR AI Settings under an OCR-specific setting key with an `enabled` boolean and an independently selected model identifier. A missing setting MUST resolve to `enabled: false` and an empty model. Saving or migrating OCR AI Settings MUST NOT copy, change, or clear the Translation model or enablement. The same model identifier MAY be selected independently for both features.

#### Scenario: Existing Translation configuration is present

- **WHEN** an existing database is opened and has Translation settings but no OCR AI settings
- **THEN** the system adds OCR AI settings with `enabled: false` and an empty model
- **AND** it preserves the existing Translation model, endpoint projection, enablement, and extension fields

#### Scenario: A model is explicitly selected

- **WHEN** an operator saves an OCR AI model identifier
- **THEN** the identifier is stored only in the OCR AI setting
- **AND** the Translation model is not read or changed

### Requirement: OCR model validation reuses the shared provider

The Settings UI MUST discover installed models and check a selected model using the existing shared Ollama catalog and per-model readiness APIs. Settings MUST NOT create a second endpoint or contact Ollama directly. Model discovery and readiness checks MUST be explicit user actions and MUST NOT invoke inference.

#### Scenario: A selected OCR model is available

- **WHEN** the operator checks a selected model and the shared provider confirms its metadata
- **THEN** the UI reports the model as available
- **AND** it leaves the saved model identifier and enabled state unchanged until the operator explicitly saves changes

#### Scenario: A selected model is missing or the provider is unavailable

- **WHEN** the model readiness check returns missing, offline, invalid, or unavailable
- **THEN** the UI reports that state
- **AND** it preserves the selected identifier without silently replacing or clearing it
- **AND** it does not silently enable or disable OCR AI

#### Scenario: OCR AI starts disabled

- **WHEN** no OCR AI setting exists or the operator turns OCR AI off while a model remains selected
- **THEN** OCR AI remains disabled
- **AND** the selected model is preserved
- **AND** the Settings action does not invoke model inference

### Requirement: OCR AI edits use the shared Settings draft lifecycle

The OCR AI editor MUST participate in the existing Subtitles Settings draft registry and shared save protection. Refetches and navigation MUST preserve unsaved OCR AI edits. Saving MUST persist the OCR AI setting through the section save flow, and a failed save MUST leave the draft available for correction or retry.

#### Scenario: An unsaved OCR AI selection survives refetch and navigation

- **WHEN** an operator changes the OCR AI model and Settings refetches or navigates away from and back to Subtitles
- **THEN** the unsaved OCR AI model selection remains in the draft
- **AND** the Settings UI continues to identify the Subtitles section as unsaved

#### Scenario: The shared save persists and reloads the OCR AI selection

- **WHEN** the operator saves an OCR AI draft from the Subtitles section
- **THEN** the section save persists the `ocrAISettings` value
- **AND** a subsequent Settings refetch shows the saved value
- **AND** the draft is no longer marked unsaved
