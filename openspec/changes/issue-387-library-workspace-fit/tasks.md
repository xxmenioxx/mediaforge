## 1. Responsive Library layout

- [x] 1.1 Make the Library inventory constrain to its actual workspace width and progressively collapse secondary columns while keeping the filename, selection, status, and row actions visible; verify at wide and constrained container widths.
- [x] 1.2 Keep horizontal overflow bounded and reachable when the narrowest layout cannot fit; verify keyboard and pointer access to the last row action without page-wide overflow.

## 2. Regression coverage and validation

- [x] 2.1 Add focused inventory and workspace regressions for long filenames, 100+ assets, resized hierarchy widths, preserved RAW layout, and rightmost actions; verify the relevant test files pass.
- [x] 2.2 Run `./scripts/verify.sh --auto` through the serialized shared-validation lock and verify it reports `ALL CHECKS PASSED`.
