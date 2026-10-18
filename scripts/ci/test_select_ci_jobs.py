#!/usr/bin/env python3
"""Regression matrix for conservative CI path routing."""

import unittest
from types import SimpleNamespace
from unittest.mock import patch

from select_ci_jobs import BOOLEAN_OUTPUTS, changed_paths, classify_paths, selection_for_event


def enabled(result: dict[str, str], key: str) -> bool:
    return result[key] == "true"


class ClassifyPathsTests(unittest.TestCase):
    def test_docs_only_skips_heavy_jobs(self) -> None:
        result = classify_paths(["docs/guides/queue.md", "README.md"])
        self.assertTrue(all(not enabled(result, key) for key in BOOLEAN_OUTPUTS))

    def test_backend_runs_tests_container_and_smoke(self) -> None:
        result = classify_paths(["backend/internal/handlers/queue.go"])
        self.assertTrue(enabled(result, "backend"))
        self.assertTrue(enabled(result, "ci_backend_image"))
        self.assertTrue(enabled(result, "containers"))
        self.assertTrue(enabled(result, "smoke"))
        self.assertFalse(enabled(result, "frontend"))
        self.assertFalse(enabled(result, "compliance"))

    def test_frontend_runs_frontend_and_web_image_only(self) -> None:
        result = classify_paths(["frontend/src/App.tsx"])
        self.assertTrue(enabled(result, "frontend"))
        self.assertTrue(enabled(result, "web_image"))
        self.assertTrue(enabled(result, "containers"))
        self.assertFalse(enabled(result, "backend"))
        self.assertFalse(enabled(result, "ci_backend_image"))
        self.assertFalse(enabled(result, "smoke"))

    def test_e2e_harness_runs_smoke_without_backend_suite(self) -> None:
        result = classify_paths(["scripts/e2e/pipeline_smoke.py"])
        self.assertTrue(enabled(result, "smoke"))
        self.assertFalse(enabled(result, "backend"))
        self.assertFalse(enabled(result, "frontend"))

    def test_backend_docker_context_runs_backend_and_smoke(self) -> None:
        result = classify_paths(["backend/Dockerfile", "backend/.dockerignore"])
        self.assertTrue(enabled(result, "backend"))
        self.assertTrue(enabled(result, "ci_backend_image"))
        self.assertTrue(enabled(result, "smoke"))

    def test_pr_backend_image_build_is_reused_by_pipeline_smoke(self) -> None:
        with patch("select_ci_jobs.changed_paths", return_value=(
            ["backend/internal/handlers/queue.go"], "changed-paths",
        )):
            result = selection_for_event("pull_request", {}, ".")
        self.assertTrue(enabled(result, "smoke"))
        self.assertFalse(enabled(result, "ci_backend_image"))

    def test_full_run_keeps_backend_image_build_in_ci(self) -> None:
        with patch("select_ci_jobs.changed_paths", return_value=(
            [".github/workflows/ci.yml"], "changed-paths",
        )):
            result = selection_for_event("pull_request", {}, ".")
        self.assertTrue(enabled(result, "full"))
        self.assertTrue(enabled(result, "ci_backend_image"))

    def test_frontend_docker_context_runs_web_image(self) -> None:
        result = classify_paths(["frontend/Dockerfile", "frontend/.dockerignore"])
        self.assertTrue(enabled(result, "web_image"))
        self.assertTrue(enabled(result, "containers"))
        self.assertFalse(enabled(result, "smoke"))

    def test_workflows_shared_config_and_legal_files_run_every_check(self) -> None:
        examples = (
            ".github/workflows/ci.yml",
            "scripts/ci/runner-disk-capacity.sh",
            "deploy/nas/compose.yml",
            "THIRD_PARTY_NOTICES.md",
            "docs/legal/third-party.md",
            "backend/go.mod",
            "frontend/package-lock.json",
            "unknown-root-config.yml",
        )
        for path in examples:
            with self.subTest(path=path):
                result = classify_paths([path])
                self.assertTrue(all(enabled(result, key) for key in BOOLEAN_OUTPUTS))

    def test_removal_is_classified_by_path_even_without_worktree_file(self) -> None:
        result = classify_paths(["backend/internal/removed.go"])
        self.assertTrue(enabled(result, "backend"))
        self.assertTrue(enabled(result, "smoke"))

    def test_empty_and_unknown_event_diffs_fall_back_to_full(self) -> None:
        self.assertTrue(all(enabled(classify_paths([]), key) for key in BOOLEAN_OUTPUTS))
        result = selection_for_event("merge_group", {}, ".")
        self.assertTrue(all(enabled(result, key) for key in BOOLEAN_OUTPUTS))

    def test_pull_request_uses_three_dot_merge_base_and_keeps_deleted_paths(self) -> None:
        base = "a" * 40
        head = "b" * 40
        with patch("select_ci_jobs.subprocess.run", return_value=SimpleNamespace(
            returncode=0, stdout=b"backend/removed.go\x00", stderr=b"",
        )) as run:
            paths, reason = changed_paths(
                "pull_request", {"PR_BASE_SHA": base, "PR_HEAD_SHA": head}, ".",
            )
        self.assertEqual(paths, ["backend/removed.go"])
        self.assertEqual(reason, "changed-paths")
        self.assertEqual(
            run.call_args.args[0],
            ["git", "diff", "--name-only", "--no-renames", "-z", f"{base}...{head}", "--"],
        )

    def test_push_uses_before_after_commit_range(self) -> None:
        before = "a" * 40
        after = "c" * 40
        with patch("select_ci_jobs.subprocess.run", return_value=SimpleNamespace(
            returncode=0, stdout=b"frontend/src/App.tsx\x00", stderr=b"",
        )) as run:
            paths, _ = changed_paths(
                "push", {"PUSH_BEFORE_SHA": before, "PUSH_AFTER_SHA": after}, ".",
            )
        self.assertEqual(paths, ["frontend/src/App.tsx"])
        self.assertEqual(
            run.call_args.args[0], ["git", "diff", "--name-only", "--no-renames", "-z", before, after, "--"],
        )

    def test_selective_push_reuses_pipeline_smoke_backend_image(self) -> None:
        with patch("select_ci_jobs.changed_paths", return_value=(
            ["backend/internal/handlers/queue.go"], "changed-paths",
        )):
            result = selection_for_event("push", {}, ".")
        self.assertTrue(enabled(result, "backend"))
        self.assertTrue(enabled(result, "smoke"))
        self.assertFalse(enabled(result, "ci_backend_image"))

    def test_manual_dispatch_runs_full_validation(self) -> None:
        result = selection_for_event("workflow_dispatch", {}, ".")
        self.assertTrue(all(enabled(result, key) for key in BOOLEAN_OUTPUTS))


if __name__ == "__main__":
    unittest.main()
