#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'frontend evidence capture failed: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 2 || ( $# -eq 3 && "$3" == "--replace" ) ]] || fail "usage: $0 IMAGE_TAG OUTPUT_FILE [--replace]"
image="$1"
output_path="$2"
replace_output=false
[[ $# -eq 3 ]] && replace_output=true
[[ ! -e "$output_path" || "$replace_output" == true ]] || fail "refusing to overwrite evidence file: $output_path"
command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"

metadata="$(docker image inspect "$image" --format 'image={{.Id}} platform={{.Os}}/{{.Architecture}} created={{.Created}}')" \
  || fail "local image is unavailable: $image"
docker run --rm --entrypoint sh "$image" -c \
  'test -s /usr/share/nginx/html/licenses/npm/inventory.json && test -s /usr/share/nginx/html/licenses/npm/THIRD_PARTY_NOTICES.txt' \
  || fail "npm license inventory or notice bundle is missing from the runtime image"
inventory="$(docker run --rm --entrypoint cat "$image" /usr/share/nginx/html/licenses/npm/inventory.json)" \
  || fail "could not read npm inventory from runtime image"
jq -e '(.packages | length) > 0 and (.unresolved | length) == 0' <<<"$inventory" >/dev/null \
  || fail "runtime npm inventory is empty or has unresolved entries"
inventory_summary="$(jq -r '"packages=\(.packages | length) unresolved=\(.unresolved | length)"' <<<"$inventory")"
notice_summary="$(docker run --rm --entrypoint wc "$image" -lc /usr/share/nginx/html/licenses/npm/THIRD_PARTY_NOTICES.txt)" \
  || fail "could not count npm notice lines in runtime image"

mkdir -p "$(dirname "$output_path")"
temporary_output="$(mktemp "${output_path}.tmp.XXXXXX")"
trap 'rm -f "$temporary_output"' EXIT
{
  printf 'Frontend container technical compliance evidence\n%s\n\n' "$metadata"
  printf '%s\n' '--- shipped npm inventory ---' "$inventory_summary"
  printf '\n%s\n' '--- shipped third-party notice file ---' "$notice_summary"
  printf '\n%s\n' '/usr/share/nginx/html/licenses/npm/inventory.json'
  printf '%s\n' '/usr/share/nginx/html/licenses/npm/THIRD_PARTY_NOTICES.txt'
} > "$temporary_output"
mv "$temporary_output" "$output_path"
trap - EXIT
printf 'Wrote frontend technical evidence: %s\n' "$output_path"
