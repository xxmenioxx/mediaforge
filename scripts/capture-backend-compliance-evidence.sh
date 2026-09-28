#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'backend evidence capture failed: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 2 || ( $# -eq 3 && "$3" == "--replace" ) ]] || fail "usage: $0 IMAGE_TAG OUTPUT_FILE [--replace]"
image="$1"
output_path="$2"
replace_output=false
[[ $# -eq 3 ]] && replace_output=true
[[ ! -e "$output_path" || "$replace_output" == true ]] || fail "refusing to overwrite evidence file: $output_path"
command -v docker >/dev/null 2>&1 || fail "docker is required"

metadata="$(docker image inspect "$image" --format 'image={{.Id}} platform={{.Os}}/{{.Architecture}} created={{.Created}}')" \
  || fail "local image is unavailable: $image"
version="$(docker run --rm --entrypoint /opt/ffmpeg/bin/ffmpeg "$image" -hide_banner -version)" \
  || fail "could not execute ffmpeg -version in $image"
buildconf="$(docker run --rm --entrypoint /opt/ffmpeg/bin/ffmpeg "$image" -hide_banner -buildconf)" \
  || fail "could not execute ffmpeg -buildconf in $image"
grep -Fq -- '--enable-gpl' <<<"$buildconf" || fail "built FFmpeg omits --enable-gpl"
grep -Fq -- '--enable-version3' <<<"$buildconf" || fail "built FFmpeg omits --enable-version3"
grep -Fq -- '--enable-libx264' <<<"$buildconf" || fail "built FFmpeg omits --enable-libx264"
grep -Fq -- '--enable-libx265' <<<"$buildconf" || fail "built FFmpeg omits --enable-libx265"
grep -Fq -- '--enable-nonfree' <<<"$buildconf" && fail "built FFmpeg unexpectedly enables --enable-nonfree"

linkage="$(docker run --rm --entrypoint sh "$image" -c 'ldd /opt/ffmpeg/lib/libavcodec.so')" \
  || fail "could not inspect libavcodec linkage"
for library in libx264.so libx265.so libva.so libvpl.so libva-drm.so libdrm.so libOpenCL.so; do
  grep -Fq "$library" <<<"$linkage" || fail "expected runtime linkage missing: $library"
done
hwaccels="$(docker run --rm --entrypoint /opt/ffmpeg/bin/ffmpeg "$image" -hide_banner -hwaccels)" \
  || fail "could not query FFmpeg hardware acceleration methods"
grep -Fxq 'vaapi' <<<"$hwaccels" || fail "FFmpeg does not report VAAPI"
grep -Fxq 'qsv' <<<"$hwaccels" || fail "FFmpeg does not report QSV"

package_versions="$(docker run --rm --entrypoint dpkg-query "$image" \
  -W -f='${binary:Package}\t${Version}\n')" \
  || fail "could not query the complete runtime package inventory"
license_metadata="$(docker run --rm --entrypoint sh "$image" -c '
  for package in libx264-164 libx265-215 libaom3 libdav1d7 libvpx9 libbluray2 libfribidi0 libmp3lame0 libopus0 libsoxr0 libzimg2 libopenjp2-7; do
    grep -H -m 4 "^License:" "/usr/share/licenses/mvforge/debian/${package}.copyright"
  done')" || fail "could not read runtime Debian license metadata"
debian_notices="$(docker run --rm --entrypoint sh "$image" -c \
  "find /usr/share/licenses/mvforge/debian -maxdepth 1 -type f -name '*.copyright' -print")" \
  || fail "could not list copied Debian package copyright files"
for package in libx264-164 libx265-215 libaom3 libdav1d7 libvpx9 libbluray2 libfribidi0 libmp3lame0 libopus0 libsoxr0 libzimg2 libopenjp2-7; do
  grep -Fq "/${package}.copyright" <<<"$debian_notices" || fail "runtime image omits Debian copyright evidence for $package"
done
component_notices="$(docker run --rm --entrypoint sh "$image" -c \
  "find /usr/share/licenses/mvforge -type f -print | grep -E '(ffmpeg/COPYING.GPLv3|svt-av1/LICENSE|seconv/LICENSE|skiasharp/|harfbuzzsharp/|oneVPL-intel-gpu/LICENSE)'")" \
  || fail "could not list component notices"
for notice in \
  /usr/share/licenses/mvforge/ffmpeg/COPYING.GPLv3 \
  /usr/share/licenses/mvforge/seconv/LICENSE \
  /usr/share/licenses/mvforge/skiasharp/LICENSE.txt \
  /usr/share/licenses/mvforge/harfbuzzsharp/LICENSE.txt \
  /usr/share/licenses/mvforge/oneVPL-intel-gpu/LICENSE; do
  grep -Fxq "$notice" <<<"$component_notices" || fail "runtime image omits required notice $notice"
done
intel_files="$(docker run --rm --entrypoint sh "$image" -c \
  "find /opt/intel/lib/x86_64-linux-gnu -type f | grep -E '/(iHD_drv_video\.so|libvpl\.so|libmfx-gen\.so)'")" \
  || fail "could not inspect Intel QSV/VAAPI runtime files"
grep -Fq 'iHD_drv_video.so' <<<"$intel_files" || fail "Intel VAAPI driver file is missing"

mkdir -p "$(dirname "$output_path")"
temporary_output="$(mktemp "${output_path}.tmp.XXXXXX")"
trap 'rm -f "$temporary_output"' EXIT
{
  printf 'Backend container technical compliance evidence\n%s\n\n' "$metadata"
  printf '%s\n%s\n\n' '--- ffmpeg -version ---' "$version"
  printf '%s\n\n' '--- ffmpeg -buildconf ---'
  printf '%s\n\n' "$buildconf"
  printf '%s\n\n' '--- ldd /opt/ffmpeg/lib/libavcodec.so ---'
  grep -E '(x264|x265|va|vpl|drm|OpenCL|aom|dav1d|SvtAv1|vpx)' <<<"$linkage"
  printf '%s\n\n' '--- ffmpeg -hwaccels ---'
  printf '%s\n\n' "$hwaccels"
  printf '%s\n\n' '--- runtime Debian package versions ---'
  printf '%s\n\n' "$package_versions"
  printf '%s\n\n' '--- Debian copyright License: metadata for directly linked libraries ---'
  printf '%s\n\n' "$license_metadata"
  printf '%s\n\n' '--- copied runtime notice paths ---'
  printf '%s\n' "$component_notices"
  printf '%s\n\n' "$debian_notices"
  printf '%s\n' '--- Intel QSV/VAAPI runtime files ---'
  printf '%s\n' "$intel_files"
  printf '\nGPU hardware was not attached to the local container; QSV/VAAPI execution was not tested.\n'
} > "$temporary_output"
mv -f "$temporary_output" "$output_path"
trap - EXIT
printf 'Wrote backend technical evidence: %s\n' "$output_path"
