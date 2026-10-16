import json
import io
import os
import subprocess
import tempfile
import textwrap
import unittest
from contextlib import redirect_stdout
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
        self.remote = self.parent / "fixture remote.git"
        subprocess.run(["git", "init", "--bare", str(self.remote)], check=True,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.git("remote", "add", "origin", str(self.remote))
        self.current, self.primary, self.common = dupla.repo_context(self.repo)
        self.issues = [
            {"number": 41, "title": "Fixture issue one", "body": "first", "state": "OPEN", "labels": []},
            {"number": 42, "title": "Fixture issue two", "body": "second", "state": "OPEN", "labels": []},
        ]
        self.fake_bin = self.parent / "fake bin"
        self.fake_bin.mkdir()
        self.fake_gh_state = self.parent / "fake gh state.json"
        self.fake_gh_state.write_text(json.dumps({"prs": [], "actions": []}), encoding="utf-8")
        fake_gh = self.fake_bin / "gh"
        fake_gh.write_text(textwrap.dedent("""\
            #!/usr/bin/env python3
            import json, os, pathlib, subprocess, sys
            path = pathlib.Path(os.environ['DUPLA_FAKE_GH_STATE'])
            state = json.loads(path.read_text())
            args = sys.argv[1:]
            def option(flag): return args[args.index(flag) + 1]
            def save(): path.write_text(json.dumps(state))
            if args[:2] == ['pr', 'list']:
                branch = option('--head')
                print(json.dumps([pr for pr in state['prs'] if pr['headRefName'] == branch]))
            elif args[:2] == ['pr', 'create']:
                branch = option('--head')
                body = pathlib.Path(option('--body-file')).read_text()
                number = 700 + len(state['prs'])
                head = subprocess.run(['git', 'rev-parse', 'HEAD'], check=True, text=True, capture_output=True).stdout.strip()
                pr = {'number': number, 'url': 'https://github.com/fixture/mediaforge/pull/{}'.format(number),
                      'title': option('--title'), 'body': body, 'state': 'OPEN', 'isDraft': '--draft' in args,
                      'baseRefName': option('--base'), 'headRefName': branch, 'headRefOid': head, 'statusCheckRollup': []}
                state['prs'].append(pr)
                state['actions'].append({'action': 'create', 'draft': '--draft' in args, 'number': number})
                save()
                print(pr['url'])
            elif args[:2] == ['pr', 'edit']:
                number = int(args[2])
                pr = next(pr for pr in state['prs'] if pr['number'] == number)
                pr['title'] = option('--title')
                pr['body'] = pathlib.Path(option('--body-file')).read_text()
                state['actions'].append({'action': 'edit', 'number': number})
                save()
                print(pr['url'])
            elif args[:2] == ['pr', 'view']:
                number = int(args[2])
                print(json.dumps(next(pr for pr in state['prs'] if pr['number'] == number)))
            else:
                print('unexpected gh command', file=sys.stderr)
                sys.exit(2)
            """), encoding="utf-8")
        fake_gh.chmod(0o755)

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

    def fake_github(self):
        return mock.patch.dict("os.environ", {
            "PATH": str(self.fake_bin) + os.pathsep + os.environ.get("PATH", ""),
            "DUPLA_FAKE_GH_STATE": str(self.fake_gh_state),
        })

    def commit_issue_change(self, state, issue_id, text="fixture implementation", verify="FAIL: fixture verifier failure"):
        worker = state["workers"][str(issue_id)]
        worktree = Path(worker["worktree"])
        filename = "issue-{}-implementation.txt".format(issue_id)
        (worktree / filename).write_text(text + "\n", encoding="utf-8")
        dupla.git(worktree, "add", filename)
        dupla.git(worktree, "commit", "-m", "fix: implement issue #{}".format(issue_id))
        summary = {
            "status": "FAIL" if not dupla.verify_passed(verify) else "PASS",
            "issue": issue_id, "branch": worker["branch"], "worktree": str(worktree),
            "summary": "Implemented fixture behavior for issue #{}".format(issue_id),
            "commits": [dupla.git(worktree, "rev-parse", "HEAD").stdout.strip()],
            "files": [filename], "tests": [{"command": "go test ./fixture", "result": "PASS"}],
            "verify": verify, "pending": ["Inspect failed verifier fixture"] if not dupla.verify_passed(verify) else [],
        }
        worker.update({"status": "complete", "exit_code": 0, "summary": summary})
        dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
        return state, worker

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

    def test_completed_workers_resume_without_requiring_codex_binary(self):
        state = self.prepare()
        for worker in state["workers"].values():
            worker.update({"status": "complete", "exit_code": 0})
        dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
        with mock.patch("dupla.shutil.which", return_value=None) as which:
            resumed = dupla.run_workers(self.common, [41, 42], resume=True)
        self.assertEqual(resumed["status"], "complete")
        which.assert_not_called()

    def test_unpublished_completed_issue_cannot_enter_another_session(self):
        state = self.prepare()
        state["status"] = "complete"
        for worker in state["workers"].values():
            worker["status"] = "complete"
        state["workers"]["41"]["publication"] = {"status": "failed"}
        dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
        with self.assertRaisesRegex(dupla.DuplaError, "unpublished session"):
            dupla.check_duplicate_sessions(self.common, [41, 43])

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
        fake_bin.mkdir(exist_ok=True)
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
    'worktree': str(worktree), 'summary': 'Fixture implementation', 'commits': [], 'files': [], 'tests': [],
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

    def test_publishes_failed_validation_as_draft_and_reuses_pr_on_retry(self):
        state = self.prepare()
        state, worker = self.commit_issue_change(state, 41)
        with self.fake_github():
            first = dupla.publish_issue(self.common, [41, 42], 41)
            self.assertEqual(first["status"], "published")
            self.assertTrue(first["is_draft"])
            self.assertFalse(first["validation_passed"])
            state = dupla.read_json(dupla.session_path(self.common, [41, 42]))
            state["workers"]["41"]["summary"]["summary"] = "Updated implementation summary"
            dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
            second = dupla.publish_issue(self.common, [41, 42], 41)
        self.assertEqual(second["number"], first["number"])
        remote_ref = subprocess.run(["git", "--git-dir", str(self.remote), "show-ref", "--verify",
                                     "refs/heads/" + worker["branch"]], text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(remote_ref.returncode, 0)
        gh_state = json.loads(self.fake_gh_state.read_text(encoding="utf-8"))
        self.assertEqual(len(gh_state["prs"]), 1)
        self.assertEqual([item["action"] for item in gh_state["actions"]], ["create", "edit"])
        pr = gh_state["prs"][0]
        self.assertTrue(pr["isDraft"])
        self.assertIn("Refs #41", pr["body"])
        self.assertIn("Updated implementation summary", pr["body"])
        self.assertIn("go test ./fixture", pr["body"])
        self.assertIn("FAIL: fixture verifier failure", pr["body"])
        self.assertIn("does not mark the issue complete", pr["body"])

    def test_secret_detection_blocks_push_and_pr_without_echoing_secret(self):
        state = self.prepare()
        token = "github_pat_abcdefghijklmnopqrstuvwxyz0123456789"
        state, worker = self.commit_issue_change(state, 41, "fixture token " + token)
        with self.fake_github():
            publication = dupla.publish_issue(self.common, [41, 42], 41)
        self.assertEqual(publication["status"], "failed")
        self.assertIn("Potential secret detected", publication["error"])
        saved = json.dumps(dupla.read_json(dupla.session_path(self.common, [41, 42])))
        self.assertNotIn(token, saved + self.fake_gh_state.read_text(encoding="utf-8"))
        self.assertEqual(json.loads(self.fake_gh_state.read_text(encoding="utf-8"))["actions"], [])
        remote_ref = subprocess.run(["git", "--git-dir", str(self.remote), "show-ref", "--verify",
                                     "refs/heads/" + worker["branch"]], text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertNotEqual(remote_ref.returncode, 0)

    def test_secret_detection_also_blocks_secrets_removed_by_the_patch(self):
        token = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
        (self.repo / "tracked.txt").write_text("credential = {}\n".format(token), encoding="utf-8")
        self.git("add", "tracked.txt")
        self.git("commit", "-m", "fixture secret baseline")
        state = self.prepare()
        worker = state["workers"]["41"]
        worktree = Path(worker["worktree"])
        (worktree / "tracked.txt").write_text("credential removed\n", encoding="utf-8")
        dupla.git(worktree, "add", "tracked.txt")
        dupla.git(worktree, "commit", "-m", "fix: remove credential for issue #41")
        worker.update({"status": "complete", "exit_code": 0, "summary": {
            "status": "PASS", "issue": 41, "branch": worker["branch"], "worktree": str(worktree),
            "summary": "Remove fixture credential", "commits": [], "files": ["tracked.txt"],
            "tests": ["fixture assertion: PASS"], "verify": "PASS — ALL CHECKS PASSED", "pending": [],
        }})
        dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
        with self.fake_github():
            publication = dupla.publish_issue(self.common, [41, 42], 41)
        self.assertEqual(publication["status"], "failed")
        self.assertIn("Potential secret detected", publication["error"])
        self.assertNotIn(token, json.dumps(dupla.read_json(dupla.session_path(self.common, [41, 42]))))
        self.assertEqual(json.loads(self.fake_gh_state.read_text(encoding="utf-8"))["actions"], [])

    def test_resume_retries_failed_push_and_creates_one_draft_pr(self):
        state = self.prepare()
        self.commit_issue_change(state, 41, verify="PASS — ALL CHECKS PASSED")
        missing_remote = self.parent / "remote unavailable.git"
        self.git("remote", "set-url", "origin", str(missing_remote))
        with self.fake_github():
            failed = dupla.publish_issue(self.common, [41, 42], 41)
            self.assertEqual(failed["status"], "failed")
            self.assertIn("push", failed["error"])
            self.git("remote", "set-url", "origin", str(self.remote))
            args = type("Args", (), {"issues": [41, 42]})()
            output = io.StringIO()
            with mock.patch("dupla.repo_context", return_value=(self.repo, self.repo, self.common)), \
                    mock.patch("dupla.run_workers", return_value=state) as run_workers, redirect_stdout(output):
                result = dupla.cmd_run(args, resume=True)
        self.assertTrue(run_workers.call_args.kwargs["resume"])
        self.assertEqual(result, 1)  # issue #42 has no implementation summary in this fixture
        report = json.loads(output.getvalue())
        self.assertIn("https://github.com/fixture/mediaforge/pull/700", report["pull_requests"].values())
        self.assertEqual(report["publication_status"], "pending")
        gh_state = json.loads(self.fake_gh_state.read_text(encoding="utf-8"))
        self.assertEqual(len(gh_state["prs"]), 1)
        self.assertEqual([item["action"] for item in gh_state["actions"]], ["create"])

    def test_refuses_empty_pr_even_with_a_commit(self):
        state = self.prepare()
        worker = state["workers"]["41"]
        worktree = Path(worker["worktree"])
        dupla.git(worktree, "commit", "--allow-empty", "-m", "chore: issue #41 no file changes")
        worker.update({"status": "complete", "summary": {
            "status": "PASS", "issue": 41, "branch": worker["branch"], "worktree": str(worktree),
            "summary": "No-op fixture", "commits": [], "files": [], "tests": [],
            "verify": "PASS — ALL CHECKS PASSED", "pending": [],
        }})
        dupla.write_json(dupla.session_path(self.common, [41, 42]), state)
        with self.fake_github():
            publication = dupla.publish_issue(self.common, [41, 42], 41)
        self.assertEqual(publication["status"], "failed")
        self.assertIn("no changed files", publication["error"])
        self.assertEqual(json.loads(self.fake_gh_state.read_text(encoding="utf-8"))["actions"], [])


if __name__ == "__main__":
    unittest.main()
