#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'corresponding-source build failed: %s\n' "$*" >&2
  exit 1
}

if [[ $# -ne 1 ]]; then
  fail "usage: $0 /path/to/mvforge-corresponding-source.tar.gz"
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
archive_path="$1"
contents_path="${archive_path}.contents"
archive_parent="$(dirname "$archive_path")"
# The default is the checked-in release Dockerfile. The override is for local
# validation when the external BuildKit syntax image cannot be fetched.
dockerfile_path="${MVFORGE_BACKEND_DOCKERFILE:-Dockerfile}"
mkdir -p "$archive_parent"
[[ ! -e "$archive_path" ]] || fail "output archive already exists: $archive_path"
[[ ! -e "${archive_path}.sha256" ]] || fail "output checksum already exists: ${archive_path}.sha256"
[[ ! -e "$contents_path" ]] || fail "output directory already exists: $contents_path"

(
  cd "$repo_root/backend"
  docker buildx build \
    --platform linux/amd64 \
    --file "$dockerfile_path" \
    --target corresponding-source \
    --output "type=local,dest=${contents_path}" \
    .
)

[[ -s "$contents_path/sources/ffmpeg/ffmpeg-8.1.2.tar.xz" ]] || fail "FFmpeg source archive missing"
[[ -s "$contents_path/sources/svt-av1/SVT-AV1-4.1.0.tar.gz" ]] || fail "SVT-AV1 source archive missing"
find "$contents_path/sources/debian" -type f -name 'x264_*.dsc' -print | grep -q . || fail "matching x264 Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'x265_*.dsc' -print | grep -q . || fail "matching x265 Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'libbluray_*.dsc' -print | grep -q . || fail "matching libbluray Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'lame_*.dsc' -print | grep -q . || fail "matching LAME Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'libsoxr_*.dsc' -print | grep -q . || fail "matching SoXR Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'fribidi_*.dsc' -print | grep -q . || fail "matching FriBidi Debian source descriptor missing"
find "$contents_path/sources/debian" -type f -name 'zimg_*.dsc' -print | grep -q . || fail "matching zimg Debian source descriptor missing"
[[ -s "$contents_path/notices/runtime/seconv/LICENSE" ]] || fail "SeConv license notice missing"
[[ -n "$(find "$contents_path/notices/runtime/skiasharp" -type f -print -quit)" ]] || fail "SkiaSharp notices missing"
[[ -n "$(find "$contents_path/notices/runtime/harfbuzzsharp" -type f -print -quit)" ]] || fail "HarfBuzzSharp notices missing"
[[ -s "$contents_path/notices/backend/SOURCE-OFFER.md" ]] || fail "source manifest missing"
install -d "$contents_path/build/scripts"
install -m 0644 "$repo_root/scripts/build-corresponding-source.sh" "$contents_path/build/scripts/"
install -m 0644 "$repo_root/scripts/package-corresponding-source.py" "$contents_path/build/scripts/"

python3 "$repo_root/scripts/package-corresponding-source.py" "$contents_path" "$archive_path"
printf 'Built corresponding-source archive: %s\n' "$archive_path"
