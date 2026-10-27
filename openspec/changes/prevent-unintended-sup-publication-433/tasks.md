## 1. Frozen authorization and provenance

- [x] 1.1 Preserve the resolver's explicit original and converted format/mode/source decisions; focused resolver tests verify PGS SRT, ASS, both, original, precedence, and provenance.
- [x] 1.2 Carry source stream identity into Worker subtitle artifacts and authorize Publisher promotion from exact frozen decisions; focused artifact and manifest tests verify identity binding.
- [x] 1.3 Cover extra/stale/mismatched ready artifacts, empty plans, same-language streams with distinct indexes, explicit PGS original, required/optional behavior, and legacy sidecars in focused backend tests.
- [x] 1.4 Render original versus converted formats and format decision source in Queue Preflight; the focused component test verifies the visible labels.

## 2. Validation

- [x] 2.1 Run the focused resolver, worker/subtitle artifact, Publisher, and Preflight tests, and verify `openspec validate prevent-unintended-sup-publication-433 --type change --strict` passes.
- [x] 2.2 Run the serialized `./scripts/verify.sh --auto`; record its selected scope, check count, warnings, and final result.
