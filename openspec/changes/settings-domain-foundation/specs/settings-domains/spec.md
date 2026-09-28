## ADDED Requirements

### Requirement: Settings definitions have canonical domains and policies

The system MUST resolve known setting keys through one canonical definition that identifies its domain and the validation, normalization, response projection, and update effects applicable to that key. Existing setting keys and non-secret JSON values MUST remain readable and writable with compatible behavior.

#### Scenario: Existing setting key is updated

- **WHEN** a client updates a known persisted setting key
- **THEN** the system applies that key's registered validation and normalization
- **AND** it stores the value under the same `AppSetting` key
- **AND** any registered database effects commit with the setting value

#### Scenario: Existing unknown legacy key is updated

- **WHEN** a client updates a key not yet present in the canonical registry
- **THEN** the system preserves the existing generic JSON upsert behavior through a compatible legacy definition

### Requirement: Multi-key Settings updates are atomic

The system MUST support a batch update that validates and normalizes all entries and applies all setting writes and registered database effects in one transaction.

#### Scenario: Batch succeeds

- **WHEN** every key and value in a batch is valid and every database operation succeeds
- **THEN** all setting values and registered effects commit together

#### Scenario: Batch validation or persistence fails

- **WHEN** a batch contains an invalid entry or any write/effect fails
- **THEN** none of the batch's setting values or registered effects remain committed
- **AND** the response identifies the failure without returning secret values

### Requirement: Settings API responses do not expose credentials

The system MUST keep provider credentials outside ordinary `AppSetting` values and MUST NOT include credential values in Settings list or update responses.

#### Scenario: Jellyfin settings are listed or updated

- **WHEN** the normal Settings API returns Jellyfin settings
- **THEN** it exposes only a stable credential reference or configured indicator
- **AND** it omits the credential value, including for rows migrated from the legacy format

#### Scenario: Jellyfin credential is set or cleared

- **WHEN** a client uses the existing `apiToken` or `clearCredential` update fields
- **THEN** the value is written to or removed from the protected secret source transactionally
- **AND** the non-secret Jellyfin setting remains under its existing key

### Requirement: Settings domains remain addressable and behavior-compatible

The Settings UI MUST present domain-scoped components including General and Providers & Integrations, preserve existing domain links, and retain the current Asset Defaults behavior.

#### Scenario: Domain is opened by URL

- **WHEN** a user opens Settings with an existing or new domain section in the URL
- **THEN** the matching domain is selected and its existing controls remain available

#### Scenario: Asset Defaults are edited

- **WHEN** Asset Defaults are loaded or updated through Settings
- **THEN** current precedence, resolver, and Queue snapshot behavior remain unchanged
