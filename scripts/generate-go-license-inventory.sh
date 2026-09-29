#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
license_scanner="$repo_root/scripts/go-license-scanner.sh"
cd "$repo_root/backend"

go list -m all > licenses/GO_MODULES.txt
"$license_scanner" report ./... > licenses/GO_LICENSES.csv
"$license_scanner" save ./... --save_path=licenses/go-modules

printf 'Generated Go module and license evidence under backend/licenses.\n'
