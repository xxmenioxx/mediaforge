import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import dupla


class DuplaWorktreeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="dupla repo with spaces ")
        self.parent = Path(self.temp.name)
        self.repo = self.parent / "mediaforge fixture"
        self.repo.mkdir()
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Dupla Test")
        self.git("config", "user.email", "dupla-test@example.invalid")
        (self.repo / "tracked.txt").write_text("base\n", encoding="utf-8")
        self.git("add", "tracked.txt")
        self.git("commit", "-m", "fixture base")
        self.current, self.primary, self.common = dupla.repo_context(self.repo)
        self.issues = [
            {"number": 41, "title": "Fixture issue one", "body": "first", "state": "OPEN", "labels": []},
            {"number": 42, "title": "Fixture issue two", "body": "second", "state": "OPEN", "labels": []},
        ]

    def tearDown(self):
        self.temp.cleanup()

    def git(self, *args):
        result = subprocess.run(["git", "-C", str(self.repo), *args], text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode:
            self.fail("git {} failed: {}".format(" ".join(args), result.stderr))
        return result.stdout.strip()

    def prepare(self, safe=True):
        return dupla.prepare_pair(self.repo, self.common, [41, 42], self.issues,
                                  "refs/heads/main", safe)

    def test_creates_two_sibling_worktrees_from_same_main_commit(self):
        state = self.prepare()
        base = self.git("rev-parse", "main")
        self.assertEqual(state["base_sha"], base)
        self.assertEqual(set(state["workers"]), {"41", "42"})
        for worker in state["workers"].values():
            path = Path(worker["worktree"])
            self.assertEqual(path.parent.resolve(), self.repo.parent.resolve())
            self.assertEqual(dupla.git(path, "rev-parse", "HEAD").stdout.strip(), base)
            self.assertTrue(dupla.issue_in_branch(worker["branch"], int(worker["issue"]["number"])))

    def test_reuses_saved_worktrees_and_preserves_dirty_files(self):
        first = self.prepare()
        worker_path = Path(first["workers"]["41"]["worktree"])
        (worker_path / "uncommitted.txt").write_text("keep me\n", encoding="utf-8")
        loaded_state = json.loads(dupla.session_path(self.common, [41, 42]).read_text(encoding="utf-8"))
        second = dupla.prepare_pair(self.repo, self.common, [41, 42], self.issues, "refs/heads/main", True)
        self.assertEqual(second["workers"]["41"]["action"], "reuse")
        self.assertEqual((worker_path / "uncommitted.txt").read_text(encoding="utf-8"), "keep me\n")
        self.assertEqual(loaded_state["base_sha"], second["base_sha"])

    def test_adopts_only_a_matching_registered_issue_worktree(self):
        branch = dupla.branch_for(self.issues[0])
        path = self.repo.parent / (self.repo.name + "-issue-41")
        self.git("worktree", "add", "-b", branch, str(path), "refs/heads/main")
        state = self.prepare()
        self.assertEqual(state["workers"]["41"]["worktree"], str(path.resolve()))
        self.assertEqual(state["workers"]["41"]["action"], "reuse")

    def test_refuses_an_unregistered_path_without_touching_it(self):
        path = self.repo.parent / (self.repo.name + "-issue-41")
        path.mkdir()
        marker = path / "keep.txt"
        marker.write_text("unrelated\n", encoding="utf-8")
        with self.assertRaises(dupla.DuplaError):
            self.prepare()
        self.assertEqual(marker.read_text(encoding="utf-8"), "unrelated\n")
        self.assertEqual(self.git("branch", "--list", "fix/41-fixture-issue-one"), "")

    def test_parallel_gate_fails_before_creating_any_worktree(self):
        with self.assertRaises(dupla.DuplaError):
            self.prepare(safe=False)
        self.assertEqual(len(dupla.parse_worktrees(self.repo)), 1)

    def test_worktree_recovery_after_reloading_saved_state(self):
        first = self.prepare()
        path = Path(first["workers"]["42"]["worktree"])
        new_process_state = dupla.read_json(dupla.session_path(self.common, [41, 42]))
        again = dupla.prepare_pair(self.repo, self.common, [41, 42], self.issues, "refs/heads/main", True)
        self.assertEqual(again["workers"]["42"]["worktree"], str(path))
        self.assertEqual(new_process_state["workers"]["42"]["branch"], again["workers"]["42"]["branch"])

    def test_read_only_review_can_discover_existing_issue_worktrees(self):
        self.prepare()
        dupla.session_path(self.common, [41, 42]).unlink()
        report = dupla.review_pair(self.common, [41, 42])
        self.assertEqual(report["workers"]["41"]["status"], "unknown")
        self.assertEqual(report["workers"]["42"]["status"], "unknown")
        self.assertEqual(report["merge_tree"]["exit_code"], 0)

    def test_supervisor_collects_second_worker_when_first_fails(self):
        self.prepare()
        fake_bin = self.parent / "fake bin"
        fake_bin.mkdir()
        fake_codex = fake_bin / "codex"
        fake_codex.write_text("""#!/usr/bin/env python3
import json, pathlib, sys
args = sys.argv[1:]
worktree = pathlib.Path(args[args.index('--cd') + 1])
summary = pathlib.Path(args[args.index('--output-last-message') + 1])
issue = int(worktree.name.rsplit('-', 1)[1])
sys.stdin.read()
print('fixture worker {}'.format(issue))
if issue == 41:
    sys.exit(7)
summary.write_text(json.dumps({
    'status': 'PASS', 'issue': issue, 'branch': 'fix/42-fixture-issue-two',
    'worktree': str(worktree), 'commits': [], 'files': [], 'tests': [],
    'verify': 'NOT_RUN', 'pending': []
}))
""", encoding="utf-8")
        fake_codex.chmod(0o755)
        with mock.patch.dict("os.environ", {"PATH": str(fake_bin) + os.pathsep + os.environ.get("PATH", "")}):
            state = dupla.run_workers(self.common, [41, 42])
        self.assertEqual(state["workers"]["41"]["status"], "failed")
        self.assertEqual(state["workers"]["42"]["status"], "complete")
        self.assertTrue(Path(state["workers"]["41"]["log"]).is_file())
        self.assertTrue(Path(state["workers"]["42"]["summary_file"]).is_file())
        self.assertEqual(state["status"], "partial")
        resource_41 = dupla.state_root(self.common) / "resources" / "issue-41-attempt-1"
        resource_42 = dupla.state_root(self.common) / "resources" / "issue-42-attempt-1"
        self.assertTrue((resource_41 / "go-cache").is_dir())
        self.assertTrue((resource_42 / "go-cache").is_dir())
        self.assertNotEqual(resource_41, resource_42)

        preserved = Path(state["workers"]["41"]["worktree"]) / "partial-work.txt"
        preserved.write_text("retain on resume\n", encoding="utf-8")
        resumed = dupla.run_workers(self.common, [41, 42], resume=True)
        self.assertEqual(resumed["workers"]["41"]["attempt"], 2)
        self.assertEqual(resumed["workers"]["42"]["attempt"], 1)
        self.assertEqual(preserved.read_text(encoding="utf-8"), "retain on resume\n")
        self.assertNotEqual(resumed["workers"]["41"]["log"], state["workers"]["41"]["log"])


if __name__ == "__main__":
    unittest.main()
