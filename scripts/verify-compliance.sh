#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'compliance check failed: %s\n' "$*" >&2
  exit 1
}

for path in LICENSE NOTICE LEGAL.md THIRD_PARTY_LICENSES.md \
            backend/licenses/NOTICE backend/licenses/THIRD_PARTY_LICENSES.md \
            backend/licenses/MVFORGE-APACHE-2.0.txt \
            backend/licenses/SOURCE-OFFER.md \
            backend/licenses/GO_LICENSES.md backend/licenses/GO_MODULES.txt \
            backend/licenses/GO_LICENSES.csv backend/LICENSE \
            frontend/licenses/npm/inventory.json \
            frontend/licenses/npm/THIRD_PARTY_NOTICES.txt \
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

grep -Fq 'golang:1.25-trixie@sha256:2c4c60ef415fbfa5e90300722293bef36c5e63fae17570ce18f580af933dbd73' backend/Dockerfile || fail "backend Go base image is not digest-pinned"
grep -Fq 'debian:trixie-slim@sha256:020c0d20b9880058cbe785a9db107156c3c75c2ac944a6aa7ab59f2add76a7bd' backend/Dockerfile || fail "backend Debian base image is not digest-pinned"
grep -Fq 'ARG LIBX264_PACKAGE_VERSION=2:0.164.3108+git31e19f9-2+b1' backend/Dockerfile || fail "x264 package version is not pinned"
grep -Fq 'ARG LIBX265_PACKAGE_VERSION=4.1-2' backend/Dockerfile || fail "x265 package version is not pinned"
grep -Fq 'ARG LIBBLURAY_PACKAGE_VERSION=1:1.3.4-1+b2' backend/Dockerfile || fail "Blu-ray package version is not pinned"
grep -Fq 'ARG FRIBIDI_PACKAGE_VERSION=1.0.16-1' backend/Dockerfile || fail "FriBidi package version is not pinned"
grep -Fq 'ARG LAME_PACKAGE_VERSION=3.100-6+b3' backend/Dockerfile || fail "LAME package version is not pinned"
grep -Fq 'ARG SOXR_PACKAGE_VERSION=0.1.3-4+b2' backend/Dockerfile || fail "SoXR package version is not pinned"
grep -Fq 'ARG ZIMG_PACKAGE_VERSION=3.0.5+ds1-1+b2' backend/Dockerfile || fail "zimg package version is not pinned"
grep -Fq 'apt-get source "x264=${X264_SOURCE_VERSION}" "x265=${X265_SOURCE_VERSION}"' backend/Dockerfile || fail "corresponding x264/x265 source is not built"
grep -Fq '"libbluray=${LIBBLURAY_SOURCE_VERSION}"' backend/Dockerfile || fail "matching libbluray source is not included"
grep -Fq '"lame=${LAME_SOURCE_VERSION}"' backend/Dockerfile || fail "matching LAME source is not included"
grep -Fq '"libsoxr=${SOXR_SOURCE_VERSION}"' backend/Dockerfile || fail "matching SoXR source is not included"
grep -Fq '"fribidi=${FRIBIDI_SOURCE_VERSION}"' backend/Dockerfile || fail "matching FriBidi source is not included"
grep -Fq '"zimg=${ZIMG_SOURCE_VERSION}"' backend/Dockerfile || fail "matching zimg source is not included"
grep -Fq 'FROM scratch AS corresponding-source' backend/Dockerfile || fail "backend Dockerfile lacks the corresponding-source target"
grep -Fq 'cp /src/vpl-gpu/LICENSE' backend/Dockerfile || fail "oneVPL Intel GPU runtime license is not copied"
grep -Fq 'tar -xOzf /tmp/seconv.tar.gz ./LICENSE' backend/Dockerfile || fail "SeConv license is not copied from its release archive"
grep -Fq 'THIRD[-_. ]PARTY[-_. ]NOTICES?' backend/Dockerfile || fail "NuGet package notice extraction is missing"
grep -Fq 'find /usr/share/doc -type f -name copyright -print' backend/Dockerfile || fail "Debian package copyright files are not copied into the backend image"
grep -Fq 'test -s "/usr/share/licenses/mvforge/debian/${package}.copyright"' backend/Dockerfile || fail "backend image does not require key Debian package copyright files"

cmp -s LICENSE backend/LICENSE || fail "backend module license differs from repository LICENSE"
if ! diff -q backend/licenses/GO_MODULES.txt <(cd backend && go list -m all) >/dev/null 2>&1; then
  fail "resolved Go module inventory is stale"
fi
if ! diff -q backend/licenses/GO_LICENSES.csv <(cd backend && go run github.com/google/go-licenses@v1.6.0 report ./...) >/dev/null 2>&1; then
  fail "Go package/license report is stale or cannot be reproduced"
fi
grep -Fq ',Unknown' backend/licenses/GO_LICENSES.csv && fail "Go license report contains an Unknown license"
go_report_rows="$(wc -l < backend/licenses/GO_LICENSES.csv | tr -d '[:space:]')"
go_notice_files="$(find backend/licenses/go-modules -type f -print | wc -l | tr -d '[:space:]')"
[[ "$go_report_rows" -gt 0 && "$go_report_rows" = "$go_notice_files" ]] || fail "Go report and bundled license-text counts differ"

grep -Fq 'no distribuye contenido' README.md || fail "README is missing the content-distribution boundary"
grep -Fiq 'no implementa ni incluye herramientas para evadir DRM' README.md || fail "README is missing the DRM non-goal"
grep -Fq 'LEGAL.md' README.md || fail "README does not link the legal boundaries"
grep -Fq 'Radarr/Sonarr' README.md || fail "README does not position MVForge alongside Radarr/Sonarr"
grep -Fq 'backend/go.mod' THIRD_PARTY_LICENSES.md || fail "third-party inventory does not identify the backend Go manifest"
grep -Fq 'frontend/package-lock.json' THIRD_PARTY_LICENSES.md || fail "third-party inventory does not identify the frontend npm lockfile"

python3 -c 'import json,sys; lock=json.load(open("frontend/package-lock.json", encoding="utf-8")); packages=[(name,entry) for name,entry in lock.get("packages",{}).items() if name.startswith("node_modules/")]; missing=[name for name,entry in packages if not entry.get("license")]; print(f"npm lock license metadata: {len(packages)} resolved packages, {len(missing)} missing"); sys.exit(bool(missing) or not packages)'
node frontend/scripts/generate-license-inventory.mjs --check --require-complete || fail "npm runtime license evidence is stale or incomplete"
frontend_context_line="$(grep -n -m 1 '^COPY \. \.$' frontend/Dockerfile | cut -d: -f1)"
frontend_inventory_line="$(grep -n -m 1 'npm run compliance:licenses -- --require-complete' frontend/Dockerfile | cut -d: -f1)"
[[ -n "$frontend_context_line" && -n "$frontend_inventory_line" && "$frontend_context_line" -lt "$frontend_inventory_line" ]] || fail "frontend Dockerfile must copy the generator before it runs"
grep -Fq 'COPY --from=build /app/licenses/npm/ /usr/share/nginx/html/licenses/npm/' frontend/Dockerfile || fail "web runtime image does not include npm notice bundle"
grep -Fq 'node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402' frontend/Dockerfile || fail "frontend Node base image is not digest-pinned"
grep -Fq 'nginx:1.28-alpine@sha256:a8b39bd9cf0f83869a2162827a0caf6137ddf759d50a171451b335cecc87d236' frontend/Dockerfile || fail "frontend Nginx base image is not digest-pinned"

grep -Fq './scripts/verify-compliance.sh' .github/workflows/ci.yml || fail "CI does not run the compliance verifier"
grep -Fq './scripts/verify-compliance.sh' .github/workflows/release-images.yml || fail "release workflow does not run the compliance verifier"
grep -Fq 'sbom: true' .github/workflows/release-images.yml || fail "release images do not request an SBOM attestation"
grep -Fq 'provenance: mode=max' .github/workflows/release-images.yml || fail "release images do not request provenance"
grep -Fq 'MVFORGE_PUBLIC_RELEASES_ENABLED:-false' .github/workflows/release-images.yml || fail "release gate must default to closed"
grep -Fq '!= "true"' .github/workflows/release-images.yml || fail "release gate must require an explicit true value"
grep -Fq 'MVFORGE_EXISTING_DISTRIBUTIONS_REVIEWED:-false' .github/workflows/release-images.yml || fail "release gate must require external review of retained distributions"
grep -Fq 'build-corresponding-source.sh' .github/workflows/release-images.yml || fail "release workflow does not generate corresponding source"
grep -Fq 'verify-release-evidence.sh' .github/workflows/release-images.yml || fail "release workflow does not verify release-side evidence"
grep -Fq 'verify-image-attestations.sh' .github/workflows/release-images.yml || fail "release workflow does not verify image attestations"
[[ -x scripts/verify-image-attestations.sh ]] || fail "image attestation verifier is not executable"
[[ -x scripts/capture-backend-compliance-evidence.sh ]] || fail "backend build evidence capture is not executable"
[[ -x scripts/capture-frontend-compliance-evidence.sh ]] || fail "frontend image evidence capture is not executable"
grep -Fq -- '-buildconf' scripts/capture-backend-compliance-evidence.sh || fail "backend evidence capture omits ffmpeg -buildconf"
grep -Fq 'ldd /opt/ffmpeg/lib/libavcodec.so' scripts/capture-backend-compliance-evidence.sh || fail "backend evidence capture omits FFmpeg linkage"
for notice in LICENSE NOTICE LEGAL.md THIRD_PARTY_LICENSES.md; do
  grep -Fq "$notice" .github/workflows/release-images.yml || fail "release workflow omits $notice"
done

if [[ "${MVFORGE_REQUIRE_RELEASE_EVIDENCE:-false}" == "true" ]]; then
  [[ -n "${MVFORGE_RELEASE_EVIDENCE_DIR:-}" ]] || fail "MVFORGE_RELEASE_EVIDENCE_DIR is required in release-evidence mode"
  ./scripts/verify-release-evidence.sh "$MVFORGE_RELEASE_EVIDENCE_DIR"
fi

printf 'Compliance baseline checks passed.\n'
