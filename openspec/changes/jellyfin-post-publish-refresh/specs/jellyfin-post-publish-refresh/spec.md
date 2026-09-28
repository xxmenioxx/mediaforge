## Purpose

Keep Jellyfin's catalog current when consumable media changes in an MVForge Library, while preserving Publisher and Test Encode as separate lifecycle authorities and keeping Jellyfin failures isolated from both.

## ADDED Requirements

### Requirement: Canonical publication refresh is driven only by AssetPublished

The system SHALL request a canonical publication refresh only after consuming the canonical `AssetPublished` event for a publication whose `PUBLISHED` state and archive outcome are recorded. The event SHALL be the sole authority for this trigger. Worker completion, Validation completion, staging, `PUBLISHING`, partial or failed publication, and legacy `PublishedAt` values SHALL NOT trigger a canonical publication refresh.

#### Scenario: Canonical publication event requests refresh
- **WHEN** the canonical `AssetPublished` event is delivered after publication and archive outcome recording
- **THEN** the integration SHALL resolve the event's stable MVForge Library identity and request a refresh for its configured Jellyfin Library mapping

#### Scenario: Other publication lifecycle activity does not request refresh
- **WHEN** a Worker completes, Validation completes, output is staged, publication is `PUBLISHING`, publication is partial or failed, or only a legacy `PublishedAt` value exists
- **THEN** the integration SHALL NOT request a canonical publication refresh

#### Scenario: Duplicate publication event is safe
- **WHEN** the same `AssetPublished` event is delivered more than once
- **THEN** the integration SHALL deduplicate it using the canonical event identity and SHALL NOT create uncontrolled refresh requests

#### Scenario: Jellyfin failure preserves publication
- **WHEN** a Jellyfin refresh request for a canonical publication fails
- **THEN** the publication SHALL remain `PUBLISHED`, its archive outcome and Validation evidence SHALL remain unchanged, and the failure SHALL be recorded only as integration evidence

### Requirement: Test Encode refresh requires a consumable READY output

The system SHALL request a Test Encode refresh only after FFmpeg completes, the media output and generated or required sidecars are activated in the selected MVForge Library, output validation and probing succeed, and the Test Encode is persisted as `READY`. A Test Encode refresh SHALL use `TestEncode.LibraryID` to resolve the configured Jellyfin Library ID. Output path, source path, and either library's display name SHALL NOT be used to infer a target.

#### Scenario: Ready Test Encode requests its mapped library refresh
- **WHEN** a Test Encode reaches persisted `READY` after media and sidecar activation and successful output validation
- **THEN** the integration SHALL request `test_encode_ready` for the Jellyfin Library ID mapped from that Test Encode's `LibraryID`

#### Scenario: Mapping uses stable Library identity
- **WHEN** a Test Encode output path or either displayed library name suggests a different target than its persisted `LibraryID` mapping
- **THEN** the integration SHALL use only the persisted `LibraryID` mapping

#### Scenario: Incomplete or invalid output does not request refresh
- **WHEN** FFmpeg completes but output activation is incomplete, sidecar activation fails, output validation or probing fails, or the Test Encode fails or is canceled
- **THEN** the integration SHALL NOT request a Test Encode refresh

#### Scenario: Test Encode is not a publication
- **WHEN** a Test Encode reaches `READY` or is removed
- **THEN** it SHALL NOT emit `AssetPublished`, impersonate `PublisherPublication`, authorize publication through #53 `ValidationRun`, archive or modify the source asset, or otherwise enter the Queue or Publisher lifecycle

#### Scenario: Jellyfin failure preserves Test Encode and source state
- **WHEN** a Jellyfin refresh request for a Test Encode fails
- **THEN** the Test Encode SHALL remain `READY` with its `Keep` value unchanged, the source asset SHALL remain unchanged, and Queue, Publisher, and Validation evidence SHALL remain unchanged

### Requirement: Test Encode removal refresh follows successful physical cleanup

The system SHALL request `test_encode_removed` when a previously `READY` Test Encode's Library output or registered sidecars are physically mutated by explicit deletion, expiration, or housekeeping cleanup. The request SHALL use the persisted `TestEncode.LibraryID`. A cleanup that removes at least one owned artifact before a later removal fails SHALL still request Jellyfin reconciliation, while preserving the existing failed cleanup/deletion lifecycle result and recording integration evidence separately. A failed cleanup SHALL NOT be represented as a successful Test Encode deletion.

#### Scenario: Explicit deletion refreshes after removing READY output
- **WHEN** explicit deletion physically removes a previously `READY` Test Encode output or registered sidecars from its MVForge Library
- **THEN** the integration SHALL request refresh for the Jellyfin Library ID mapped from the persisted `LibraryID` after the filesystem removal succeeds

#### Scenario: Expiration and housekeeping refresh after removal
- **WHEN** expiration or housekeeping physically removes a previously `READY` Test Encode output or registered sidecars from its MVForge Library
- **THEN** the integration SHALL request refresh for the Jellyfin Library ID mapped from the persisted `LibraryID` after the filesystem removal succeeds

#### Scenario: Retained Test Encode does not request removal refresh
- **WHEN** a Test Encode has `Keep=true` and its output remains in the MVForge Library
- **THEN** no removal refresh SHALL be requested

#### Scenario: Failed or no-op cleanup does not report a successful removal refresh
- **WHEN** filesystem cleanup fails or the cleanup operation removes none of the Test Encode's existing Library output or registered sidecars
- **THEN** the integration SHALL NOT report a successful removal refresh

#### Scenario: Partial cleanup failure still requests reconciliation
- **WHEN** cleanup physically removes an owned Library output or sidecar and a later artifact removal fails
- **THEN** the delete or expiration operation SHALL retain its existing failure semantics, the integration SHALL separately request `test_encode_removed` reconciliation for the persisted Library mapping, and the Test Encode SHALL NOT be reported as successfully deleted

#### Scenario: Jellyfin failure preserves removal evidence
- **WHEN** a removal refresh fails after successful filesystem cleanup
- **THEN** the Test Encode's deletion or expiration evidence SHALL remain unchanged, and the integration failure SHALL NOT restore files or touch source media

### Requirement: Jellyfin settings use stable library IDs and bounded discovery

The system SHALL provide optional Jellyfin settings, disabled by default, for enablement, Base URL, API key or token, refresh-after-publish, Test Encode refresh, debounce window, and MVForge Library ID to Jellyfin Library ID mappings. Mapping identity SHALL use stable IDs; display names SHALL be presentation only. Connection testing SHALL use bounded network operations to report reachability, authentication, server version when available, and discoverable Jellyfin Library IDs with display names. It SHALL NOT guess IDs, run shell commands, write Jellyfin database data, or manipulate Jellyfin files.

#### Scenario: Mapping binds configured Library IDs
- **WHEN** a user maps an MVForge Library to a discovered Jellyfin library
- **THEN** the saved mapping SHALL bind the MVForge Library ID to the Jellyfin Library ID, and renamed display names SHALL NOT change mapping identity

#### Scenario: Connection test discovers stable IDs
- **WHEN** the user tests a reachable and authenticated Jellyfin server
- **THEN** the result SHALL include its version when available and discovered Jellyfin Library IDs with display names for mapping

#### Scenario: Unavailable version does not invalidate connection
- **WHEN** Jellyfin is reachable and authentication and library discovery succeed but server version is unavailable
- **THEN** the connection test SHALL report the successful capabilities and an unavailable version without inventing a version

#### Scenario: Disabled or unmapped integration is bounded
- **WHEN** a qualifying trigger arrives while integration is disabled or its MVForge Library has no mapping
- **THEN** the system SHALL record `skipped_disabled` or `skipped_unmapped` respectively and SHALL NOT call Jellyfin

### Requirement: One bounded coordinator handles refresh and integration outcomes

Canonical publication, Test Encode readiness, Test Encode removal, and optional manual refresh SHALL use one shared Jellyfin integration coordinator and one Jellyfin client, settings source, mapping set, debounce policy, and retry policy. Reasons SHALL be limited to `asset_published`, `test_encode_ready`, `test_encode_removed`, and `manual_refresh`; they SHALL remain integration context and SHALL NOT become lifecycle states.

The coordinator SHALL persist bounded outcomes sufficient for UI and retry, including a bounded status, reason, stable internal deduplication identity, mapped target identity, attempt count, timestamps, and sanitized failure classification. It SHALL NOT persist raw HTTP error text or credentials as retry evidence. Bounded statuses SHALL include `pending`, `success`, `skipped_disabled`, `skipped_unmapped`, `retryable_failure`, and `permanent_configuration_error`.

#### Scenario: Requests for one effective target coalesce
- **WHEN** publication, Test Encode readiness, Test Encode removal, or manual refresh requests arrive for the same effective Jellyfin Library target during its debounce window
- **THEN** the coordinator SHALL issue one refresh for that target and associate the bounded result with the contributing integration outcomes

#### Scenario: Effective refresh targets debounce independently
- **WHEN** qualifying requests target different effective Jellyfin refresh targets
- **THEN** each effective target SHALL have its own debounce window and refresh request

#### Scenario: Server-wide fallback coalesces mapped libraries
- **WHEN** the supported Jellyfin contract does not establish a scoped operation that detects descendant additions and removals
- **THEN** requests for different mapped Jellyfin Library IDs SHALL share the server-wide effective target and coalesce into one scan

#### Scenario: Duplicate READY observation does not create a scan storm
- **WHEN** the same Test Encode `READY` observation is delivered more than once
- **THEN** the coordinator SHALL deduplicate it using a stable Test Encode identity and SHALL coalesce it with any pending request for the same effective target

#### Scenario: Debounce delay is bounded
- **WHEN** triggers continue to arrive for one effective target during a debounce window
- **THEN** the target SHALL be refreshed no later than the configured maximum delay from the first pending trigger

#### Scenario: Restart preserves pending and retryable work
- **WHEN** MVForge restarts while a debounced refresh or bounded retry is pending
- **THEN** persisted pending work SHALL resume using its recorded due time and deduplication identity without creating a separate generic scheduler

#### Scenario: Manual retry reuses the coordinator
- **WHEN** a user retries a failed Jellyfin refresh for a publication or Test Encode
- **THEN** the request SHALL reuse the shared coordinator and SHALL NOT mutate the related publication, Test Encode, source asset, or Validation state

### Requirement: Refresh calls are narrow, bounded, and failure-isolated

The system SHALL use the narrowest reliable Jellyfin refresh operation supported by the connected server and SHALL prefer the configured mapped Jellyfin Library target. If that server does not support a scoped library refresh, the system SHALL make any broader refresh fallback explicit in the integration status and user-facing documentation. Connection and refresh requests SHALL have bounded timeouts and backoff; transient failures SHALL be retryable and invalid credentials, invalid configuration, or a stale mapped Jellyfin Library ID SHALL be classified as permanent configuration errors until corrected.

#### Scenario: Scoped refresh targets the mapped library
- **WHEN** the connected Jellyfin contract is known to provide a scoped operation that detects descendant additions and removals, and the mapped Library ID is valid
- **THEN** the system SHALL request refresh for that mapped library without using output paths or names

#### Scenario: Broader server refresh is disclosed
- **WHEN** the connected Jellyfin contract does not establish scoped descendant scanning for the mapped library
- **THEN** the system SHALL use the documented server-wide refresh fallback and identify the broader scope in the integration outcome

#### Scenario: Transient outage is retried within bounds
- **WHEN** Jellyfin returns a timeout, rate limit, network failure, or transient server error
- **THEN** the coordinator SHALL retry with bounded backoff and a finite attempt limit, then retain a `retryable_failure` outcome for explicit retry if the limit is exhausted

#### Scenario: Invalid credentials or stale mapping fail safely
- **WHEN** Jellyfin rejects credentials or does not recognize a mapped Jellyfin Library ID
- **THEN** the coordinator SHALL record `permanent_configuration_error`, SHALL NOT guess a replacement ID, and SHALL NOT change the originating lifecycle state

#### Scenario: Disabled or skipped work makes no request
- **WHEN** a trigger is disabled or has no mapping
- **THEN** no Jellyfin refresh request SHALL be sent and the bounded skipped outcome SHALL remain visible

### Requirement: Jellyfin credentials and diagnostics remain secret

The system SHALL treat Jellyfin API credentials as secrets. Read responses SHALL expose configured/not-configured state rather than stored plaintext whenever the Settings architecture permits. Credentials SHALL NOT appear in History, metrics labels, logs, retry evidence, diagnostics, UI responses, or error messages; diagnostics SHALL be redacted before persistence or display. This change SHALL NOT introduce authentication infrastructure.

#### Scenario: Settings reads redact credentials
- **WHEN** Jellyfin settings are read by the frontend
- **THEN** the response SHALL report whether a credential is configured and SHALL NOT return the stored credential value

#### Scenario: Connection and refresh errors are sanitized
- **WHEN** an authentication, discovery, or refresh request fails
- **THEN** persisted and displayed diagnostics SHALL use bounded sanitized classifications and SHALL NOT contain the API credential, authorization header, or credential-bearing URL

#### Scenario: Credentials stay out of History and metric labels
- **WHEN** an integration outcome is shown or exposed for observability
- **THEN** it SHALL omit credential values and future metric labels SHALL omit paths, media filenames, Test Encode IDs, publication IDs, event IDs, Jellyfin URLs, and raw error text

### Requirement: Integration status stays concise and separate from lifecycle state

The system SHALL expose bounded Jellyfin integration outcomes for Settings, Publisher or History, and Test Encode where useful, including status and time and a retry action for eligible failures. Jellyfin outcomes SHALL remain separate from publication, Test Encode, Queue, and Validation lifecycle states. #56 SHALL expose bounded evidence only; Prometheus and Grafana implementation remains owned by #55.

#### Scenario: Publisher or History shows refresh result
- **WHEN** a publication has a Jellyfin integration outcome
- **THEN** Publisher or History SHALL show concise status and time, including a retry action for an eligible failure, without changing the `PUBLISHED` label

#### Scenario: Test Encode shows concise integration result
- **WHEN** a Test Encode has a Jellyfin integration outcome
- **THEN** its view MAY show concise status and time separately from `READY`, `Keep`, and removal state

#### Scenario: Future observability consumes bounded outcomes
- **WHEN** #55 consumes Jellyfin integration evidence
- **THEN** it SHALL receive bounded status, reason, and duration evidence without raw error strings or high-cardinality identifiers as metric labels
