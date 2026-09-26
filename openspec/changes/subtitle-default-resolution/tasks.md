## 1. Canonical effective settings contract

- [ ] 1.1 Extend the existing backend subtitle planning input/output to preserve optional operation-field presence and report effective format, OCR language/quality, fallback outputs, style policy, and per-field resolution source; verify with table-driven precedence tests covering explicit, asset/profile, global Settings, and built-in values.
- [ ] 1.2 Resolve the existing Tracks Asset Defaults and Subtitle Settings through the established asset/profile authority without adding settings or a parallel resolver; verify configured output format, OCR mode, SRT fallback, and styling values are represented in the canonical plan.

## 2. Asset Info extraction and conversion

- [ ] 2.1 Route text subtitle extraction and SRT/ASS conversion through the canonical resolved plan; verify omitted fields inherit defaults, explicit formats win, original extraction remains explicit, and an unresolved converted format uses SRT.
- [ ] 2.2 Pass resolved OCR language, OCR quality, format, fallback, and style policy into PGS/VobSub OCR → SRT/ASS; verify configured defaults, explicit per-field overrides, one shared OCR result for both formats, and existing evidence/cache reuse.
- [ ] 2.3 Update Asset Info request construction so untouched displayed OCR suggestions are omitted while user-selected values remain explicit; verify request payloads distinguish absent values, explicit values, and explicit auto without changing the control layout.

## 3. Consumer and lifecycle compatibility

- [ ] 3.1 Keep Preview, Test Encode, and Queue decisions on the same canonical plan; verify equivalent inputs produce matching output formats, OCR inputs, fallback outputs, and applicable styling decisions.
- [ ] 3.2 Preserve Queue freeze and legacy snapshot behavior; verify settings changes before freeze are resolved, changes after freeze do not alter execution/retry, and snapshots without new effective-settings metadata are not re-resolved.
- [ ] 3.3 Preserve SRT fallback eligibility, ASS styling/geometry/positioning, #114 preferred-language selection, and translation ownership; verify targeted regressions for each boundary.
- [ ] 3.4 Preserve Asset Info cancellation and retry identity behavior; verify cancellation remains terminal, retry receives a new operation identity using captured request intent/settings, and valid canonical OCR evidence remains reusable only under existing identity checks.

## 4. Validation

- [ ] 4.1 Run focused subtitle resolver, Asset Info extraction/OCR, and Queue snapshot regressions; verify configured defaults are honored and explicit overrides win deterministically.
- [ ] 4.2 Run ./scripts/verify.sh --auto and confirm ALL CHECKS PASSED.
