#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'compliance check failed: %s\n' "$*" >&2
  exit 1
}

for path in LICENSE NOTICE LEGAL.md THIRD_PARTY_LICENSES.md; do
  [[ -s "$path" ]] || fail "missing or empty $path"
done

if grep -Fq -- '--enable-nonfree' backend/Dockerfile; then
  fail "backend/Dockerfile enables FFmpeg --enable-nonfree"
fi

for flag in --enable-gpl --enable-version3 --enable-libx264 --enable-libx265; do
  grep -Fq -- "$flag" backend/Dockerfile     || fail "expected FFmpeg licensing flag missing: $flag"
done

grep -Fq './scripts/verify-compliance.sh' .github/workflows/ci.yml   || fail "CI does not run the compliance verifier"
grep -Fq './scripts/verify-compliance.sh' .github/workflows/release-images.yml   || fail "release workflow does not run the compliance verifier"
grep -Fq 'MVFORGE_PUBLIC_RELEASES_ENABLED' .github/workflows/release-images.yml   || fail "release workflow is missing the temporary public-release gate"

printf 'Compliance baseline checks passed.\n'
