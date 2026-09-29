#!/usr/bin/env bash
set -euo pipefail

exec go run github.com/google/go-licenses/v2@v2.0.1 "$@"
