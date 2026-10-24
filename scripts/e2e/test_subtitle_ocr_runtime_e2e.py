"""Regression tests for the self-contained issue-419 SeConv E2E fixtures."""

from __future__ import annotations

import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import subtitle_ocr_runtime_e2e as e2e


class SubtitleOCRRuntimeE2ETests(unittest.TestCase):
    def test_mkv_is_generated_from_versioned_sup_without_ignored_container_fixture(self) -> None:
        repository = Path(__file__).resolve().parents[2]
        source_fixtures = repository / "backend/internal/handlers/testdata/issue-419"
        source_manifest = json.loads((source_fixtures / "manifest.json").read_text(encoding="utf-8"))
        self.assertNotIn("two-bitmap-tracks.mkv", source_manifest["sha256"])

        with tempfile.TemporaryDirectory(prefix="issue-419-clean-checkout-") as temporary:
            fixture_copy = Path(temporary) / "issue-419"
            fixture_copy.mkdir()
            shutil.copy2(source_fixtures / "manifest.json", fixture_copy / "manifest.json")
            for relative in source_manifest["sha256"]:
                source = source_fixtures / relative
                destination = fixture_copy / relative
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)
            self.assertFalse((fixture_copy / "two-bitmap-tracks.mkv").exists())

            metadata, before_hashes = e2e.verify_fixture_manifest(fixture_copy)
            media = Path(temporary) / "generated" / "two-bitmap-tracks.mkv"
            commands: list[list[str]] = []
            command_environments: list[object] = []
            container_environment = {
                "MVFORGE_OCR_E2E_MOUNT_ROOT": temporary,
                "MVFORGE_OCR_E2E_IMAGE": "mvforge-backend:test",
            }

            def fake_runner(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
                commands.append(command)
                command_environments.append(kwargs.get("env"))
                if command[0] == "ffmpeg":
                    Path(command[-1]).write_bytes(b"synthetic temporary matroska")
                    return subprocess.CompletedProcess(command, 0, "", "")
                probe = {
                    "streams": [
                        {"index": 0, "codec_type": "video", "codec_name": "ffv1"},
                        {
                            "index": 1,
                            "codec_type": "subtitle",
                            "codec_name": "hdmv_pgs_subtitle",
                            "tags": {"language": "fra", "title": "synthetic-french"},
                        },
                        {
                            "index": 2,
                            "codec_type": "subtitle",
                            "codec_name": "hdmv_pgs_subtitle",
                            "tags": {"language": "kor", "title": "synthetic-korean"},
                        },
                    ],
                    "packets": [
                        {"stream_index": stream, "pts_time": str(timestamp)}
                        for stream in (1, 2)
                        for timestamp in (1.0, 2.5, 3.0, 4.5)
                    ],
                }
                return subprocess.CompletedProcess(command, 0, json.dumps(probe), "")

            e2e.generate_and_probe_mkv(
                "ffmpeg",
                "ffprobe",
                fixture_copy,
                media,
                metadata,
                runner=fake_runner,
                env=container_environment,
            )

            self.assertEqual(len(commands), 2)
            self.assertEqual(command_environments, [container_environment, container_environment])
            mux, probe = commands
            self.assertIn("-copyts", mux)
            self.assertEqual(mux[mux.index("-c:v") + 1], "ffv1")
            input_paths = [mux[index + 1] for index, arg in enumerate(mux[:-1]) if arg == "-i"]
            self.assertEqual(input_paths[1:], [str(fixture_copy / "french.sup"), str(fixture_copy / "korean.sup")])
            self.assertNotIn(str(fixture_copy / "two-bitmap-tracks.mkv"), input_paths)
            self.assertIn("-show_packets", probe)
            self.assertTrue(media.is_file())
            _, after_hashes = e2e.verify_fixture_manifest(fixture_copy)
            self.assertEqual(after_hashes, before_hashes)

    def test_generated_mkv_rejects_missing_ffprobe_cue_timestamps(self) -> None:
        metadata = {"pgs": {"cuesSeconds": [[1.0, 2.5], [3.0, 4.5]]}}
        probe = {
            "streams": [
                {"index": 0, "codec_type": "video", "codec_name": "ffv1"},
                {
                    "index": 1,
                    "codec_type": "subtitle",
                    "codec_name": "hdmv_pgs_subtitle",
                    "tags": {"language": "fra", "title": "synthetic-french"},
                },
                {
                    "index": 2,
                    "codec_type": "subtitle",
                    "codec_name": "hdmv_pgs_subtitle",
                    "tags": {"language": "kor", "title": "synthetic-korean"},
                },
            ],
            "packets": [{"stream_index": 1, "pts_time": "0.0"}],
        }
        with self.assertRaisesRegex(ValueError, "missing expected cue boundaries"):
            e2e.validate_generated_mkv(probe, metadata)


if __name__ == "__main__":
    unittest.main()
