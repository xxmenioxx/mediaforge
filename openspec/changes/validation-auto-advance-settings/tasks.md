## 1. Settings contract and UI

- [x] 1.1 Record the existing PASS-only legacy default, explicit warning opt-in, and independent Publisher gates in OpenSpec.
- [x] 1.2 Add the automatic outcome control, current-policy status, Auto Publisher status, confirmation, and merge-safe preservation in the Validation Settings card.

## 2. Regression coverage

- [x] 2.1 Cover missing and empty settings, explicit warning values, saved-value reload, disabled Auto Publisher, and confirmation scope.
- [x] 2.2 Cover merge-safe saves and preservation across preset changes and restore-defaults.

## 3. Validation

- [ ] 3.1 Run focused frontend tests and strict OpenSpec validation.
- [ ] 3.2 Run `./scripts/verify.sh --auto` through the serialized dupla lock and record the result.
