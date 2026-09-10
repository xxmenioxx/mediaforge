#!/usr/bin/env bash

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mvforge-deploy-dev.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

DOCKER_CONTEXT="${MVFORGE_DEV_DOCKER_CONTEXT:-mvforge-dev-nas}"
NAS_SSH_HOST="${MVFORGE_DEV_NAS_SSH_HOST:-mvs-nas}"
NAS_STACK_DIR="${MVFORGE_NAS_STACK_DIR:-/volume1/docker/nas-media-stack}"
DEV_VERSION="${MVFORGE_DEV_VERSION:-dev}"
VERIFY_SCRIPT="$ROOT_DIR/scripts/verify.sh"

BACKEND_IMAGE=""
WEB_IMAGE=""
START_TIME="$(date +%s)"

usage() {
    cat <<'USAGE'
Usage:
  ./scripts/deploy-dev.sh [mode]

Modes:
  --auto       Validate with verify.sh --auto and deploy detected scope
  --backend    Validate, build and deploy backend only
  --frontend   Validate, build and deploy frontend only
  --full       Validate, build and deploy backend + frontend
  --status     Compare local source, :dev images and running containers
  --restore    Restore release version from nas-media-stack/.env
  --help       Show help

Default:
  --auto

Environment overrides:
  MVFORGE_DEV_DOCKER_CONTEXT   Default: mvforge-dev-nas
  MVFORGE_DEV_NAS_SSH_HOST    Default: mvs-nas
  MVFORGE_NAS_STACK_DIR       Default: /volume1/docker/nas-media-stack
  MVFORGE_DEV_VERSION         Default: dev

Safety:
  - Never pushes images.
  - Never creates Git tags or GitHub releases.
  - Never edits nas-media-stack/.env.
  - Dev deployment uses --pull never.
USAGE
}

fail() {
    printf '\nERROR: %s\n' "$*" >&2
    exit 1
}

run_logged() {
    local name="$1"
    shift
    local log_file="$TMP_DIR/${name}.log"

    if "$@" >"$log_file" 2>&1; then
        return 0
    fi

    printf '\nFAILED: %s\n\n' "$name" >&2
    printf '%s\n' '---- failure output (last 60 lines) ----' >&2
    tail -n 60 "$log_file" >&2
    printf '%s\n' '---- end failure output ----' >&2
    exit 1
}

quote_remote() {
    local value="$1"
    printf "'%s'" "${value//\'/\'\\\'\'}"
}

remote_stack_command() {
    local command="$1"
    local quoted_dir
    quoted_dir="$(quote_remote "$NAS_STACK_DIR")"
    ssh "$NAS_SSH_HOST" "cd ${quoted_dir} && ${command}"
}

image_repository() {
    local ref="$1"
    ref="${ref%@*}"

    if [[ "$ref" =~ ^(.+):[^/:]+$ ]]; then
        printf '%s' "${BASH_REMATCH[1]}"
    else
        printf '%s' "$ref"
    fi
}

resolve_dev_images() {
    local backend_current web_current backend_repo web_repo

    backend_current="$(
        docker --context "$DOCKER_CONTEXT" inspect mediaforge-backend \
            --format '{{.Config.Image}}' 2>/dev/null
    )" || fail "Cannot inspect running container: mediaforge-backend"

    web_current="$(
        docker --context "$DOCKER_CONTEXT" inspect mediaforge \
            --format '{{.Config.Image}}' 2>/dev/null
    )" || fail "Cannot inspect running container: mediaforge"

    backend_repo="$(image_repository "$backend_current")"
    web_repo="$(image_repository "$web_current")"

    [[ -n "$backend_repo" ]] || fail "Could not resolve backend image repository."
    [[ -n "$web_repo" ]] || fail "Could not resolve frontend image repository."

    BACKEND_IMAGE="${backend_repo}:${DEV_VERSION}"
    WEB_IMAGE="${web_repo}:${DEV_VERSION}"
}

preflight() {
    command -v docker >/dev/null 2>&1 || fail "docker CLI is not installed."
    command -v ssh >/dev/null 2>&1 || fail "ssh is not installed."
    command -v git >/dev/null 2>&1 || fail "git is not installed."

    [[ -x "$VERIFY_SCRIPT" ]] || fail "Missing or non-executable validator: $VERIFY_SCRIPT"
    [[ -f "$ROOT_DIR/backend/Dockerfile" ]] || fail "Missing backend/Dockerfile"
    [[ -f "$ROOT_DIR/frontend/Dockerfile" ]] || fail "Missing frontend/Dockerfile"

    git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
        || fail "Repository root is not a Git working tree: $ROOT_DIR"

    docker context inspect "$DOCKER_CONTEXT" >/dev/null 2>&1 \
        || fail "Docker context not found: $DOCKER_CONTEXT"

    docker --context "$DOCKER_CONTEXT" info >/dev/null 2>&1 \
        || fail "Cannot connect to Docker context: $DOCKER_CONTEXT"

    ssh "$NAS_SSH_HOST" "test -f $(quote_remote "$NAS_STACK_DIR/compose.yaml")" \
        || fail "compose.yaml not found: $NAS_STACK_DIR"

    ssh "$NAS_SSH_HOST" "test -f $(quote_remote "$NAS_STACK_DIR/.env")" \
        || fail ".env not found: $NAS_STACK_DIR"

    remote_stack_command \
        "docker compose --env-file .env config --services | grep -qx 'mediaforge-backend'" \
        >/dev/null 2>&1 \
        || fail "mediaforge-backend service is not available."

    remote_stack_command \
        "docker compose --env-file .env config --services | grep -qx 'mediaforge'" \
        >/dev/null 2>&1 \
        || fail "mediaforge service is not available."

    resolve_dev_images
}

verify_repository() {
    local verify_mode="$1"
    local log_file="$TMP_DIR/verify.log"

    if "$VERIFY_SCRIPT" "$verify_mode" >"$log_file" 2>&1; then
        return 0
    fi

    cat "$log_file" >&2
    exit 1
}

resolve_scope() {
    local requested_mode="$1"
    local scope

    case "$requested_mode" in
        --backend)  printf '%s' backend; return ;;
        --frontend) printf '%s' frontend; return ;;
        --full)     printf '%s' full; return ;;
    esac

    scope="$(awk '/^Scope:[[:space:]]+/ { value=$2 } END { print value }' "$TMP_DIR/verify.log")"

    case "$scope" in
        backend|frontend|full) printf '%s' "$scope" ;;
        *) fail "Could not determine deployment scope from verify.sh --auto." ;;
    esac
}

git_short_sha() {
    git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown'
}

git_scope_dirty() {
    local scope="$1"
    if [[ -n "$(git -C "$ROOT_DIR" status --porcelain --untracked-files=normal -- "$scope" 2>/dev/null)" ]]; then
        printf 'true'
    else
        printf 'false'
    fi
}

# Git-aware fingerprint for the source scope. It changes when:
# - committed content in the scope changes;
# - tracked files are modified/staged/deleted;
# - untracked, non-ignored files appear or change.
# This gives us a compact identity for the local backend/frontend source.
source_fingerprint() {
    local scope="$1"
    local base_tree

    base_tree="$(git -C "$ROOT_DIR" rev-parse "HEAD:${scope}" 2>/dev/null || printf 'missing')"

    {
        printf 'scope=%s\n' "$scope"
        printf 'base-tree=%s\n' "$base_tree"

        git -C "$ROOT_DIR" diff --binary HEAD -- "$scope"

        git -C "$ROOT_DIR" ls-files --others --exclude-standard -- "$scope" \
            | LC_ALL=C sort \
            | while IFS= read -r file; do
                [[ -n "$file" ]] || continue
                printf 'untracked=%s\n' "$file"
                git -C "$ROOT_DIR" hash-object -- "$file"
            done
    } | git -C "$ROOT_DIR" hash-object --stdin
}

image_label() {
    local image="$1"
    local label="$2"

    docker --context "$DOCKER_CONTEXT" image inspect "$image" \
        --format "{{index .Config.Labels \"${label}\"}}" 2>/dev/null || true
}

image_id() {
    local image="$1"
    docker --context "$DOCKER_CONTEXT" image inspect "$image" \
        --format '{{.Id}}' 2>/dev/null || true
}

container_image_id() {
    local container="$1"
    docker --context "$DOCKER_CONTEXT" inspect "$container" \
        --format '{{.Image}}' 2>/dev/null || true
}

container_config_image() {
    local container="$1"
    docker --context "$DOCKER_CONTEXT" inspect "$container" \
        --format '{{.Config.Image}}' 2>/dev/null || true
}

build_backend() {
    local sha dirty source_hash

    sha="$(git_short_sha)"
    dirty="$(git_scope_dirty backend)"
    source_hash="$(source_fingerprint backend)"

    printf 'Build:      backend\n'
    printf 'Image:      %s\n' "$BACKEND_IMAGE"
    printf 'Source:     %s  SHA=%s DIRTY=%s\n' "$source_hash" "$sha" "$dirty"

    run_logged build-backend \
        docker --context "$DOCKER_CONTEXT" build \
            --pull=false \
            --label "dev.mvforge.git-sha=${sha}" \
            --label "dev.mvforge.dirty=${dirty}" \
            --label "dev.mvforge.source-hash=${source_hash}" \
            --label "dev.mvforge.kind=backend" \
            -t "$BACKEND_IMAGE" \
            "$ROOT_DIR/backend"

    local built_hash
    built_hash="$(image_label "$BACKEND_IMAGE" dev.mvforge.source-hash)"
    [[ "$built_hash" == "$source_hash" ]] \
        || fail "Backend image source fingerprint does not match the local source after build."
}

build_frontend() {
    local sha dirty source_hash

    sha="$(git_short_sha)"
    dirty="$(git_scope_dirty frontend)"
    source_hash="$(source_fingerprint frontend)"

    printf 'Build:      frontend\n'
    printf 'Image:      %s\n' "$WEB_IMAGE"
    printf 'Source:     %s  SHA=%s DIRTY=%s\n' "$source_hash" "$sha" "$dirty"

    run_logged build-frontend \
        docker --context "$DOCKER_CONTEXT" build \
            --pull=false \
            --target production \
            --label "dev.mvforge.git-sha=${sha}" \
            --label "dev.mvforge.dirty=${dirty}" \
            --label "dev.mvforge.source-hash=${source_hash}" \
            --label "dev.mvforge.kind=frontend" \
            -t "$WEB_IMAGE" \
            "$ROOT_DIR/frontend"

    local built_hash
    built_hash="$(image_label "$WEB_IMAGE" dev.mvforge.source-hash)"
    [[ "$built_hash" == "$source_hash" ]] \
        || fail "Frontend image source fingerprint does not match the local source after build."
}

deploy_backend() {
    printf 'Deploy:     backend\n'
    run_logged deploy-backend \
        remote_stack_command \
        "MEDIAFORGE_VERSION=$(quote_remote "$DEV_VERSION") docker compose --env-file .env up -d --no-build --no-deps --pull never --force-recreate mediaforge-backend"
}

deploy_frontend() {
    printf 'Deploy:     frontend\n'
    run_logged deploy-frontend \
        remote_stack_command \
        "MEDIAFORGE_VERSION=$(quote_remote "$DEV_VERSION") docker compose --env-file .env up -d --no-build --no-deps --pull never --force-recreate mediaforge"
}

deploy_full() {
    printf 'Deploy:     backend + frontend\n'
    run_logged deploy-full \
        remote_stack_command \
        "MEDIAFORGE_VERSION=$(quote_remote "$DEV_VERSION") docker compose --env-file .env up -d --no-build --pull never --force-recreate mediaforge-backend mediaforge"
}

container_health() {
    local container="$1"
    docker --context "$DOCKER_CONTEXT" inspect \
        --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
        "$container" 2>/dev/null || true
}

container_has_healthcheck() {
    local container="$1"
    docker --context "$DOCKER_CONTEXT" inspect \
        --format '{{if .Config.Healthcheck}}yes{{else}}no{{end}}' \
        "$container" 2>/dev/null || true
}

wait_healthy() {
    local container="$1"
    local timeout_seconds="$2"
    local deadline state

    deadline=$(( $(date +%s) + timeout_seconds ))

    while [[ "$(date +%s)" -lt "$deadline" ]]; do
        state="$(container_health "$container")"

        case "$state" in
            healthy) return 0 ;;
            running)
                if [[ "$(container_has_healthcheck "$container")" == no ]]; then
                    return 0
                fi
                ;;
            unhealthy|exited|dead)
                printf '\nFAILED: %s container state is %s\n\n' "$container" "$state" >&2
                docker --context "$DOCKER_CONTEXT" logs --tail 60 "$container" >&2 || true
                exit 1
                ;;
        esac

        sleep 2
    done

    printf '\nFAILED: %s did not become healthy within %ss\n\n' "$container" "$timeout_seconds" >&2
    docker --context "$DOCKER_CONTEXT" logs --tail 60 "$container" >&2 || true
    exit 1
}

status_component() {
    local title="$1"
    local scope="$2"
    local image="$3"
    local container="$4"

    local local_hash image_hash local_sha image_sha dirty
    local expected_image_id running_image_id configured_image health
    local source_state runtime_state

    local_hash="$(source_fingerprint "$scope")"
    local_sha="$(git_short_sha)"
    dirty="$(git_scope_dirty "$scope")"

    image_hash="$(image_label "$image" dev.mvforge.source-hash)"
    image_sha="$(image_label "$image" dev.mvforge.git-sha)"
    expected_image_id="$(image_id "$image")"
    running_image_id="$(container_image_id "$container")"
    configured_image="$(container_config_image "$container")"
    health="$(container_health "$container")"

    if [[ -n "$image_hash" && "$local_hash" == "$image_hash" ]]; then
        source_state="MATCH"
    else
        source_state="MISMATCH"
    fi

    if [[ -n "$expected_image_id" && "$expected_image_id" == "$running_image_id" ]]; then
        runtime_state="MATCH"
    else
        runtime_state="MISMATCH"
    fi

    printf '%s\n' "$title"
    printf '  Local source:    %s\n' "$local_hash"
    printf '  Local SHA:       %s  DIRTY=%s\n' "$local_sha" "$dirty"
    printf '  Dev image:       %s\n' "$image"
    printf '  Image source:    %s\n' "${image_hash:-<missing label>}"
    printf '  Image SHA:       %s\n' "${image_sha:-<missing label>}"
    printf '  Container image: %s\n' "${configured_image:-<missing>}"
    printf '  Health:          %s\n' "${health:-<missing>}"
    printf '  Source:          %s\n' "$source_state"
    printf '  Runtime image:   %s\n' "$runtime_state"

    if [[ "$source_state" == MATCH && "$runtime_state" == MATCH ]]; then
        printf '  Result:          CURRENT\n'
    else
        printf '  Result:          STALE\n'
    fi
}

show_status() {
    printf 'MVForge NAS dev status\n\n'
    status_component Backend backend "$BACKEND_IMAGE" mediaforge-backend
    printf '\n'
    status_component Frontend frontend "$WEB_IMAGE" mediaforge
}

release_version_from_nas() {
    remote_stack_command \
        "awk -F= '\$1 == \"MEDIAFORGE_VERSION\" { print substr(\$0, index(\$0, \"=\") + 1); exit }' .env" \
        2>/dev/null
}

restore_release() {
    local release_version
    release_version="$(release_version_from_nas)"

    [[ -n "$release_version" ]] \
        || fail "Could not read MEDIAFORGE_VERSION from $NAS_STACK_DIR/.env"

    printf 'Restore:    %s\n' "$release_version"

    run_logged restore-release \
        remote_stack_command \
        "unset MEDIAFORGE_VERSION; docker compose --env-file .env up -d --no-build --pull missing --force-recreate mediaforge-backend mediaforge"

    wait_healthy mediaforge-backend 180
    wait_healthy mediaforge 120

    local end_time duration
    end_time="$(date +%s)"
    duration=$(( end_time - START_TIME ))

    printf '\nMVForge NAS dev deployment summary\n\n'
    printf 'Mode:       restore\n'
    printf 'Version:    %s\n' "$release_version"
    printf 'Context:    %s\n' "$DOCKER_CONTEXT"
    printf 'Health:     PASS\n'
    printf 'Duration:   %ss\n' "$duration"
    printf '\nRELEASE RESTORED\n'
}

MODE="${1:---auto}"

case "$MODE" in
    --auto|--backend|--frontend|--full|--status|--restore) ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
esac

preflight

if [[ "$MODE" == --status ]]; then
    show_status
    exit 0
fi

if [[ "$MODE" == --restore ]]; then
    restore_release
    exit 0
fi

verify_repository "$MODE"
SCOPE="$(resolve_scope "$MODE")"

case "$SCOPE" in
    backend)
        build_backend
        deploy_backend
        wait_healthy mediaforge-backend 180
        ;;
    frontend)
        build_frontend
        deploy_frontend
        wait_healthy mediaforge 120
        ;;
    full)
        build_backend
        build_frontend
        deploy_full
        wait_healthy mediaforge-backend 180
        wait_healthy mediaforge 120
        ;;
esac

END_TIME="$(date +%s)"
DURATION=$(( END_TIME - START_TIME ))

printf '\nMVForge NAS dev deployment summary\n\n'
printf 'Mode:       %s -> %s\n' "${MODE#--}" "$SCOPE"
printf 'Version:    %s\n' "$DEV_VERSION"
printf 'Context:    %s\n' "$DOCKER_CONTEXT"

case "$SCOPE" in
    backend)
        printf 'Image:      %s\n' "$BACKEND_IMAGE"
        printf 'Source:     %s\n' "$(source_fingerprint backend)"
        ;;
    frontend)
        printf 'Image:      %s\n' "$WEB_IMAGE"
        printf 'Source:     %s\n' "$(source_fingerprint frontend)"
        ;;
    full)
        printf 'Images:     %s\n' "$BACKEND_IMAGE"
        printf '            %s\n' "$WEB_IMAGE"
        printf 'Backend:    %s\n' "$(source_fingerprint backend)"
        printf 'Frontend:   %s\n' "$(source_fingerprint frontend)"
        ;;
esac

printf 'Validation: PASS\n'
printf 'Health:     PASS\n'
printf 'Duration:   %ss\n' "$DURATION"
printf '\nDEV DEPLOYMENT READY\n'
