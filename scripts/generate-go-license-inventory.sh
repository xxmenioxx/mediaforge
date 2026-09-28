#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root/backend"

go list -m all > licenses/GO_MODULES.txt
go run github.com/google/go-licenses@v1.6.0 report ./... > licenses/GO_LICENSES.csv
go run github.com/google/go-licenses@v1.6.0 save ./... --save_path=licenses/go-modules

printf 'Generated Go module and license evidence under backend/licenses.\n'
