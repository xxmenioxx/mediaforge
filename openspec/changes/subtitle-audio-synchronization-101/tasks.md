## 1. Canonical proposal and transform

- [x] 1.1 Add an engine-neutral fixed-offset proposal and timing-transform contract; verify model validation tests cover fixed, affine, and segmented proposal shapes without claiming unsupported engine capability.
- [x] 1.2 Implement SRT and protected ASS/SSA timing-only transforms with byte-preservation and invalid interval checks; verify focused format tests preserve all non-timing content.
- [x] 1.3 Resolve explicit audio stream identity and bind proposals to current subtitle/audio revisions; verify missing and stale identities fail closed.

## 2. Safe Asset Info operation

- [x] 2.1 Add a read-only preview API with bounded before/after cue samples; verify API tests prove it does not mutate the source.
- [x] 2.2 Add cancellable apply through the existing subtitle operation registry and atomic replacement path; verify cancellation, stale revision, invalid output, and publication failure preserve the original.
- [x] 2.3 Persist compact applied provenance and expose it for later audit; verify a temporary SQLite database records and retrieves source/audio identity and transform metadata without cue text.

## 3. Asset Info review flow

- [x] 3.1 Add an exact-sidecar Sync action with explicit audio selection and fixed-offset input; verify UI tests require a selected backend-resolved audio stream.
- [x] 3.2 Show bounded timing preview, model, reference audio, and review state before Apply; verify preview cannot publish or replace a subtitle.
- [x] 3.3 Show operation progress/cancellation and refresh sidecar evidence after apply; verify polling stops on every terminal status.

## 4. Optional automatic alignment boundary

- [x] 4.1 Verify ffsubsync and ALASS command contracts, format support, upstream licenses, distribution, and NAS resource limits from primary sources; record the decision before adding an adapter or dependency.
- [x] 4.2 Add a dependency-free, bounded SRT fixed-offset proposal engine and backend-authoritative runtime capability reporting; verify missing-helper tests preserve manual fixed-offset use, automatic preview is read-only, and low-confidence proposals cannot apply.

## 5. Verification

- [x] 5.1 Run focused backend and frontend synchronization tests and resolve only scoped failures; verify the exact commands and results are recorded.
- [x] 5.2 Run `./scripts/verify.sh --auto` through the serialized `shared-validation` lock and confirm `ALL CHECKS PASSED`.
