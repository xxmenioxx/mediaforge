#!/usr/bin/env python3
"""Unit tests for the built OCR runtime image smoke."""

from __future__ import annotations

import unittest
from pathlib import Path
from unittest.mock import patch

import ocr_runtime_image_smoke


class OCRRuntimeImageSmokeTests(unittest.TestCase):
    def test_package_versions_are_declared_in_final_runtime_stage(self) -> None:
        dockerfile = Path(__file__).parents[2] / "backend" / "Dockerfile"
        lines = dockerfile.read_text(encoding="utf-8").splitlines()
        final_from = max(index for index, line in enumerate(lines) if line.startswith("FROM "))
        final_stage = lines[final_from:]
        install_run = next(
            index for index, line in enumerate(final_stage) if line.startswith("RUN apt-get update")
        )
        runtime_args = final_stage[:install_run]

        self.assertIn("ARG TESSERACT_PACKAGE_VERSION=5.5.0-1+b1", runtime_args)
        self.assertIn("ARG TESSERACT_LANGPACK_VERSION=1:4.1.0-2", runtime_args)

    def test_requires_existing_and_new_supported_languages(self) -> None:
        output = """List of available languages in \"/usr/share/tessdata/\" (6):
eng
fra
jpn
jpn_vert
kor
spa
"""
        self.assertEqual(
            ocr_runtime_image_smoke.require_languages(output),
            {"eng", "fra", "jpn", "jpn_vert", "kor", "spa"},
        )

    def test_missing_language_data_fails_instead_of_accepting_aliases(self) -> None:
        output = """List of available languages in \"/usr/share/tessdata/\" (6):
eng
fre
jpn
jpn_vert
spa
"""
        with self.assertRaisesRegex(
            RuntimeError,
            "missing required language data: fra, kor",
        ):
            ocr_runtime_image_smoke.require_languages(output)

    def test_runtime_container_command_uses_direct_arguments(self) -> None:
        self.assertEqual(
            ocr_runtime_image_smoke.container_command(
                "mvforge-backend:test", "tesseract", ["--list-langs"]
            ),
            [
                "docker",
                "run",
                "--rm",
                "--entrypoint",
                "tesseract",
                "mvforge-backend:test",
                "--list-langs",
            ],
        )

    def test_synthetic_model_probe_attaches_stdin_explicitly(self) -> None:
        self.assertEqual(
            ocr_runtime_image_smoke.container_command(
                "mvforge-backend:test",
                "tesseract",
                ["stdin", "stdout", "--psm", "6", "-l", "fra"],
                attach_stdin=True,
            ),
            [
                "docker",
                "run",
                "--rm",
                "-i",
                "--entrypoint",
                "tesseract",
                "mvforge-backend:test",
                "stdin",
                "stdout",
                "--psm",
                "6",
                "-l",
                "fra",
            ],
        )

    def test_package_inventory_requires_pinned_engine_and_language_pack_versions(self) -> None:
        output = "\n".join(
            f"{package}\t{version}\t100"
            for package, version in ocr_runtime_image_smoke.EXPECTED_PACKAGE_VERSIONS.items()
        )
        inventory = ocr_runtime_image_smoke.parse_package_inventory(output)
        self.assertEqual(inventory["tesseract-ocr-fra"], ("1:4.1.0-2", 100))

    def test_package_inventory_rejects_an_unexpected_version(self) -> None:
        rows = [
            f"{package}\t{'1:4.1.0-3' if package == 'tesseract-ocr-kor' else version}\t100"
            for package, version in ocr_runtime_image_smoke.EXPECTED_PACKAGE_VERSIONS.items()
        ]
        with self.assertRaisesRegex(RuntimeError, "pinned versions"):
            ocr_runtime_image_smoke.parse_package_inventory("\n".join(rows))

    def test_report_separates_capability_smoke_from_independent_seconv_e2e(self) -> None:
        languages = "eng\nfra\njpn\njpn_vert\nkor\nspa\n"
        packages = "\n".join(
            f"{package}\t{version}\t100"
            for package, version in ocr_runtime_image_smoke.EXPECTED_PACKAGE_VERSIONS.items()
        )
        with (
            patch.object(ocr_runtime_image_smoke, "run_checked", side_effect=[languages, packages, "1234"]),
            patch.object(ocr_runtime_image_smoke, "synthetic_model_probe", return_value=1.0),
        ):
            report = ocr_runtime_image_smoke.verify_image("mvforge-backend:test")

        self.assertEqual(report["runtime_capability_package_list_model_init"], "PASS")
        self.assertEqual(report["seconv_pgs_vobsub_recognition_e2e"], "NOT_RUN_BY_THIS_SMOKE")
        self.assertEqual(report["independent_seconv_e2e"], "NOT_RUN")

        with (
            patch.object(ocr_runtime_image_smoke, "run_checked", side_effect=[languages, packages, "1234"]),
            patch.object(ocr_runtime_image_smoke, "synthetic_model_probe", return_value=1.0),
        ):
            report = ocr_runtime_image_smoke.verify_image(
                "mvforge-backend:test", independent_seconv_e2e_status="PASS"
            )
        self.assertEqual(report["independent_seconv_e2e"], "PASS")

        with (
            patch.object(ocr_runtime_image_smoke, "run_checked", side_effect=[languages, packages, "1234"]),
            patch.object(ocr_runtime_image_smoke, "synthetic_model_probe", return_value=1.0),
        ):
            report = ocr_runtime_image_smoke.verify_image(
                "mvforge-backend:test", independent_seconv_e2e_status="FAIL"
            )
        self.assertEqual(report["independent_seconv_e2e"], "FAIL")


if __name__ == "__main__":
    unittest.main()
