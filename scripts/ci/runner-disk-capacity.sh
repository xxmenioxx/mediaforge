#!/usr/bin/env bash
set -euo pipefail

readonly minimum_free_bytes=$((10 * 1024 * 1024 * 1024))

usage() {
  cat >&2 <<'EOF'
Usage:
  runner-disk-capacity.sh report <phase> [--buildkit]
  runner-disk-capacity.sh check <phase> [--buildkit]
  runner-disk-capacity.sh reclaim
  runner-disk-capacity.sh prune-buildkit
EOF
}

available_bytes() {
  local free_bytes

  free_bytes="$(df -P -B1 / | awk 'NR == 2 { print $4 }')"
  if [[ ! "$free_bytes" =~ ^[0-9]+$ ]]; then
    printf 'Unable to determine available bytes on / from df output.\n' >&2
    return 1
  fi
  printf '%s\n' "$free_bytes"
}

format_gib() {
  awk -v bytes="$1" 'BEGIN { printf "%.2f GiB", bytes / 1073741824 }'
}

report_storage() {
  local phase="$1"
  local include_buildkit="${2:-false}"
  local free_bytes
  local diagnostics_status=0

  printf '\n::group::Runner storage diagnostics: %s\n' "$phase"
  printf '%s\n' 'Filesystem capacity (df -h):'
  df -h || diagnostics_status=1
  printf '\n%s\n' 'Filesystem inode capacity (df -i):'
  df -i || diagnostics_status=1
  printf '\n%s\n' 'Docker image and cache usage (docker system df):'
  docker system df || diagnostics_status=1
  if [[ "$include_buildkit" == true ]]; then
    printf '\n%s\n' 'Selected Buildx builder cache (docker buildx du):'
    docker buildx du || diagnostics_status=1
  fi

  if free_bytes="$(available_bytes)"; then
    printf '\nAvailable bytes on /: %s (%s)\n' "$free_bytes" "$(format_gib "$free_bytes")"
  else
    diagnostics_status=1
  fi
  printf '::endgroup::\n'
  return "$diagnostics_status"
}

cleanup_target() {
  local target="$1"
  local canonical_path
  local allocated_bytes

  case "$target" in
    /opt/hostedtoolcache/CodeQL|/opt/hostedtoolcache/dotnet|/usr/local/lib/android|/usr/local/.ghcup|/usr/local/share/boost|/usr/share/dotnet)
      ;;
    *)
      printf 'Refusing non-allowlisted cleanup target: %s\n' "$target" >&2
      return 1
      ;;
  esac

  if [[ -L "$target" ]]; then
    printf 'Skipping symlink cleanup target: %s\n' "$target"
    return 0
  fi
  if [[ ! -d "$target" ]]; then
    printf 'Cleanup target not present: %s\n' "$target"
    return 0
  fi

  canonical_path="$(realpath -e -- "$target")"
  if [[ "$canonical_path" != "$target" ]]; then
    printf 'Refusing cleanup target whose resolved path changed: %s -> %s\n' \
      "$target" "$canonical_path" >&2
    return 1
  fi

  allocated_bytes="$(sudo du -s -B1 "$canonical_path" | awk '{ print $1 }')"
  printf 'Removing unused hosted-runner toolchain %s (%s allocated bytes)\n' \
    "$canonical_path" "$allocated_bytes"
  sudo find "$canonical_path" -xdev -depth -delete
}

reclaim_hosted_runner_space() {
  local before_bytes
  local after_bytes

  if [[ "${RUNNER_ENVIRONMENT:-}" != github-hosted ]]; then
    printf 'Refusing hosted-runner cleanup: RUNNER_ENVIRONMENT is not github-hosted.\n' >&2
    return 1
  fi

  before_bytes="$(available_bytes)"
  printf 'Available disk space before cleanup: %s (%s)\n' \
    "$before_bytes" "$(format_gib "$before_bytes")"

  # This workflow uses Go, Node and Docker; these runner-installed toolchains
  # are not inputs to either image build or the NAS Compose validation.
  sudo apt-get clean
  cleanup_target /opt/hostedtoolcache/CodeQL
  cleanup_target /opt/hostedtoolcache/dotnet
  cleanup_target /usr/local/lib/android
  cleanup_target /usr/local/.ghcup
  cleanup_target /usr/local/share/boost
  cleanup_target /usr/share/dotnet

  after_bytes="$(available_bytes)"
  printf 'Available disk space after cleanup: %s (%s); net filesystem recovery: %s (%s)\n' \
    "$after_bytes" "$(format_gib "$after_bytes")" \
    "$((after_bytes - before_bytes))" "$(format_gib "$((after_bytes - before_bytes))")"
}

require_backend_headroom() {
  local phase="$1"
  local include_buildkit="${2:-false}"
  local free_bytes

  report_storage "$phase" "$include_buildkit"
  free_bytes="$(available_bytes)"
  if (( free_bytes < minimum_free_bytes )); then
    printf 'Insufficient runner disk before backend build: %s free; at least %s is required after cleanup.\n' \
      "$(format_gib "$free_bytes")" "$(format_gib "$minimum_free_bytes")" >&2
    printf '%s\n' \
      'The hosted runner cleanup did not recover the documented 10 GiB build headroom.' \
      'Use a runner with more storage or a prebuilt media-dependency image; do not skip the image build.' >&2
    return 1
  fi
  printf 'Backend build headroom check passed: %s free (minimum %s).\n' \
    "$(format_gib "$free_bytes")" "$(format_gib "$minimum_free_bytes")"
}

prune_job_buildkit_cache() {
  if [[ "${RUNNER_ENVIRONMENT:-}" != github-hosted ]]; then
    printf 'Refusing BuildKit cache cleanup: RUNNER_ENVIRONMENT is not github-hosted.\n' >&2
    return 1
  fi

  printf '%s\n' \
    'Pruning all cache records from the current setup-buildx-action builder.' \
    'The backend build action has completed its type=gha cache export; the web build uses a separate cache scope.'
  docker buildx prune --all --force
}

command="${1:-}"
case "$command" in
  report)
    if (( $# < 2 || $# > 3 )); then usage; exit 2; fi
    include_buildkit=false
    if [[ "${3:-}" == --buildkit ]]; then include_buildkit=true; fi
    if (( $# == 3 )) && [[ "$include_buildkit" != true ]]; then usage; exit 2; fi
    report_storage "$2" "$include_buildkit"
    ;;
  check)
    if (( $# < 2 || $# > 3 )); then usage; exit 2; fi
    include_buildkit=false
    if [[ "${3:-}" == --buildkit ]]; then include_buildkit=true; fi
    if (( $# == 3 )) && [[ "$include_buildkit" != true ]]; then usage; exit 2; fi
    require_backend_headroom "$2" "$include_buildkit"
    ;;
  reclaim)
    if (( $# != 1 )); then usage; exit 2; fi
    reclaim_hosted_runner_space
    ;;
  prune-buildkit)
    if (( $# != 1 )); then usage; exit 2; fi
    prune_job_buildkit_cache
    ;;
  *)
    usage
    exit 2
    ;;
esac
