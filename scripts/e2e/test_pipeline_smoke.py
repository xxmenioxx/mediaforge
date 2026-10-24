from __future__ import annotations

import json
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
            self.assertEqual(pipeline_smoke.SUPPORTED_SCENARIOS, frozenset({
                "happy-path", "track-actions", "language-normalization",
                "language-normalization-exclusions",
                "cadence-progressive", "cadence-ivtc", "cadence-ambiguous",
            }))

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
            self.assertNotIn("-map_metadata", command)
            self.assertEqual(command[-1], "/media/raw/synthetic-smoke.mkv")

            subtitle = (sandbox / "fixtures" / "synthetic-smoke.srt").read_text(encoding="utf-8")
            chapters = (sandbox / "fixtures" / "synthetic-smoke.ffmeta").read_text(encoding="utf-8")
            self.assertIn("MVForge synthetic pipeline subtitle", subtitle)
            self.assertEqual(chapters.count("[CHAPTER]"), 3)
            self.assertIn("TIMEBASE=1/1000", chapters)
            for _, _, title in scenario.chapters:
                self.assertIn(f"title={title}", chapters)
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
                "audioStreams": [{"streamIndex": 1, "title": scenario.audio_tracks[0].title}],
                "removedAudioStreams": [{"streamIndex": 2}],
                "subtitleStreams": [{
                    "streamIndex": 3,
                    "language": "eng",
                    "title": scenario.subtitle_track_for_stream_index(3).title,
                    "action": "extract",
                }],
                "sidecarOutputs": [
                    {"streamIndex": 3, "format": "srt", "language": "eng"},
                    {"streamIndex": 3, "format": "ass", "language": "eng"},
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

    def test_issue_330_language_scenario_uses_language_policy_and_frozen_canonical_tracks(self) -> None:
        scenario = pipeline_smoke.scenario_for("language-normalization")
        self.assertEqual(scenario.track_profile_audio_mode, "languages")
        self.assertEqual(scenario.track_profile_scope, "path")
        self.assertEqual(scenario.track_profile_audio_languages, ("ko", "kor", "ko-KR"))
        self.assertEqual(scenario.track_profile_subtitle_mode, "languages")
        self.assertEqual(scenario.track_profile_subtitle_languages, ("fr", "fra", "fre", "fr-FR"))
        self.assertEqual(scenario.expected_resolved_audio_languages, ("kor", "kor", "kor"))
        self.assertEqual(scenario.expected_resolved_subtitle_language, "fra")
        self.assertEqual(tuple(track.language for track in scenario.audio_tracks), ("ko", "kor", "ko-KR", "eng"))
        self.assertEqual(tuple(track.language for track in scenario.source_subtitle_tracks), ("fr", "fra", "fre", "fr-FR"))
        self.assertEqual(scenario.output_audio_stream_indexes, (1, 2, 3))
        self.assertEqual(scenario.resolved_subtitle_stream_indexes, (5, 6, 7, 8))
        self.assertEqual(scenario.subtitle_language, "fra")
        profile = pipeline_smoke.track_profile_for_scenario("synthetic-language-policy", scenario)
        self.assertEqual(profile["audioMode"], "languages")
        self.assertEqual(profile["scope"], "path")
        self.assertEqual(profile["audioLanguages"], ["ko", "kor", "ko-KR"])
        self.assertNotIn("keepAudioStreams", profile)
        self.assertEqual(profile["subtitleMode"], "languages")
        self.assertEqual(profile["subtitleLanguages"], ["fr", "fra", "fre", "fr-FR"])
        self.assertNotIn("keepSubtitleStreams", profile)

        with tempfile.TemporaryDirectory() as temp_base:
            root = pipeline_smoke.create_run_root(temp_base, "issue-330-fixture")
            sandbox = root / "sandbox"
            pipeline_smoke.prepare_sandbox(sandbox)
            with patch.object(pipeline_smoke, "run_command") as run_command:
                pipeline_smoke.generate_fixture(pipeline_smoke.DEFAULT_IMAGE, sandbox, scenario)
            command = run_command.call_args.args[0]
            for language in ("ko", "kor", "ko-KR", "fr", "fra", "fre", "fr-FR"):
                self.assertIn(f"language={language}", command)
            for subtitle_input in ("5:s:0", "6:s:0", "7:s:0", "8:s:0"):
                self.assertIn(subtitle_input, command)
            self.assertEqual(command[command.index("-map_chapters") + 1], "9")
            for index, track in enumerate(scenario.source_subtitle_tracks, start=1):
                subtitle_path = sandbox / "fixtures" / f"synthetic-smoke-subtitle-{index}.srt"
                self.assertIn(track.text, subtitle_path.read_text(encoding="utf-8"))
            self.assertEqual(command[-1], "/media/raw/synthetic-smoke.mkv")

        def frozen_job(for_scenario: pipeline_smoke.SyntheticScenario) -> dict[str, object]:
            selected_audio = for_scenario.output_audio_stream_indexes
            selected_subtitles = for_scenario.resolved_subtitle_stream_indexes
            return {
                "profileSnapshot": {"preserveSubtitles": True, "preserveChapters": True},
                "profileCapturedAt": "2026-10-23T00:00:00Z",
                "trackProfileSnapshot": {"resolvedTrackPlan": {
                    "audioStreams": [
                        {
                            "streamIndex": index,
                            "language": "kor",
                            "title": for_scenario.audio_tracks[index - 1].title,
                        }
                        for index in selected_audio
                    ],
                    "removedAudioStreams": [
                        {"streamIndex": index, "language": for_scenario.audio_tracks[index - 1].language}
                        for index in for_scenario.audio_stream_indexes
                        if index not in selected_audio
                    ],
                    "subtitleStreams": [
                        {
                            "streamIndex": index,
                            "language": for_scenario.expected_resolved_subtitle_languages_by_stream[index],
                            "title": for_scenario.subtitle_track_for_stream_index(index).title,
                            "action": "extract",
                        }
                        for index in selected_subtitles
                    ],
                    "sidecarOutputs": [
                        {
                            "streamIndex": index,
                            "format": format_name,
                            "language": for_scenario.expected_sidecar_languages_by_stream[index],
                        }
                        for index in selected_subtitles
                        for format_name in for_scenario.subtitle_sidecar_formats
                    ],
                    "chapterPolicy": "keep",
                    "chaptersKept": True,
                }},
            }

        job = frozen_job(scenario)
        frozen = pipeline_smoke.assert_frozen_snapshots(job, scenario)
        self.assertEqual(frozen["audioStreamIndexes"], [1, 2, 3])
        self.assertEqual(frozen["audioLanguages"], ["kor", "kor", "kor"])
        self.assertEqual(frozen["removedAudioStreamIndexes"], [4])
        self.assertEqual(frozen["subtitleStreamIndexes"], [5, 6, 7, 8])
        self.assertEqual(frozen["subtitleLanguages"], ["fra", "fra", "fra", "fra"])
        self.assertEqual(len(frozen["sidecarOutputs"]), 8)

        exclusion_scenario = pipeline_smoke.scenario_for("language-normalization-exclusions")
        exclusion_profile = pipeline_smoke.track_profile_for_scenario("synthetic-language-policy", exclusion_scenario)
        self.assertEqual(exclusion_scenario.asset_keep_audio_stream_indexes, (1, 3))
        self.assertEqual(exclusion_scenario.output_audio_stream_indexes, (1, 3))
        self.assertEqual(exclusion_profile["audioMode"], "languages")
        self.assertNotIn("keepAudioStreams", exclusion_profile)
        exclusion_frozen = pipeline_smoke.assert_frozen_snapshots(frozen_job(exclusion_scenario), exclusion_scenario)
        self.assertEqual(exclusion_frozen["audioStreamIndexes"], [1, 3])
        self.assertEqual(exclusion_frozen["removedAudioStreamIndexes"], [2, 4])

        wrong_language = {
            **job,
            "trackProfileSnapshot": {"resolvedTrackPlan": {
                **job["trackProfileSnapshot"]["resolvedTrackPlan"],
                "audioStreams": [{
                    "streamIndex": 1,
                    "language": "ko",
                    "title": scenario.audio_tracks[0].title,
                }],
            }},
        }
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_frozen_snapshots(wrong_language, scenario)

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
                    "language": "eng",
                    "default": True,
                    "forced": False,
                    "role": "unknown",
                    "stagedPath": f"/media/staging/job-1/synthetic-smoke.eng.{format_name}",
                    "status": "ready",
                })
            job = {"trackProfileSnapshot": {"resolvedTrackPlan": {
                "sidecarOutputs": [
                    {"streamIndex": 3, "format": "srt", "language": "eng"},
                    {"streamIndex": 3, "format": "ass", "language": "eng"},
                ],
            }}}
            result = {"artifacts": {"sidecars": entries}}
            summary = pipeline_smoke.assert_result_artifact_manifest(result, job, scenario, sandbox)
            self.assertEqual(summary["requiredArtifactIds"], ["subtitle:3:converted:ass", "subtitle:3:converted:srt"])
            self.assertEqual({item["format"] for item in summary["sidecars"]}, {"srt", "ass"})
            self.assertEqual(pipeline_smoke.result_artifact_evidence(result, "result.json")["artifactManifest"], result["artifacts"])

            published_entries = []
            for entry in entries:
                published_path = f"/media/library/published/synthetic-smoke.eng.default.{entry['format']}"
                host_path = sandbox / "library" / "published" / Path(published_path).name
                host_path.parent.mkdir(parents=True, exist_ok=True)
                host_path.write_bytes((sandbox / "staging" / "job-1" / Path(entry["stagedPath"]).name).read_bytes())
                published_entries.append({**entry, "publishedPath": published_path})
            published = pipeline_smoke.assert_published_sidecars(
                {"subtitleArtifacts": published_entries}, summary, scenario, sandbox,
                "/media/library/published/synthetic-smoke.mkv",
            )
            self.assertEqual({item["format"] for item in published}, {"srt", "ass"})
            self.assertTrue(all(item["apiPublishedPath"] for item in published))
            unpublished_paths = pipeline_smoke.assert_published_sidecars(
                {"subtitleArtifacts": [{**entry, "publishedPath": ""} for entry in published_entries]},
                summary,
                scenario,
                sandbox,
                "/media/library/published/synthetic-smoke.mkv",
            )
            self.assertTrue(all(item["apiPublishedPath"] == "" for item in unpublished_paths))

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
                 "tags": {"language": "eng", "title": "Original English Mono"}},
            ],
            "chapters": [
                {"tags": {"title": "Synthetic chapter 1"}},
                {"tags": {"title": "Synthetic chapter 2"}},
                {"tags": {"title": "Synthetic chapter 3"}},
            ],
        }
        summary = pipeline_smoke.assert_media_probe(probe, scenario, "Result artifact", output=True)
        self.assertEqual(summary["streamCounts"], {"video": 1, "audio": 1, "subtitle": 0})
        self.assertEqual(summary["audioTracks"], [{
            "streamIndex": 1,
            "language": "eng",
            "title": "Original English Mono",
        }])
        self.assertEqual(summary["chapterTitles"], [title for _, _, title in scenario.chapters])
        for streams, chapters in (
            ([*probe["streams"], {"index": 2, "codec_type": "audio", "tags": {"language": "spa", "title": "Synthetic Spanish"}}], probe["chapters"]),
            ([*probe["streams"], {"index": 2, "codec_type": "subtitle"}], probe["chapters"]),
            (probe["streams"], probe["chapters"][:2]),
            ([probe["streams"][0], {**probe["streams"][1], "tags": {"language": "spa", "title": "Original Spanish Mono"}}], probe["chapters"]),
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

    def test_issue_285_cadence_scenarios_use_synthetic_cpu_fixtures_and_frozen_authority(self) -> None:
        progressive = pipeline_smoke.scenario_for("cadence-progressive")
        ivtc = pipeline_smoke.scenario_for("cadence-ivtc")
        self.assertEqual(progressive.expected_cadence_rate, "24000/1001")
        self.assertEqual(progressive.cadence_mode, "auto")
        self.assertEqual(progressive.duration_seconds, 90)
        self.assertEqual(progressive.fixture_video_source, "color=c=gray:s=320x180:r=24000/1001:d=90")
        self.assertEqual(progressive.fixture_frame_count, 2158)
        self.assertEqual(ivtc.fixture_frame_rate, "30000/1001")
        self.assertEqual(ivtc.expected_cadence_rate, "24000/1001")
        self.assertEqual(ivtc.expected_cadence_operation, "inverse_telecine")
        ambiguous = pipeline_smoke.scenario_for("cadence-ambiguous")
        self.assertIsNone(ambiguous.expected_frame_fidelity_status)
        self.assertIsNone(ambiguous.expected_frame_fidelity_rate_status)
        self.assertEqual(ambiguous.fixture_frame_count, 720)
        self.assertIsNone(ambiguous.output_frame_count)

        for scenario in (progressive, ivtc, ambiguous):
            with self.subTest(scenario=scenario.name), tempfile.TemporaryDirectory() as temp_base:
                root = pipeline_smoke.create_run_root(temp_base, f"issue-285-{scenario.name}")
                sandbox = root / "sandbox"
                pipeline_smoke.prepare_sandbox(sandbox)
                with patch.object(pipeline_smoke, "run_command") as run_command:
                    pipeline_smoke.generate_fixture(pipeline_smoke.DEFAULT_IMAGE, sandbox, scenario)
                command = run_command.call_args.args[0]
                self.assertEqual(command[command.index("--network") + 1], "none")
                self.assertIn("--read-only", command)
                self.assertEqual(command.count("--mount"), 1)
                if scenario is progressive:
                    self.assertIn("color=c=gray:s=320x180:r=24000/1001:d=90", command)
                elif scenario is ivtc:
                    self.assertIn("testsrc2=size=320x180:rate=24000/1001:duration=24", command)
                    self.assertEqual(command[command.index("-vf") + 1], "telecine=first_field=top")
                else:
                    self.assertIn("-filter_complex", command)
                    self.assertIn("[telecined][native]concat=n=2:v=1:a=0[smoke_video]", " ".join(command))
                    self.assertIn("-map", command)
                    self.assertIn("[smoke_video]", command)
                self.assertEqual(command[-1], "/media/raw/synthetic-smoke.mkv")

    def test_issue_285_cadence_assertions_bind_snapshot_plan_command_and_frame_fidelity(self) -> None:
        cases = (
            ("cadence-progressive", "preserve", "24000/1001", "format=yuv420p", "passed"),
            ("cadence-ivtc", "inverse_telecine", "24000/1001",
             "fieldmatch=order=tff:combmatch=full,bwdif=mode=send_frame:parity=tff:deint=interlaced,decimate,setfield=prog", "passed"),
        )
        for name, operation, rate, filters, fidelity_status in cases:
            with self.subTest(scenario=name):
                scenario = pipeline_smoke.scenario_for(name)
                worker = {
                    "cadenceMode": scenario.cadence_mode,
                    "fieldStructureMode": scenario.field_structure_mode,
                    "effectiveCadenceOperation": operation,
                    "effectiveOutputFrameRate": rate,
                    "effectiveOutputFrameRateUnknown": name == "cadence-ambiguous",
                    "effectiveCadenceFieldOrder": "tff" if name == "cadence-ivtc" else None,
                    "videoFilters": filters,
                }
                command_filter = filters or "format=yuv420p"
                command = f"ffmpeg -i /media/raw/synthetic-smoke.mkv -vf '{command_filter}'"
                if name == "cadence-ivtc":
                    command += " -fps_mode cfr"
                job = {
                    "id": 7, "activeExecutionPlanId": 11, "profileVersion": 2,
                    "profileSnapshot": {"workerConfig": worker},
                    "notes": f"Conversion command: {command}",
                }
                plan = {"id": 11, "jobId": 7, "profileVersion": 2}
                summary = pipeline_smoke.assert_cadence_execution(job, plan, scenario)
                self.assertEqual(summary["operation"], operation)
                validation = {"report": {"frameFidelity": {
                    "status": fidelity_status,
                    "fields": {"frameRate": {"status": scenario.expected_frame_fidelity_rate_status}},
                    "expected": {"frameRate": rate},
                    "actual": {"frameRate": rate},
                }}}
                fidelity = pipeline_smoke.assert_cadence_validation(validation, scenario)
                self.assertEqual(fidelity["status"], fidelity_status)

    def test_issue_285_ambiguous_cadence_passes_only_after_expected_preflight_rejection(self) -> None:
        queued = {
            "id": 7, "status": "queued", "stage": "queued", "profileSnapshot": None,
            "trackProfileSnapshot": None, "activeExecutionPlanId": None, "profileCapturedAt": None,
        }
        job = {
            "id": 7, "status": "failed", "stage": "failed",
            "errorMessage": (
                "claim-time Queue HEVC preflight: HEVC Level/Tier decision is unresolved: "
                "HEVC Level/Tier is unresolved because effective output FPS is unknown. "
                "Cadence resolution: inverse telecine is unresolved because an explicit or "
                "validated field order is required; review required."
            ),
            "profileSnapshot": {}, "trackProfileSnapshot": {}, "profileCapturedAt": None,
            "profileResolution": {}, "activeExecutionPlanId": None,
            "validationStatus": "", "validationReport": {}, "errorCode": None,
            "stageHistory": [
                {"stage": "queued"}, {"stage": "claimed"}, {"stage": "failed"},
            ],
            "notes": (
                "Synthetic isolated pipeline smoke; no production media\n"
                "Execution failed after the worker claimed the job; its reservation was released"
            ),
            "outputPath": "", "publishedPath": "", "publishedAt": None, "publishedSizeBytes": 0,
        }
        plans: list[dict[str, object]] = []
        artifacts: dict[str, object] = {"jobId": 7}
        validation_history = {"items": [{"id": 7, "status": "failed", "validationStatus": ""}]}
        publications = {"items": []}
        backend_log = json.dumps({
            "component": "system",
            "event": "auto_worker_claim_failed",
            "error": job["errorMessage"],
        })

        summary = pipeline_smoke.assert_ambiguous_cadence_preflight(
            queued, job, plans, artifacts, validation_history, publications, backend_log,
        )
        self.assertEqual(summary["preflight"]["status"], "PASS: expected negative rejection")
        self.assertEqual(summary["preflight"]["effectiveOutputFrameRate"], "unknown")
        self.assertEqual(summary["preflight"]["errorCode"], None)
        self.assertEqual(summary["preflight"]["structuredEvent"], "auto_worker_claim_failed")
        self.assertEqual(summary["executionPlan"], "NOT_CREATED")
        self.assertEqual(summary["workerExecution"]["ffmpegConversion"], "NOT_RUN")
        self.assertEqual(summary["result"], "NOT_CREATED")
        self.assertEqual(summary["publication"], "NOT_CREATED")
        self.assertEqual(summary["validation"]["validationRun"], "NOT_RUN_BY_DESIGN")
        self.assertEqual(summary["frameFidelity"], "NOT_RUN_BY_DESIGN")
        self.assertTrue(summary["overallAcceptance"].startswith("PASS:"))

        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, {**job, "errorMessage": "cadence guessed 24000/1001"}, plans, artifacts,
                validation_history, publications, backend_log,
            )
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, job, [{"id": 11, "jobId": 7}], artifacts,
                validation_history, publications, backend_log,
            )
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, job, plans, {"jobId": 7, "result": {"status": "ready"}},
                validation_history, publications, backend_log,
            )
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, job, plans, artifacts,
                {"items": [{"id": 7, "status": "failed", "validationStatus": "failed", "validationRunId": "run-1"}]},
                publications, backend_log,
            )

    def test_issue_285_ambiguous_cadence_fails_if_execution_or_publication_is_observed(self) -> None:
        queued = {
            "id": 7, "status": "queued", "stage": "queued", "profileSnapshot": None,
            "trackProfileSnapshot": None, "activeExecutionPlanId": None, "profileCapturedAt": None,
        }
        error = (
            "claim-time Queue HEVC preflight: HEVC Level/Tier decision is unresolved: "
            "HEVC Level/Tier is unresolved because effective output FPS is unknown. "
            "Cadence resolution: inverse telecine is unresolved because an explicit or "
            "validated field order is required; review required."
        )
        job = {
            "id": 7, "status": "failed", "stage": "failed", "errorMessage": error,
            "profileSnapshot": {}, "trackProfileSnapshot": {}, "profileResolution": {},
            "profileCapturedAt": None, "activeExecutionPlanId": None, "validationStatus": "",
            "validationReport": {}, "stageHistory": [
                {"stage": "queued"}, {"stage": "claimed"}, {"stage": "failed"},
            ],
            "notes": "Synthetic isolated pipeline smoke",
            "outputPath": "", "publishedPath": "", "publishedAt": None, "publishedSizeBytes": 0,
        }
        validation_history = {"items": [{"id": 7, "status": "failed", "validationStatus": ""}]}
        publications = {"items": []}
        backend_log = json.dumps({"event": "auto_worker_claim_failed", "error": error})

        cases = (
            ("completed job", {**job, "status": "completed", "stage": "completed"}),
            ("processing stage", {**job, "stageHistory": [
                {"stage": "queued"}, {"stage": "claimed"}, {"stage": "processing"}, {"stage": "failed"},
            ]}),
            ("FFmpeg command", {**job, "notes": "Conversion command: ffmpeg -i synthetic.mkv output.mkv"}),
            ("worker output", {**job, "outputPath": "/media/staging/job-7/output.mkv"}),
            ("published path", {**job, "publishedPath": "/media/library/published/output.mkv"}),
            ("concrete cadence", {**job, "profileResolution": {"effectiveOutputFrameRate": "24000/1001"}}),
        )
        for label, changed_job in cases:
            with self.subTest(observation=label), self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_ambiguous_cadence_preflight(
                    queued, changed_job, [], {"jobId": 7}, validation_history, publications, backend_log,
                )

        publication = {"items": [{"publication": {"queueJobId": 7, "state": "published"}}]}
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, job, [], {"jobId": 7}, validation_history, publication, backend_log,
            )

        wrong_event = json.dumps({"event": "worker_execution_started", "error": error})
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_ambiguous_cadence_preflight(
                queued, job, [], {"jobId": 7}, validation_history, publications, wrong_event,
            )

    def test_issue_285_ambiguous_cadence_cannot_reach_execution_or_validation_helpers(self) -> None:
        scenario = pipeline_smoke.scenario_for("cadence-ambiguous")
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_cadence_execution({}, {}, scenario)
        with self.assertRaises(pipeline_smoke.SmokeFailure):
            pipeline_smoke.assert_cadence_validation({}, scenario)

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
                {"index": 3, "codec_type": "subtitle", "codec_name": "subrip",
                 "tags": {"language": "eng", "title": "Synthetic English subtitles"}},
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
        publication_id, request_id = pipeline_smoke.assert_manual_publish_accepted({
            "status": "accepted",
            "requestId": "manual-request-41",
            "publicationId": "publication-41",
        })
        self.assertEqual(publication_id, "publication-41")
        self.assertEqual(request_id, "manual-request-41")
        for response in (
            {"status": "published", "requestId": "manual-request-41", "publicationId": "publication-41"},
            {"status": "accepted", "requestId": "manual-request-41"},
            {"status": "accepted", "publicationId": "publication-41"},
        ):
            with self.subTest(response=response), self.assertRaises(pipeline_smoke.SmokeFailure):
                pipeline_smoke.assert_manual_publish_accepted(response)

    def test_publisher_retry_waits_for_the_current_attempt_after_prior_block(self) -> None:
        blocked_prior_attempt = {
            "manualRequestId": "manual-request-first",
            "manualRequestStatus": "failed",
            "state": "BLOCKED",
        }
        self.assertEqual(
            pipeline_smoke.manual_publication_attempt_state(blocked_prior_attempt, "manual-request-retry"),
            "pending",
        )
        accepted_retry = {
            "manualRequestId": "manual-request-retry",
            "manualRequestStatus": "accepted",
            "state": "BLOCKED",
        }
        self.assertEqual(
            pipeline_smoke.manual_publication_attempt_state(accepted_retry, "manual-request-retry"),
            "pending",
        )
        failed_retry = {**accepted_retry, "manualRequestStatus": "failed"}
        self.assertEqual(
            pipeline_smoke.manual_publication_attempt_state(failed_retry, "manual-request-retry"),
            "failed",
        )
        published_retry = {**accepted_retry, "state": "PUBLISHED"}
        self.assertEqual(
            pipeline_smoke.manual_publication_attempt_state(published_retry, "manual-request-retry"),
            "published",
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
