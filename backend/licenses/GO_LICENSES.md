# Go dependency license evidence

`GO_MODULES.txt` records the complete resolved `go list -m all` graph for
`backend/go.mod` and `backend/go.sum`. `GO_LICENSES.csv` is the package/license
report from `github.com/google/go-licenses/v2` v2.0.1 for the Go packages in the
backend binaries. The `go-modules/` tree contains the license texts collected
from those exact resolved module versions. The root backend Apache-2.0 license
is also copied to `backend/LICENSE` so the local main module is classified by
the scanner and its notice is packaged with the backend image.

Regenerate with `./scripts/generate-go-license-inventory.sh`. The compliance
verifier checks the resolved graph and package/license report against the
checked-in inventories. `go-licenses` warns about C and assembly files in
several modules because it does not inspect non-Go imports; those files remain
covered by their containing module license text and are disclosed in the
audit.
