from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pipeline_smoke


class PipelineSmokeIsolationTests(unittest.TestCase):
    def test_each_run_gets_an_independent_temporary_root(self) -> None:
        with tempfile.TemporaryDirectory() as temp_base:
            first = pipeline_smoke.create_run_root(temp_base, "issue-283-attempt-1")
            second = pipeline_smoke.create_run_root(temp_base, "issue-283-attempt-2")
            self.assertNotEqual(first, second)
            self.assertEqual(first.parent, Path(temp_base).resolve())
            self.assertEqual(second.parent, Path(temp_base).resolve())
            self.assertTrue(first.name.startswith(pipeline_smoke.RUN_PREFIX))

    def test_run_root_refuses_repository_and_nas_paths(self) -> None:
        for path in (pipeline_smoke.REPO_ROOT, Path("/volume1"), Path("/volume2")):
            with self.subTest(path=path), self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.validate_temp_base(path)

    def test_runtime_container_has_one_isolated_mount_and_no_network(self) -> None:
        with tempfile.TemporaryDirectory() as temp_base:
            root = pipeline_smoke.create_run_root(temp_base, "isolation-test")
            sandbox = root / "sandbox"
            sandbox.mkdir()
            args = pipeline_smoke.runtime_container_args("smoke-test", "image:test", sandbox)
            self.assertEqual(args.count("--mount"), 1)
            self.assertIn("--network", args)
            self.assertEqual(args[args.index("--network") + 1], "none")
            self.assertIn("--read-only", args)
            self.assertEqual(args[args.index("--user") + 1], pipeline_smoke.host_user())
            mount = args[args.index("--mount") + 1]
            self.assertEqual(mount, f"type=bind,src={sandbox.resolve()},dst=/media")
            self.assertNotIn("--volume", args)
            env = [args[index + 1] for index, item in enumerate(args[:-1]) if item == "--env"]
            for key in ("MVFORGE_DB_PATH", "MVFORGE_OCR_LIFECYCLE_ROOT", "MVFORGE_OCR_PERSISTENT_ROOT", "XDG_CACHE_HOME", "TMPDIR"):
                self.assertTrue(any(value.startswith(key + "=/media/") for value in env), key)

    def test_pipeline_automation_allows_worker_and_keeps_followup_steps_controlled(self) -> None:
        settings = pipeline_smoke.PIPELINE_AUTOMATION_SETTINGS
        self.assertEqual(settings["reviewMode"], "automatic")
        self.assertTrue(settings["autoExecutionEnabled"])
        self.assertFalse(settings["autoAnalysisEnabled"])
        self.assertFalse(settings["autoValidationEnabled"])
        self.assertFalse(settings["autoPublisherEnabled"])
        self.assertFalse(settings["publisherOverwriteEnabled"])
        self.assertFalse(settings["publishedJobReconciliationEnabled"])

    def test_runtime_and_workspace_policy_are_portable_for_synthetic_storage(self) -> None:
        runtime = pipeline_smoke.SYNTHETIC_RUNTIME_POLICY
        self.assertEqual(runtime["mode"], "manual")
        self.assertEqual(runtime["preferredProfile"], "desktop_safe")
        limits = runtime["overrides"]["desktop_safe"]
        self.assertEqual(limits["minFreeWorkGb"], 0)
        self.assertEqual(limits["minFreeLibraryGb"], 0)
        self.assertTrue(limits["allowDirectMode"])
        workspace = pipeline_smoke.SYNTHETIC_WORKSPACE_SETTINGS
        self.assertEqual(workspace["preferredMode"], "copy_to_work_disk")
        self.assertEqual(workspace["fallbackMode"], "direct")
        self.assertTrue(workspace["allowDirectMode"])

    def test_plan_summary_preserves_blocking_state_and_decision_evidence(self) -> None:
        summary = pipeline_smoke.extract_plan_summary([{
            "id": 7,
            "status": "waiting",
            "waitingState": "WAITING_REVIEW",
            "approvalMode": "conditional",
            "approvalStatus": "pending",
            "decisionReasons": ["Waiting for review"],
            "warnings": ["synthetic warning"],
        }])
        self.assertEqual(summary["plans"][0]["waitingState"], "WAITING_REVIEW")
        self.assertEqual(summary["plans"][0]["decisionReasons"], ["Waiting for review"])

    def test_inspect_rejects_unexpected_network_or_host_mount(self) -> None:
        valid = {
            "Config": {"User": pipeline_smoke.host_user()},
            "HostConfig": {
                "NetworkMode": "none",
                "ReadonlyRootfs": True,
                "Binds": None,
                "CapDrop": ["ALL"],
                "SecurityOpt": ["no-new-privileges:true"],
                "Devices": None,
                "DeviceRequests": None,
                "PortBindings": None,
            },
            "Mounts": [],
        }
        with tempfile.TemporaryDirectory() as temp_base:
            root = pipeline_smoke.create_run_root(temp_base, "inspect-test")
            sandbox = root / "sandbox"
            sandbox.mkdir()
            valid["Mounts"] = [{
                "Type": "bind", "Destination": "/media", "Source": str(sandbox), "RW": True,
            }]
            pipeline_smoke.validate_container_inspect(valid, sandbox)
            invalid = {**valid, "HostConfig": {**valid["HostConfig"], "NetworkMode": "bridge"}}
            with self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.validate_container_inspect(invalid, sandbox)
            invalid = {**valid, "Mounts": valid["Mounts"] + [{"Type": "bind", "Destination": "/mnt/nas", "Source": "/mnt/nas", "RW": True}]}
            with self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.validate_container_inspect(invalid, sandbox)

    def test_evidence_writer_caps_each_file(self) -> None:
        with tempfile.TemporaryDirectory() as temp_base:
            evidence = pipeline_smoke.Evidence(Path(temp_base))
            evidence.write("bounded.log", b"x" * 200, max_file_bytes=64)
            self.assertLessEqual((Path(temp_base) / "bounded.log").stat().st_size, 64)

    def test_media_paths_must_stay_below_container_sandbox(self) -> None:
        self.assertEqual(pipeline_smoke.container_media_path("/media/staging/result.mkv", "result"), "/media/staging/result.mkv")
        for path in ("/volume1/RAW/movie.mkv", "/media/../volume1/file.mkv"):
            with self.subTest(path=path), self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.container_media_path(path, "result")


if __name__ == "__main__":
    unittest.main()
