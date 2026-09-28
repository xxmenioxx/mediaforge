## Purpose

Each container release must carry enough version-specific source, notice, inventory, and build evidence to let recipients identify and review the components conveyed by that image.

## ADDED Requirements

### Requirement: Container release evidence matches the image build

The release process SHALL produce an evidence bundle for each container image that identifies the image digest, exact build inputs and scripts, resolved runtime packages, and the corresponding source location or source bundle for GPL-covered FFmpeg, x264, and x265 components. The evidence SHALL distinguish what was inspected from what remains unavailable.

#### Scenario: GPL-enabled backend image is prepared for release
- **WHEN** a backend image is prepared for publication
- **THEN** the release evidence includes the exact FFmpeg configuration, x264/x265 linkage, package versions, build recipe, and corresponding source bundle or stable source location tied to that image

#### Scenario: Required source evidence cannot be assembled
- **WHEN** an exact GPL component version or matching source artifact cannot be resolved
- **THEN** compliance verification fails and the public release gate remains closed

### Requirement: Conveyed runtime components retain their notices

Each container release SHALL provide the license and notice materials for the third-party components it conveys, including notices supplied inside NuGet packages and notices for SeConv and the Intel GPU runtime. Repository-level summaries SHALL identify the source of the exact notice material and SHALL NOT substitute for the notice text itself.

#### Scenario: Runtime notices are packaged
- **WHEN** a backend or web runtime image is built
- **THEN** the image and associated release evidence contain the required notices for its conveyed components

#### Scenario: A required notice is missing
- **WHEN** a required component notice is absent from the runtime or release evidence
- **THEN** compliance verification fails before public publication

### Requirement: Dependency inventories describe shipped dependencies

The release evidence SHALL contain a resolved dependency and license inventory for the backend Go binary and the frontend npm packages shipped in the web image. The inventory SHALL record versions and license evidence, and SHALL identify missing or ambiguous license metadata rather than silently treating package metadata as proof of license-text satisfaction.

#### Scenario: Backend and web inventories are generated
- **WHEN** the release evidence is generated from locked manifests
- **THEN** it includes resolved backend modules and the production dependency closure included in the frontend bundle

#### Scenario: Dependency license evidence is incomplete
- **WHEN** a shipped dependency has no resolvable license evidence or required notice text
- **THEN** the inventory marks it unresolved and the release check fails

### Requirement: Public release publication remains evidence-gated

The public release workflow SHALL fail closed by default and SHALL refuse to publish unless the required release evidence has been generated and validated. SBOM and provenance outputs SHALL be associated with the exact published image digest before the corresponding source and notice assets are released.

#### Scenario: Gate remains at its default setting
- **WHEN** the publication workflow runs without an explicit enable setting
- **THEN** it fails before publishing any image or GitHub Release

#### Scenario: Publication is explicitly enabled without complete evidence
- **WHEN** the enable setting is true but a required inventory, source bundle, notice, SBOM, or provenance output is absent
- **THEN** the workflow fails without creating the GitHub Release

#### Scenario: Publication has complete matching evidence
- **WHEN** all evidence is present and validated for the image digest
- **THEN** the release assets expose the source, notices, SBOM, and provenance evidence associated with that image digest
