#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'compliance check failed: %s\n' "$*" >&2
  exit 1
}

for path in LICENSE NOTICE LEGAL.md THIRD_PARTY_LICENSES.md \
            backend/licenses/NOTICE backend/licenses/THIRD_PARTY_LICENSES.md \
            backend/licenses/MVFORGE-APACHE-2.0.txt \
            backend/go.mod backend/go.sum frontend/package-lock.json; do
  [[ -s "$path" ]] || fail "missing or empty $path"
done

if grep -Fq -- '--enable-nonfree' backend/Dockerfile; then
  fail "backend/Dockerfile enables FFmpeg --enable-nonfree"
fi

for flag in --enable-gpl --enable-version3 --enable-libx264 --enable-libx265; do
  grep -Fq -- "$flag" backend/Dockerfile || fail "expected FFmpeg licensing flag missing: $flag"
done

for flag in --enable-libvpl --enable-vaapi --enable-libdrm; do
  grep -Fq -- "$flag" backend/Dockerfile || fail "expected hardware-media flag missing: $flag"
done

grep -Fq 'no distribuye contenido' README.md || fail "README is missing the content-distribution boundary"
grep -Fiq 'no implementa ni incluye herramientas para evadir DRM' README.md || fail "README is missing the DRM non-goal"
grep -Fq 'LEGAL.md' README.md || fail "README does not link the legal boundaries"
grep -Fq 'backend/go.mod' THIRD_PARTY_LICENSES.md || fail "third-party inventory does not identify the backend Go manifest"
grep -Fq 'frontend/package-lock.json' THIRD_PARTY_LICENSES.md || fail "third-party inventory does not identify the frontend npm lockfile"

python3 -c 'import json,sys; lock=json.load(open("frontend/package-lock.json", encoding="utf-8")); packages=[(name,entry) for name,entry in lock.get("packages",{}).items() if name.startswith("node_modules/")]; missing=[name for name,entry in packages if not entry.get("license")]; print(f"npm lock license metadata: {len(packages)} resolved packages, {len(missing)} missing"); sys.exit(bool(missing) or not packages)'

grep -Fq './scripts/verify-compliance.sh' .github/workflows/ci.yml || fail "CI does not run the compliance verifier"
grep -Fq './scripts/verify-compliance.sh' .github/workflows/release-images.yml || fail "release workflow does not run the compliance verifier"
grep -Fq 'sbom: true' .github/workflows/release-images.yml || fail "release images do not request an SBOM attestation"
grep -Fq 'provenance: mode=max' .github/workflows/release-images.yml || fail "release images do not request provenance"
grep -Fq 'MVFORGE_PUBLIC_RELEASES_ENABLED:-false' .github/workflows/release-images.yml || fail "release gate must default to closed"
grep -Fq '!= "true"' .github/workflows/release-images.yml || fail "release gate must require an explicit true value"
for notice in LICENSE NOTICE LEGAL.md THIRD_PARTY_LICENSES.md; do
  grep -Fq "$notice" .github/workflows/release-images.yml || fail "release workflow omits $notice"
done

printf 'Compliance baseline checks passed.\n'
