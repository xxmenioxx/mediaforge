## Context

See `proposal.md` for motivation and `specs/track-language-normalization/spec.md` for the behavior contract.

Current boundaries found during the audit:

| Boundary | Current behavior | Desired behavior |
| --- | --- | --- |
| Asset Info scan and track actions | Scan/API metadata retains container language strings; explicit audio/subtitle selections are stream-index lists. | Keep original metadata and exact index choices; use canonical language identity only for matching and concise labels. |
| Settings → General | `mvforgePreferences.preferredLanguages` is an ordered JSON list; UI/backend trim and lowercase but do not canonicalize aliases. It prefills forms and does not automatically select tracks. | Canonicalize known aliases while retaining order and unknown values; preserve the existing prefill-only policy. |
| Track Profiles and Profile Lab | Profile JSON stores language lists/defaults; Profile Lab has a partial local option list, Track Profiles accepts CSV, and persisted aliases are not consistently normalized. | Use the shared frontend catalog and canonicalize profile language inputs/reads; preserve unknown values and index-based overrides. |
| Asset Defaults and language rules | Subtitle defaults/rules use a subtitle-only catalog; audio preferred defaults are copied into track resolution with only lowercase conversion. | Reuse canonical track identity for Audio defaults and supported subtitle aliases without broadening the subtitle rule catalog. |
| Queue and Batch Preflight | `resolveTrackProfileForAsset` lowercases raw profile and stream language strings before equality checks, so equivalent aliases can produce different selected index lists. Default track matching compares a canonical stream value with a lowercased request. | Resolve language policies through the same normalizer before creating the immutable index/action plan. |
| Frozen plans and sidecars | Queue snapshots already store resolved track decisions; source scans and sidecar paths are separate persisted data. | Keep old snapshots immutable and use normalized identity only for new policy resolution; do not rewrite source tags or existing files. |

The repository has no single shared frontend catalog for all track workflows.
Backend subtitle rules already have an alias catalog, while the generic track
normalizer is adjacent to the canonical resolver. No schema or API field shape
change is needed.

## Goals / Non-Goals

**Goals:**

- Make the existing backend track normalizer the owner of canonical identity for audio and subtitle policy decisions.
- Keep subtitle-rule validation restricted to the existing supported subtitle set while reusing its aliases.
- Make profile/settings writes canonical for known aliases and make legacy values canonical at resolution time.
- Preserve stream indexes, explicit empty selections, original scan metadata, existing sidecar paths, Queue snapshots, and fingerprints.
- Share frontend options, alias handling, labels, and input normalization across existing language controls.

**Non-Goals:**

- Parse arbitrary BCP-47 tags or infer a language from an unrecognized region tag. Only exact equivalences already named in the spec are added.
- Change OCR/translation language availability, add locales, rename existing or newly published sidecars, or alter FFmpeg capabilities.
- Re-run media analysis, migrate Queue snapshots, or introduce a new selection resolver.

## Decisions

### Extend the existing canonical track normalizer

Add the explicit regional aliases `ko-kr` and `fr-fr` to the existing backend
alias map and keep generic unknown values lowercased and intact. The same helper
will normalize saved profile filters/defaults, preferred settings, and scanned
stream values before language comparisons. `canonicalSubtitleLanguage` remains
the strict validation boundary for subtitle-only settings; it uses the shared
normalization before checking the existing supported subtitle set.

An external BCP-47 library or a generic base-subtag parser was considered and
rejected. That could make unrelated regional tags equivalent and would enlarge
behavior beyond the specified aliases.

### Normalize new policy data without rewriting provenance or snapshots

The existing settings normalization hooks will canonicalize known values in
`mvforgePreferences`, `trackProfiles`, and `assetDefaults` on save. Resolution
will still normalize legacy values read from stored profiles. ScanResult tags,
Asset Info stream-index lists, source files, sidecar paths, Queue snapshots, and
fingerprint inputs will not be backfilled or rewritten. No database migration
is required.

Storing only canonical profile values was considered but would leave old saved
profiles inconsistent until rewritten. Read-time normalization is retained so
legacy aliases remain safe.

### Resolve language policies to indexes before Queue freeze

Queue profile language filters will build normalized language sets and compare
them with normalized scanned stream tags, then persist the resulting index
selection in the existing resolved track plan. Default audio/subtitle language
matching and subtitle role/rule matching will use the same function. Explicit
per-asset index lists are applied with their current precedence and are never
expanded or narrowed by language normalization. Stream-specific decisions
remain per-index, including when two streams share a language.

The resolver will retain each lower-case source language tag in the existing
sidecar language field used to build filenames, while the resolved embedded
track keeps the canonical code used for matching. This preserves existing
sidecar naming for aliases such as `ko`, `fr`, and `fre` as well as regional
tags such as `ko-KR` and `fr-FR`.

Adding a new selection model or rewriting frozen snapshots was rejected because
the current plan already freezes the effective actions and indexes.

### Share frontend language presentation without changing workflows

Move the current language options and alias handling behind one frontend track
language utility, keep the subtitle-only option/type exports compatible, and
use the shared functions in Settings, Track Profiles, Profile Lab, Asset Info,
and existing resolution displays. Existing controls keep their shape; no new
filters or OCR/translation options are introduced. Unknown values remain
visible as normalized codes, and source edit fields retain the source value.

## Risks / Trade-offs

- A user may have relied on distinct values that are now recognized aliases → only exact documented aliases canonicalize; all other values remain distinct.
- Canonical saved profile JSON differs in spelling from a legacy alias → the meaning remains equivalent, profile ordering and index selections stay unchanged, and no Queue snapshot is edited.
- A language label might imply OCR/translation support → keep those capability lists separate and label track identity only.
- Generated artifact naming can depend on source language metadata → do not modify source tag storage or sidecar naming code as part of matching normalization.

## Migration Plan

No schema migration or media operation is needed. New settings writes canonicalize known language values; existing JSON is normalized when it is read for policy resolution. Existing QueueJobs and archives are not rewritten. Rollback is code-only: stored canonical values remain valid under the existing lower-case behavior, while previously saved alias values are still readable.

## Open Questions

None. Regional equivalence is deliberately limited to the exact `ko-KR` and `fr-FR` tags named in the contract.
