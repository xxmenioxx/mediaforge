from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

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

    def test_each_scenario_run_gets_an_independent_database_and_filesystem(self) -> None:
        with tempfile.TemporaryDirectory() as temp_base:
            roots = []
            for run_id in ("issue-284-happy-path", "issue-285-future-scenario"):
                root = pipeline_smoke.create_run_root(temp_base, run_id)
                sandbox = root / "sandbox"
                pipeline_smoke.prepare_sandbox(sandbox)
                roots.append(sandbox)
            first_db = roots[0] / "config" / "mvforge.db"
            second_db = roots[1] / "config" / "mvforge.db"
            self.assertNotEqual(first_db, second_db)
            self.assertNotEqual(roots[0] / "raw", roots[1] / "raw")
            self.assertEqual(pipeline_smoke.SUPPORTED_SCENARIOS, frozenset({"happy-path"}))

    def test_happy_path_generates_deterministic_multistream_mkv_inputs_in_sandbox(self) -> None:
        scenario = pipeline_smoke.scenario_for("happy-path")
        with tempfile.TemporaryDirectory() as temp_base:
            root = pipeline_smoke.create_run_root(temp_base, "issue-284-fixture")
            sandbox = root / "sandbox"
            pipeline_smoke.prepare_sandbox(sandbox)
            with patch.object(pipeline_smoke, "run_command") as run_command:
                pipeline_smoke.generate_fixture(pipeline_smoke.DEFAULT_IMAGE, sandbox, scenario)

            command = run_command.call_args.args[0]
            self.assertEqual(run_command.call_args.kwargs["timeout"], 90)
            self.assertEqual(command[command.index("--network") + 1], "none")
            self.assertIn("--read-only", command)
            self.assertEqual(command.count("--mount"), 1)
            self.assertIn(
                "testsrc2=size=320x180:rate=24000/1001:duration=8",
                command,
            )
            self.assertIn("sine=frequency=440:sample_rate=48000:duration=8", command)
            self.assertIn("sine=frequency=880:sample_rate=48000:duration=8", command)
            self.assertIn("-map", command)
            for stream in ("0:v:0", "1:a:0", "2:a:0", "3:s:0"):
                self.assertIn(stream, command)
            self.assertEqual(command[command.index("-map_chapters") + 1], "4")
            self.assertEqual(command[-1], "/media/raw/synthetic-smoke.mkv")

            subtitle = (sandbox / "fixtures" / "synthetic-smoke.srt").read_text(encoding="utf-8")
            chapters = (sandbox / "fixtures" / "synthetic-smoke.ffmeta").read_text(encoding="utf-8")
            self.assertIn("MVForge synthetic pipeline subtitle", subtitle)
            self.assertEqual(chapters.count("[CHAPTER]"), 3)
            self.assertIn("TIMEBASE=1/1000", chapters)
            self.assertEqual(chapters, chapters.replace("\r\n", "\n"))

    def test_issue_364_cadence_fixture_reuses_the_existing_isolated_image_and_harness(self) -> None:
        with tempfile.TemporaryDirectory() as temp_base:
            root = pipeline_smoke.create_run_root(temp_base, "issue-364-cadence")
            sandbox = root / "sandbox"
            pipeline_smoke.prepare_sandbox(sandbox)
            args = pipeline_smoke.parse_args(["generate-cadence", "--sandbox", str(sandbox)])
            with patch.object(pipeline_smoke, "run_command") as run_command:
                pipeline_smoke.generate_cadence_fixture_only(args)

            command = run_command.call_args.args[0]
            self.assertEqual(command[command.index("--network") + 1], "none")
            self.assertIn("--read-only", command)
            self.assertEqual(command.count("--mount"), 1)
            self.assertIn("testsrc2=size=320x180:rate=24000/1001:duration=8", command)
            self.assertEqual(command[command.index("-vf") + 1], "telecine=first_field=top")
            self.assertEqual(command[command.index("-flags") + 1], "+ilme+ildct")
            self.assertEqual(command[command.index("-top") + 1], "1")
            self.assertEqual(command[-1], "/media/raw/synthetic-smoke.mkv")

    def test_ffprobe_contract_requires_exact_streams_progressive_rate_and_chapters(self) -> None:
        scenario = pipeline_smoke.scenario_for("happy-path")
        probe = {
            "format": {"duration": "8.008"},
            "streams": [
                {"index": 0, "codec_type": "video", "codec_name": "h264", "width": 320, "height": 180,
                 "avg_frame_rate": "24000/1001", "r_frame_rate": "24000/1001", "field_order": "progressive"},
                {"index": 1, "codec_type": "audio", "codec_name": "aac"},
                {"index": 2, "codec_type": "audio", "codec_name": "aac"},
                {"index": 3, "codec_type": "subtitle", "codec_name": "subrip"},
            ],
            "chapters": [{}, {}, {}],
        }
        summary = pipeline_smoke.assert_media_probe(
            probe, scenario, "fixture", expected_video_codec="h264",
        )
        self.assertEqual(summary["streamCounts"], {"video": 1, "audio": 2, "subtitle": 1})
        self.assertEqual(summary["chapterCount"], 3)
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_media_probe({**probe, "streams": probe["streams"][:-1]}, scenario, "missing subtitle")
        interlaced = {**probe, "streams": [{**probe["streams"][0], "field_order": "tt"}, *probe["streams"][1:]]}
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_media_probe(interlaced, scenario, "interlaced")
        missing_chapter = {**probe, "chapters": probe["chapters"][:2]}
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_media_probe(missing_chapter, scenario, "missing chapter")

    def test_required_validation_findings_have_only_canonical_expected_states(self) -> None:
        findings = [
            {"key": key, "state": state}
            for key, state in pipeline_smoke.REQUIRED_FINDING_STATES.items()
        ]
        states = pipeline_smoke.assert_required_findings(findings)
        self.assertEqual(states, pipeline_smoke.REQUIRED_FINDING_STATES)
        invalid = [
            {**finding, "state": "WARNING"} if finding["key"] == "publication.readiness" else finding
            for finding in findings
        ]
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_required_findings(invalid)
        duplicate = findings + [{"key": "artifact.integrity", "state": "PASS"}]
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_required_findings(duplicate)

    def test_validation_response_and_run_compare_stable_finding_states(self) -> None:
        response = [{
            "key": "artifact.identity",
            "state": "PASS",
            "actual": {"filesystem": {"modifiedAtUnixNano": 1_792_370_267_621_097_456}},
        }]
        persisted = [{
            "key": "artifact.identity",
            "state": "PASS",
            "actual": {"filesystem": {"modifiedAtUnixNano": 1_792_370_267_621_097_500}},
        }]
        response_signature = pipeline_smoke.validation_finding_state_signature(response)
        self.assertEqual(response_signature, pipeline_smoke.validation_finding_state_signature(persisted))
        self.assertNotEqual(
            response_signature,
            pipeline_smoke.validation_finding_state_signature([{"key": "artifact.identity", "state": "FAIL"}]),
        )
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.validation_finding_state_signature(response + response)

    def test_result_validation_and_publisher_bind_to_the_same_frozen_generation(self) -> None:
        job = {"id": 41, "executionNumber": 3, "activeExecutionPlanId": 7}
        plan = {"id": 7, "jobId": 41, "version": 2}
        result = {
            "resultIdentity": {
                "artifactId": "job-result:41:execution:3",
                "generationToken": "generation-3",
                "generationVersion": 1,
                "outputRevision": "revision-3",
            },
            "executionBinding": {
                "executionNumber": 3,
                "executionPlanId": 7,
                "executionPlanVersion": 2,
                "executionPlanFingerprint": "plan-fingerprint",
                "frozenQueueFingerprint": "queue-fingerprint",
            },
        }
        validation_run = {
            "resultArtifactId": "job-result:41:execution:3",
            "resultGenerationToken": "generation-3",
            "resultGenerationVersion": 1,
            "resultOutputRevision": "revision-3",
            "executionNumber": 3,
            "executionPlanId": 7,
            "executionPlanVersion": 2,
            "executionPlanFingerprint": "plan-fingerprint",
        }
        summary = pipeline_smoke.assert_result_validation_binding(job, plan, result, validation_run)
        self.assertEqual(summary["resultIdentity"]["resultOutputRevision"], "revision-3")
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_result_validation_binding(job, {**plan, "version": 1}, result, validation_run)

        publication = {
            "publication": {
                "publicationId": "publication-41",
                "queueJobId": 41,
                "executionNumber": 3,
                "resultArtifactId": "job-result:41:execution:3",
                "validationRunId": "validation-41",
                "state": "PUBLISHED",
                "destinationPath": "/media/library/published/output.mkv",
            },
        }
        published = pipeline_smoke.assert_publisher_binding(
            {"items": [publication]}, 41, 3, "job-result:41:execution:3", "validation-41",
        )
        self.assertEqual(published["state"], "PUBLISHED")
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_publisher_binding(
                {"items": [publication]}, 41, 3, "job-result:41:execution:3", "stale-validation",
            )

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
