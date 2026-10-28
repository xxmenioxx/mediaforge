## Purpose

Provide one Settings experience for the shared Ollama provider and independent Translation, OCR AI, and Copilot configuration without silently changing providers, models, drafts, or inference state.

## ADDED Requirements

### Requirement: One canonical Ollama provider surface

Settings MUST present a single canonical Ollama endpoint and use the backend-owned connection, installed-model catalog, readiness, and runtime-status APIs. A connection test and a catalog refresh MUST remain distinct from saving configuration. Browser clients MUST NOT connect directly to Ollama.

#### Scenario: Provider inventory is refreshed
- **WHEN** the operator refreshes the model inventory
- **THEN** Settings shows installed models from the backend and distinguishes installed models from runtime-resident or inference-ready models
- **AND** the refresh does not save settings or select a model automatically

#### Scenario: Provider is disconnected or unavailable
- **WHEN** connection testing, inventory refresh, or readiness checking fails
- **THEN** Settings shows the unavailable state and preserves the saved endpoint and feature selections
- **AND** no feature is silently disabled or switched to another provider or model

### Requirement: Feature model selections remain independent

Translation, OCR AI, and Copilot MUST persist independent enabled/model selections. The same installed model MUST be selectable in more than one feature. A feature's selected model MUST NOT be copied from another feature, and an unavailable model MUST remain visible without an implicit replacement.

#### Scenario: The same model is selected for multiple features
- **WHEN** an operator selects one installed model for Translation, OCR AI, and Copilot
- **THEN** each feature retains its own selection and enablement state after save and reload

#### Scenario: A selected model is missing or offline
- **WHEN** the provider no longer reports a selected model as installed or ready
- **THEN** the selected identifier remains saved and visible with its unavailable state
- **AND** Settings does not choose a substitute model or provider

#### Scenario: A feature remains disabled
- **WHEN** an operator saves a disabled feature with a selected model
- **THEN** the selection is retained independently
- **AND** saving or loading the setting does not invoke inference or enable the feature

### Requirement: Translation and OCR AI settings remain compatible

Settings MUST preserve existing Translation configuration and migrate legacy values without replacing an explicitly selected model. The OCR AI selection MUST remain independent from Translation. Subtitles settings MUST retain existing compatibility and styling controls and provide access to the existing OCR Dictionary.

#### Scenario: Existing Translation settings are loaded
- **WHEN** a database contains legacy Translation settings
- **THEN** Settings preserves the existing enabled state and model across migration, save, reload, and navigation
- **AND** OCR AI and Copilot settings receive independent defaults

#### Scenario: Subtitles settings are organized
- **WHEN** the operator opens the Subtitles section
- **THEN** Translation, OCR AI, subtitle compatibility, styling, and OCR Dictionary access are available without duplicate cards in another section

### Requirement: Copilot configuration does not execute Copilot

Copilot settings MUST expose an independent enabled flag, model choice, and Quality, Balanced, or Compression preference. Copilot MUST default to disabled. Saving or loading Copilot settings MUST NOT invoke a model, download a model, or enable inference merely because a model is selected.

#### Scenario: Copilot settings are saved
- **WHEN** an operator changes Copilot configuration and saves the Copilot section
- **THEN** the model and preference persist independently
- **AND** the feature remains disabled unless the operator explicitly enables it

### Requirement: Settings drafts survive navigation and refetch

Unsaved changes MUST remain scoped to their Settings domain and survive URL navigation, unrelated provider refreshes, and unrelated settings refetches. Saving one domain MUST NOT discard or overwrite unrelated drafts or Jellyfin configuration.

#### Scenario: Provider data refreshes while feature settings are dirty
- **WHEN** provider connection or catalog data refreshes while one or more Settings domains have unsaved changes
- **THEN** the feature drafts and Jellyfin draft remain unchanged
- **AND** the user can still save or discard each domain through the existing draft guard

#### Scenario: Settings section is opened through its URL
- **WHEN** the operator navigates directly to a Subtitles, Providers & Integrations, or AI Copilot section URL
- **THEN** the matching section is shown with its persisted and draft values intact
