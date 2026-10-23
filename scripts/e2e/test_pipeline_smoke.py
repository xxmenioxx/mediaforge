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
            self.assertEqual(pipeline_smoke.SUPPORTED_SCENARIOS, frozenset({"happy-path", "track-actions"}))

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

    def test_issue_286_track_actions_scenario_freezes_the_requested_subset(self) -> None:
        scenario = pipeline_smoke.scenario_for("track-actions")
        self.assertEqual(scenario.audio_stream_indexes, (1, 2))
        self.assertEqual(scenario.output_audio_stream_indexes, (1,))
        self.assertEqual(scenario.subtitle_action, "extract")
        self.assertEqual(scenario.subtitle_sidecar_formats, ("srt", "ass"))
        self.assertEqual(scenario.expected_stream_counts, {"video": 1, "audio": 2, "subtitle": 1})
        self.assertEqual(scenario.expected_output_stream_counts, {"video": 1, "audio": 1, "subtitle": 0})
        self.assertNotEqual(scenario.audio_tracks[0].language, scenario.audio_tracks[1].language)
        self.assertNotEqual(scenario.audio_tracks[0].title, scenario.audio_tracks[1].title)
        self.assertGreaterEqual(len(scenario.chapters), 2)

        job = {
            "profileSnapshot": {"preserveSubtitles": True, "preserveChapters": True},
            "profileCapturedAt": "2026-10-23T00:00:00Z",
            "trackProfileSnapshot": {"resolvedTrackPlan": {
                "audioStreams": [{"streamIndex": 1}],
                "removedAudioStreams": [{"streamIndex": 2}],
                "subtitleStreams": [{"streamIndex": 3, "action": "extract"}],
                "sidecarOutputs": [
                    {"streamIndex": 3, "format": "srt"},
                    {"streamIndex": 3, "format": "ass"},
                ],
                "chapterPolicy": "keep",
                "chaptersKept": True,
            }},
        }
        frozen = pipeline_smoke.assert_frozen_snapshots(job, scenario)
        self.assertEqual(frozen["audioStreamIndexes"], [1])
        self.assertEqual(frozen["removedAudioStreamIndexes"], [2])
        self.assertEqual(frozen["subtitleAction"], "extract")
        self.assertEqual(frozen["sidecarOutputs"], [(3, "ass"), (3, "srt")])
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_frozen_snapshots(
                {**job, "trackProfileSnapshot": {"resolvedTrackPlan": {
                    **job["trackProfileSnapshot"]["resolvedTrackPlan"],
                    "audioStreams": [{"streamIndex": 1}, {"streamIndex": 2}],
                    "removedAudioStreams": [],
                }}},
                scenario,
            )

    def test_issue_286_result_manifest_requires_valid_exact_srt_and_ass_sidecars(self) -> None:
        scenario = pipeline_smoke.scenario_for("track-actions")
        with tempfile.TemporaryDirectory() as temp_base:
            sandbox = Path(temp_base) / "sandbox"
            entries = []
            contents = {
                "srt": "1\n00:00:01,000 --> 00:00:02,500\nMVForge synthetic pipeline subtitle\n",
                "ass": (
                    "[Script Info]\nScriptType: v4.00+\n\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,MVForge synthetic pipeline subtitle\n"
                ),
            }
            for format_name in ("srt", "ass"):
                sidecar_path = sandbox / "staging" / "job-1" / f"synthetic-smoke.eng.{format_name}"
                sidecar_path.parent.mkdir(parents=True, exist_ok=True)
                sidecar_path.write_text(contents[format_name], encoding="utf-8")
                entries.append({
                    "artifactId": f"subtitle:3:converted:{format_name}",
                    "type": "subtitle_sidecar",
                    "streamIndex": 3,
                    "format": format_name,
                    "stagedPath": f"/media/staging/job-1/synthetic-smoke.eng.{format_name}",
                    "status": "ready",
                })
            job = {"trackProfileSnapshot": {"resolvedTrackPlan": {
                "sidecarOutputs": [
                    {"streamIndex": 3, "format": "srt"},
                    {"streamIndex": 3, "format": "ass"},
                ],
            }}}
            result = {"artifacts": {"sidecars": entries}}
            summary = pipeline_smoke.assert_result_artifact_manifest(result, job, scenario, sandbox)
            self.assertEqual(summary["requiredArtifactIds"], ["subtitle:3:converted:ass", "subtitle:3:converted:srt"])
            self.assertEqual({item["format"] for item in summary["sidecars"]}, {"srt", "ass"})
            self.assertEqual(pipeline_smoke.result_artifact_evidence(result, "result.json")["artifactManifest"], result["artifacts"])

            published_entries = []
            for entry in entries:
                published_path = f"/media/library/published/synthetic-smoke.eng.{entry['format']}"
                host_path = sandbox / "library" / "published" / Path(published_path).name
                host_path.parent.mkdir(parents=True, exist_ok=True)
                host_path.write_bytes((sandbox / "staging" / "job-1" / Path(entry["stagedPath"]).name).read_bytes())
                published_entries.append({**entry, "publishedPath": published_path})
            published = pipeline_smoke.assert_published_sidecars(
                {"subtitleArtifacts": published_entries}, summary, scenario, sandbox,
            )
            self.assertEqual({item["format"] for item in published}, {"srt", "ass"})

            with self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_result_artifact_manifest(
                    {"artifacts": {"sidecars": entries[:1]}}, job, scenario, sandbox,
                )
            with self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_result_artifact_manifest(
                    {"artifacts": {"sidecars": entries + [{**entries[0], "artifactId": "unexpected"}]}},
                    job,
                    scenario,
                    sandbox,
                )

    def test_issue_286_incomplete_package_block_is_bound_to_result_and_validation(self) -> None:
        blocked = pipeline_smoke.assert_publisher_blocked({"items": [{"publication": {
            "publicationId": "publication-41",
            "queueJobId": 41,
            "executionNumber": 3,
            "resultArtifactId": "job-result:41:execution:3",
            "validationRunId": "validation-41",
            "state": "BLOCKED",
            "failureSummary": "Required delivery sidecar is not ready: missing.ass",
        }}]}, 41, 3, "job-result:41:execution:3", "validation-41")
        self.assertEqual(blocked["publicationId"], "publication-41")
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_publisher_blocked({"items": [{"publication": {
                "publicationId": "publication-41",
                "queueJobId": 41,
                "executionNumber": 3,
                "resultArtifactId": "job-result:41:execution:3",
                "validationRunId": "stale-validation",
                "state": "BLOCKED",
                "failureSummary": "Required delivery sidecar is not ready: missing.ass",
            }}]}, 41, 3, "job-result:41:execution:3", "validation-41")

    def test_issue_286_output_probe_rejects_phantom_audio_embedded_subtitles_and_lost_chapters(self) -> None:
        scenario = pipeline_smoke.scenario_for("track-actions")
        probe = {
            "format": {"duration": "8.008"},
            "streams": [
                {"index": 0, "codec_type": "video", "codec_name": "hevc", "width": 320, "height": 180,
                 "avg_frame_rate": "24000/1001", "r_frame_rate": "24000/1001", "field_order": "progressive"},
                {"index": 1, "codec_type": "audio", "codec_name": "aac",
                 "tags": {"language": "eng", "title": "Synthetic English"}},
            ],
            "chapters": [
                {"tags": {"title": "Synthetic chapter 1"}},
                {"tags": {"title": "Synthetic chapter 2"}},
                {"tags": {"title": "Synthetic chapter 3"}},
            ],
        }
        summary = pipeline_smoke.assert_media_probe(probe, scenario, "Result artifact", output=True)
        self.assertEqual(summary["streamCounts"], {"video": 1, "audio": 1, "subtitle": 0})
        self.assertEqual(summary["chapterTitles"], [title for _, _, title in scenario.chapters])
        for streams, chapters in (
            ([*probe["streams"], {"index": 2, "codec_type": "audio", "tags": {"language": "spa", "title": "Synthetic Spanish"}}], probe["chapters"]),
            ([*probe["streams"], {"index": 2, "codec_type": "subtitle"}], probe["chapters"]),
            (probe["streams"], probe["chapters"][:2]),
        ):
            with self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_media_probe({**probe, "streams": streams, "chapters": chapters}, scenario, "bad Result", output=True)

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
                {"index": 1, "codec_type": "audio", "codec_name": "aac",
                 "tags": {"language": "eng", "title": "Synthetic English"}},
                {"index": 2, "codec_type": "audio", "codec_name": "aac",
                 "tags": {"language": "spa", "title": "Synthetic Spanish"}},
                {"index": 3, "codec_type": "subtitle", "codec_name": "subrip"},
            ],
            "chapters": [
                {"tags": {"title": "Synthetic chapter 1"}},
                {"tags": {"title": "Synthetic chapter 2"}},
                {"tags": {"title": "Synthetic chapter 3"}},
            ],
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

    def test_manual_publish_acceptance_requires_durable_request_identity(self) -> None:
        publication_id = pipeline_smoke.assert_manual_publish_accepted({
            "status": "accepted",
            "requestId": "manual-request-41",
            "publicationId": "publication-41",
        })
        self.assertEqual(publication_id, "publication-41")
        for response in (
            {"status": "published", "requestId": "manual-request-41", "publicationId": "publication-41"},
            {"status": "accepted", "requestId": "manual-request-41"},
            {"status": "accepted", "publicationId": "publication-41"},
        ):
            with self.subTest(response=response), self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_manual_publish_accepted(response)

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
