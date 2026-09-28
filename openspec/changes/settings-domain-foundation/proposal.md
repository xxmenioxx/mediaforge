## Purpose

Introduce an explicit domain foundation for application Settings while keeping existing persisted keys, values, resolver behavior, and Queue snapshots compatible.

## What Changes

- Define canonical setting metadata for key, domain, validation, normalization, and transactional update effects; route single-key and batch writes through the same registry.
- Add an atomic multi-key Settings API operation and use it for the existing multi-key Settings save.
- Keep non-sensitive values in `AppSetting`; store Jellyfin credentials behind a protected secret source and expose only credential references/configured state through normal Settings responses.
- Organize the Settings UI by domains, add General and Providers & Integrations, and retain URL-addressable sections.

## Out of Scope

- Implementing Ollama or other new providers, OpenAI/DeepL/Google/Azure, translation or Advisor behavior.
- Redesigning Asset Defaults or changing configuration precedence, resolver semantics, or Queue snapshot behavior.
- V1.x/V2 roadmap work and CI modernization.

## Impact

The backend adds a canonical registry, a credential-secret persistence boundary, and an atomic batch endpoint while retaining the existing `AppSetting` schema and single-key endpoint. The frontend adds domain composition and a batch client operation. Existing Library, asset, and Queue authorities remain unchanged.
