#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'release evidence check failed: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 1 ]] || fail "usage: $0 /path/to/release-evidence"
evidence_dir="$1"
archive="$evidence_dir/mvforge-corresponding-source.tar.gz"

for path in \
  "$archive" "$archive.sha256" \
  "$evidence_dir/LICENSE" "$evidence_dir/NOTICE" "$evidence_dir/LEGAL.md" \
  "$evidence_dir/THIRD_PARTY_LICENSES.md" \
  "$evidence_dir/GO_MODULES.txt" "$evidence_dir/GO_LICENSES.csv" \
  "$evidence_dir/GO_LICENSES.md" \
  "$evidence_dir/frontend-NPM-inventory.json" \
  "$evidence_dir/frontend-THIRD_PARTY_NOTICES.txt"; do
  [[ -s "$path" ]] || fail "required release evidence is missing: $path"
done

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$evidence_dir" && sha256sum -c mvforge-corresponding-source.tar.gz.sha256) || fail "source archive checksum mismatch"
else
  (cd "$evidence_dir" && shasum -a 256 -c mvforge-corresponding-source.tar.gz.sha256) || fail "source archive checksum mismatch"
fi

tar_listing="$(tar -tzf "$archive")"
grep -Fxq 'sources/ffmpeg/ffmpeg-8.1.2.tar.xz' <<<"$tar_listing" || fail "source bundle omits FFmpeg 8.1.2"
grep -Fxq 'sources/svt-av1/SVT-AV1-4.1.0.tar.gz' <<<"$tar_listing" || fail "source bundle omits SVT-AV1 4.1.0"
grep -Eq '(^|/)x264_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact x264 Debian source"
grep -Eq '(^|/)x265_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact x265 Debian source"
grep -Eq '(^|/)libbluray_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact libbluray Debian source"
grep -Eq '(^|/)lame_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact LAME Debian source"
grep -Eq '(^|/)libsoxr_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact SoXR Debian source"
grep -Eq '(^|/)fribidi_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact FriBidi Debian source"
grep -Eq '(^|/)zimg_.*\.dsc$' <<<"$tar_listing" || fail "source bundle omits exact zimg Debian source"
grep -Fxq 'build/backend.Dockerfile' <<<"$tar_listing" || fail "source bundle omits the backend Dockerfile"
grep -Fxq 'build/scripts/build-corresponding-source.sh' <<<"$tar_listing" || fail "source bundle omits its source build script"
grep -Fxq 'build/scripts/package-corresponding-source.py' <<<"$tar_listing" || fail "source bundle omits its packaging script"
grep -Fxq 'notices/backend/SOURCE-OFFER.md' <<<"$tar_listing" || fail "source bundle omits its source manifest"
grep -Fxq 'notices/backend/NOTICE' <<<"$tar_listing" || fail "source bundle omits the backend notice"
grep -Fxq 'notices/backend/THIRD_PARTY_LICENSES.md' <<<"$tar_listing" || fail "source bundle omits the third-party inventory"
grep -Fxq 'notices/runtime/seconv/LICENSE' <<<"$tar_listing" || fail "source bundle omits the SeConv license"
grep -Eq '^notices/runtime/skiasharp/.+' <<<"$tar_listing" || fail "source bundle omits SkiaSharp notices"
grep -Eq '^notices/runtime/harfbuzzsharp/.+' <<<"$tar_listing" || fail "source bundle omits HarfBuzzSharp notices"
grep -Fxq 'notices/intel/oneVPL-intel-gpu/LICENSE' <<<"$tar_listing" || fail "source bundle omits the oneVPL Intel GPU runtime license"
grep -Fxq 'SHA256SUMS' <<<"$tar_listing" || fail "source bundle omits its content checksums"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/mvforge-release-evidence.XXXXXX")"
trap 'find "$temporary_dir" -type f -delete; find "$temporary_dir" -depth -type d -empty -delete' EXIT
tar -xzf "$archive" -C "$temporary_dir"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$temporary_dir" && sha256sum -c SHA256SUMS >/dev/null) || fail "source bundle content checksum mismatch"
else
  (cd "$temporary_dir" && shasum -a 256 -c SHA256SUMS >/dev/null) || fail "source bundle content checksum mismatch"
fi

go_report_rows="$(wc -l < "$evidence_dir/GO_LICENSES.csv" | tr -d '[:space:]')"
go_notice_files="$(grep -Ec '^notices/backend/go-modules/.+[^/]$' <<<"$tar_listing")"
[[ "$go_report_rows" -gt 0 && "$go_report_rows" = "$go_notice_files" ]] || fail "source bundle Go inventory and notice-text counts differ"

grep -Fq ',Unknown' "$evidence_dir/GO_LICENSES.csv" && fail "Go inventory has an Unknown license"
jq -e '(.unresolved | length) == 0' "$evidence_dir/frontend-NPM-inventory.json" >/dev/null || fail "npm inventory has unresolved license evidence"

printf 'Release source, notice, inventory, and checksum evidence is present.\n'
