#!/usr/bin/env python3
"""Select conservative CI checks from a GitHub event's changed paths."""

from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import PurePosixPath
from typing import Mapping, Sequence


BOOLEAN_OUTPUTS = (
    "full",
    "backend",
    "frontend",
    "containers",
    "compliance",
    "ci_backend_image",
    "web_image",
    "smoke",
)
SHA_PATTERN = re.compile(r"[0-9a-fA-F]{40,64}\Z")
ZERO_SHA_PATTERN = re.compile(r"0+\Z")
LEGAL_FILENAMES = {
    "LICENSE",
    "NOTICE",
    "COPYING",
    "COPYRIGHT",
    "LEGAL.md",
    "THIRD_PARTY_LICENSES.md",
    "THIRD_PARTY_NOTICES.md",
    "SECURITY.md",
}
DEPENDENCY_FILES = {
    "backend/go.mod",
    "backend/go.sum",
    "frontend/package.json",
    "frontend/package-lock.json",
}
DOC_ROOT_FILES = {
    "README.md",
    "CONTRIBUTING.md",
}


def all_checks(reason: str) -> dict[str, str]:
    return {**{key: "true" for key in BOOLEAN_OUTPUTS}, "reason": reason}


def no_checks(reason: str = "documentation-only") -> dict[str, str]:
    return {**{key: "false" for key in BOOLEAN_OUTPUTS}, "reason": reason}


def _is_docs_only_path(path: str) -> bool:
    if path in DOC_ROOT_FILES:
        return True
    return path.startswith("docs/") and path != "docs/AGENTS.md" and not path.startswith(
        ("docs/legal/", "docs/licenses/", "docs/compliance/"),
    )


def _requires_full_check(path: str) -> bool:
    name = PurePosixPath(path).name
    if name.upper() in LEGAL_FILENAMES:
        return True
    if path.startswith((".github/", "scripts/ci/", "scripts/verify", "deploy/")):
        return True
    if path.startswith(("backend/licenses/", "frontend/licenses/")):
        return True
    if path in DEPENDENCY_FILES:
        return True
    if path in {"AGENTS.md", "docs/AGENTS.md", "Makefile", "go.work", "go.work.sum", ".gitignore", ".gitattributes", ".gitmodules"}:
        return True
    if name in {"Dockerfile", ".dockerignore"} and "/" not in path:
        return True
    return False


def classify_paths(paths: Sequence[str]) -> dict[str, str]:
    normalized = [path.removeprefix("./") for path in paths if path]
    if not normalized:
        return all_checks("empty-or-ambiguous-diff")

    if any(_requires_full_check(path) for path in normalized):
        return all_checks("high-risk-or-shared-configuration")

    if all(_is_docs_only_path(path) for path in normalized):
        return no_checks()

    backend = any(path.startswith("backend/") for path in normalized)
    frontend = any(path.startswith("frontend/") for path in normalized)
    smoke = backend or any(path.startswith("scripts/e2e/") for path in normalized)
    recognized = all(
        _is_docs_only_path(path)
        or path.startswith(("backend/", "frontend/", "scripts/e2e/"))
        for path in normalized
    )
    if not recognized:
        return all_checks("unclassified-path")

    return {
        "full": "false",
        "backend": str(backend).lower(),
        "frontend": str(frontend).lower(),
        "containers": str(backend or frontend).lower(),
        "compliance": "false",
        "ci_backend_image": str(backend).lower(),
        "web_image": str(frontend).lower(),
        "smoke": str(smoke).lower(),
        "reason": "backend-and-frontend-paths",
    }


def _valid_sha(value: str) -> bool:
    return bool(SHA_PATTERN.fullmatch(value)) and not ZERO_SHA_PATTERN.fullmatch(value)


def changed_paths(event_name: str, env: Mapping[str, str], cwd: str) -> tuple[list[str] | None, str]:
    if event_name == "workflow_dispatch":
        return None, "manual-full-run"

    if event_name == "pull_request":
        base = env.get("PR_BASE_SHA", "")
        head = env.get("PR_HEAD_SHA", "")
        if not _valid_sha(base) or not _valid_sha(head):
            return None, "missing-pull-request-sha"
        revisions = [f"{base}...{head}"]
    elif event_name == "push":
        before = env.get("PUSH_BEFORE_SHA", "")
        after = env.get("PUSH_AFTER_SHA", "")
        if not _valid_sha(before) or not _valid_sha(after):
            return None, "missing-push-range"
        revisions = [before, after]
    else:
        return None, "unknown-event"

    result = subprocess.run(
        ["git", "diff", "--name-only", "--no-renames", "-z", *revisions, "--"],
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if result.returncode != 0:
        return None, "git-diff-failed"
    paths = [os.fsdecode(item) for item in result.stdout.split(b"\0") if item]
    if not paths:
        return None, "empty-diff"
    return paths, "changed-paths"


def selection_for_event(event_name: str, env: Mapping[str, str], cwd: str) -> dict[str, str]:
    paths, reason = changed_paths(event_name, env, cwd)
    if paths is None:
        return all_checks(reason)
    result = classify_paths(paths)
    if event_name in {"pull_request", "push"} and result["full"] == "false" and result["smoke"] == "true":
        # The Pipeline Smoke workflow builds and exercises the backend image
        # for selective backend/E2E changes, so CI only validates Compose here.
        result["ci_backend_image"] = "false"
    result["reason"] = reason + ":" + result["reason"]
    return result


def write_outputs(result: Mapping[str, str], output_path: str | None) -> None:
    lines = [f"{key}={result[key]}" for key in (*BOOLEAN_OUTPUTS, "reason")]
    if output_path:
        with open(output_path, "a", encoding="utf-8") as output:
            output.write("\n".join(lines) + "\n")
        return
    print("\n".join(lines))


def main() -> int:
    env = os.environ
    result = selection_for_event(env.get("GITHUB_EVENT_NAME", ""), env, os.getcwd())
    write_outputs(result, env.get("GITHUB_OUTPUT"))
    print(f"CI selection: {result['reason']}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
