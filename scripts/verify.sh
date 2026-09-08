#!/usr/bin/env bash

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mvforge-verify.XXXXXX")"

GO_CACHE_DIR="${MVFORGE_GOCACHE:-${TMPDIR:-/tmp}/mvforge-go-build-cache}"
GO_TMP_DIR="${MVFORGE_GOTMPDIR:-${TMPDIR:-/tmp}/mvforge-go-tmp}"

mkdir -p "$GO_CACHE_DIR" "$GO_TMP_DIR"

export GOCACHE="$GO_CACHE_DIR"
export GOTMPDIR="$GO_TMP_DIR"

trap 'rm -rf "$TMP_DIR"' EXIT

TOTAL=0
PASSED=0
FAILED=0
START_TIME="$(date +%s)"

declare -a FAILED_CHECKS=()
declare -a FAILED_LOGS=()
declare -a FAILED_COMMANDS=()
declare -a CHANGED_FILES=()
declare -a FOCUSED_FRONTEND_TESTS=()

FOCUSED_BACKEND_HANDLERS=false
SKIP_CANONICAL_HANDLER_TEST=false

usage() {
    cat <<'EOF'
Usage:
  ./scripts/verify.sh [mode]

Modes:
  --auto       Detect scope and run focused checks before canonical validation
  --backend    Run backend validation + git diff check
  --frontend   Run frontend validation + git diff check
  --full       Run all repository validation checks
  --help       Show this help

Default:
  --full
EOF
}

format_command() {
    local arg
    local output=""

    for arg in "$@"; do
        printf -v arg '%q' "$arg"

        if [[ -n "$output" ]]; then
            output+=" "
        fi

        output+="$arg"
    done

    printf '%s' "$output"
}

run_check() {
    local name="$1"
    shift

    local log_file="$TMP_DIR/${name}.log"
    local start
    local end
    local duration
    local exit_code
    local command_text

    command_text="$(format_command "$@")"

    TOTAL=$((TOTAL + 1))
    start="$(date +%s)"

    if "$@" >"$log_file" 2>&1; then
        exit_code=0
    else
        exit_code=$?
    fi

    end="$(date +%s)"
    duration=$((end - start))

    if [[ "$exit_code" -eq 0 ]]; then
        PASSED=$((PASSED + 1))
        return 0
    fi

    FAILED=$((FAILED + 1))
    FAILED_CHECKS+=("$name")
    FAILED_LOGS+=("$log_file")
    FAILED_COMMANDS+=("$command_text")

    return "$exit_code"
}

append_frontend_test() {
    local candidate="$1"
    local existing

    for existing in "${FOCUSED_FRONTEND_TESTS[@]:-}"; do
        if [[ "$existing" == "$candidate" ]]; then
            return
        fi
    done

    FOCUSED_FRONTEND_TESTS+=("$candidate")
}

collect_changed_files() {
    local file

    while IFS= read -r file; do
        [[ -z "$file" ]] && continue
        CHANGED_FILES+=("$file")
    done < <(
        {
            git -C "$ROOT_DIR" diff --name-only HEAD --
            git -C "$ROOT_DIR" ls-files --others --exclude-standard
        } | sort -u
    )
}

detect_auto_scope() {
    local backend_changed=false
    local frontend_changed=false
    local global_changed=false
    local file

    if [[ "${#CHANGED_FILES[@]}" -eq 0 ]]; then
        echo "full"
        return
    fi

    for file in "${CHANGED_FILES[@]}"; do
        case "$file" in
            backend/*)
                backend_changed=true
                ;;

            frontend/*)
                frontend_changed=true
                ;;

            *)
                # Root-level files, scripts, CI, Docker, shared
                # configuration, etc. are conservatively cross-cutting.
                global_changed=true
                ;;
        esac
    done

    if [[ "$global_changed" == true ]]; then
        echo "full"
    elif [[ "$backend_changed" == true && "$frontend_changed" == true ]]; then
        echo "full"
    elif [[ "$backend_changed" == true ]]; then
        echo "backend"
    elif [[ "$frontend_changed" == true ]]; then
        echo "frontend"
    else
        echo "full"
    fi
}

detect_focused_checks() {
    local file
    local relative_test

    for file in "${CHANGED_FILES[@]}"; do
        case "$file" in

            # Any backend handler change gets the handlers package test first.
            backend/internal/handlers/*)
                FOCUSED_BACKEND_HANDLERS=true
                ;;

            # Any changed frontend test is executed directly first.
            frontend/*.test.ts|frontend/*.test.tsx)
                relative_test="${file#frontend/}"
                append_frontend_test "$relative_test"
                ;;
        esac
    done
}

run_focused_checks() {
    local test_file

    if [[ "$FOCUSED_BACKEND_HANDLERS" == true ]]; then
        if ! run_check \
            "focused-go-test-handlers" \
            bash -lc "cd '$ROOT_DIR/backend' && go test ./internal/handlers"; then
            return 1
        fi

        # The exact same package check already passed.
        # Avoid executing it again during canonical validation.
        SKIP_CANONICAL_HANDLER_TEST=true
    fi

    for test_file in "${FOCUSED_FRONTEND_TESTS[@]:-}"; do
        [[ -z "$test_file" ]] && continue

        if [[ ! -f "$ROOT_DIR/frontend/$test_file" ]]; then
            continue
        fi

        if ! run_check \
            "focused-$(basename "$test_file")" \
            bash -lc \
                "cd '$ROOT_DIR/frontend' && npx --no-install vitest run '$test_file'"; then
            return 1
        fi
    done

    return 0
}

run_backend_checks() {
    if [[ "$SKIP_CANONICAL_HANDLER_TEST" != true ]]; then
        run_check \
            "go-test-handlers" \
            bash -lc "cd '$ROOT_DIR/backend' && go test ./internal/handlers"
    fi

    run_check \
        "go-test-all" \
        bash -lc "cd '$ROOT_DIR/backend' && go test ./..."

    run_check \
        "go-vet" \
        bash -lc "cd '$ROOT_DIR/backend' && go vet ./..."

    run_check \
        "go-build" \
        bash -lc "cd '$ROOT_DIR/backend' && go build ./..."
}

run_frontend_checks() {
    run_check \
        "frontend-build" \
        bash -lc "cd '$ROOT_DIR/frontend' && npm run build"
}

run_diff_check() {
    run_check \
        "git-diff-check" \
        bash -lc "cd '$ROOT_DIR' && git diff --check"
}

print_failure_details() {
    local i

    for i in "${!FAILED_CHECKS[@]}"; do
        echo
        printf 'Failed check: %s\n' "${FAILED_CHECKS[$i]}"
        printf 'Command:      %s\n' "${FAILED_COMMANDS[$i]}"

        echo
        echo "---- failure output (last 60 lines) ----"
        tail -n 60 "${FAILED_LOGS[$i]}"
        echo "---- end failure output ----"
    done
}

print_summary() {
    local end_time
    local total_duration

    end_time="$(date +%s)"
    total_duration=$((end_time - START_TIME))

    echo "MVForge verification summary"
    echo

    if [[ "$MODE" == "--auto" ]]; then
        printf 'Mode: auto -> %s\n' "$SCOPE"
    else
        printf 'Mode: %s\n' "$SCOPE"
    fi

    echo
    echo "========================================"
    printf 'Scope:    %s\n' "$SCOPE"
    printf 'Tests:    %d/%d\n' "$PASSED" "$TOTAL"
    printf 'Duration: %ss\n' "$total_duration"

    if [[ "$FAILED" -gt 0 ]]; then
        echo
        printf 'Failed:   %s\n' "${FAILED_CHECKS[*]}"

        print_failure_details

        echo
        echo "VALIDATION FAILED"
        return 1
    fi

    echo
    echo "ALL CHECKS PASSED"
    return 0
}

MODE="${1:---full}"

case "$MODE" in
    --help|-h)
        usage
        exit 0
        ;;

    --full)
        SCOPE="full"
        ;;

    --backend)
        SCOPE="backend"
        ;;

    --frontend)
        SCOPE="frontend"
        ;;

    --auto)
        collect_changed_files
        SCOPE="$(detect_auto_scope)"
        detect_focused_checks
        ;;

    *)
        echo "Unknown mode: $MODE"
        echo
        usage
        exit 2
        ;;
esac

#
# Focused validation is used only by --auto.
#
# If a focused check fails, stop immediately. Broader validation
# would only consume additional time and context before the local
# failure is fixed.
#
if [[ "$MODE" == "--auto" ]]; then
    if [[ "$FOCUSED_BACKEND_HANDLERS" == true ||
          "${#FOCUSED_FRONTEND_TESTS[@]}" -gt 0 ]]; then

        if ! run_focused_checks; then
            print_summary
            exit 1
        fi
    fi
fi

case "$SCOPE" in
    backend)
        run_backend_checks
        run_diff_check
        ;;

    frontend)
        run_frontend_checks
        run_diff_check
        ;;

    full)
        run_backend_checks
        run_frontend_checks
        run_diff_check
        ;;
esac

if ! print_summary; then
    exit 1
fi