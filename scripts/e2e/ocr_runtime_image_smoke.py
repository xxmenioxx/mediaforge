#!/usr/bin/env python3
"""Verify required OCR languages in a built MVForge backend image.

Run after building the supported image, for example:
  python3 scripts/e2e/ocr_runtime_image_smoke.py --image mvforge-backend:issue-419

This smoke checks runtime capability and model loadability using only a
synthetic blank PGM image. It does not run SeConv against a PGS/VobSub fixture
and does not measure recognition accuracy. The independent SeConv E2E status is
reported separately and can be supplied after that runner has executed.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
import time
from typing import Sequence


LANGUAGE_PACKAGES = {
    "eng": "tesseract-ocr-eng",
    "fra": "tesseract-ocr-fra",
    "jpn": "tesseract-ocr-jpn",
    "jpn_vert": "tesseract-ocr-jpn-vert",
    "kor": "tesseract-ocr-kor",
    "spa": "tesseract-ocr-spa",
}
EXPECTED_PACKAGE_VERSIONS = {
    "tesseract-ocr": "5.5.0-1+b1",
    **{package: "1:4.1.0-2" for package in LANGUAGE_PACKAGES.values()},
}
LANGUAGE_LINE = re.compile(r"^[a-z]{3}(?:_vert)?$")


def parse_languages(output: str) -> set[str]:
    return {
        line.strip()
        for line in output.splitlines()
        if LANGUAGE_LINE.fullmatch(line.strip())
    }


def require_languages(output: str) -> set[str]:
    reported = parse_languages(output)
    missing = sorted(set(LANGUAGE_PACKAGES) - reported)
    if missing:
        raise RuntimeError(
            "running Tesseract image is missing required language data: "
            + ", ".join(missing)
        )
    return reported


def run_checked(args: Sequence[str], *, input_bytes: bytes | None = None) -> str:
    result = subprocess.run(
        list(args),
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(
            f"command failed ({result.returncode}): {list(args)!r}: {detail}"
        )
    return result.stdout.decode("utf-8", errors="replace")


def container_command(
    image: str,
    entrypoint: str,
    args: Sequence[str],
    *,
    attach_stdin: bool = False,
) -> list[str]:
    command = ["docker", "run", "--rm"]
    if attach_stdin:
        command.append("-i")
    return [*command, "--entrypoint", entrypoint, image, *args]


def parse_package_inventory(output: str) -> dict[str, tuple[str, int]]:
    inventory: dict[str, tuple[str, int]] = {}
    for line in output.splitlines():
        fields = line.split("\t")
        if len(fields) != 3:
            raise RuntimeError(f"unexpected dpkg-query inventory row: {line!r}")
        package, version, size = fields
        inventory[package] = (version, int(size))
    expected = set(EXPECTED_PACKAGE_VERSIONS)
    if set(inventory) != expected:
        raise RuntimeError(
            "installed Tesseract package inventory differs from the required set: "
            f"missing={sorted(expected - set(inventory))}, "
            f"unexpected={sorted(set(inventory) - expected)}"
        )
    mismatched = {
        package: {"expected": EXPECTED_PACKAGE_VERSIONS[package], "actual": inventory[package][0]}
        for package in sorted(expected)
        if inventory[package][0] != EXPECTED_PACKAGE_VERSIONS[package]
    }
    if mismatched:
        raise RuntimeError(f"Tesseract package versions do not match pinned versions: {mismatched}")
    return inventory


def package_inventory(image: str) -> dict[str, tuple[str, int]]:
    template = "${Package}\t${Version}\t${Installed-Size}\n"
    output = run_checked(
        container_command(
            image,
            "dpkg-query",
            ["-W", f"-f={template}", *EXPECTED_PACKAGE_VERSIONS],
        )
    )
    return parse_package_inventory(output)


def synthetic_model_probe(image: str, language: str) -> float:
    # A blank PGM exercises model initialization without any real subtitle.
    width, height = 256, 256
    pgm = f"P5\n{width} {height}\n255\n".encode("ascii") + bytes([255]) * width * height
    command = container_command(
        image,
        "tesseract",
        ["stdin", "stdout", "--psm", "6", "-l", language],
        attach_stdin=True,
    )
    started = time.perf_counter()
    run_checked(command, input_bytes=pgm)
    return (time.perf_counter() - started) * 1000


def verify_image(
    image: str,
    *,
    independent_seconv_e2e_status: str = "NOT_RUN",
) -> dict[str, object]:
    languages_output = run_checked(
        container_command(image, "tesseract", ["--list-langs"])
    )
    languages = require_languages(languages_output)
    inventory = package_inventory(image)
    image_size = int(
        run_checked(
            ["docker", "image", "inspect", "--format={{.Size}}", image]
        ).strip()
    )
    probes = {
        language: round(synthetic_model_probe(image, language), 1)
        for language in ("fra", "kor")
    }
    return {
        "image": image,
        "image_bytes": image_size,
        "reported_languages": sorted(languages),
        "tesseract_package_versions_and_installed_kib": {
            package: {"version": version, "installed_kib": installed_kib}
            for package, (version, installed_kib) in inventory.items()
        },
        "fra_kor_package_installed_kib": sum(
            inventory[LANGUAGE_PACKAGES[language]][1] for language in ("fra", "kor")
        ),
        "synthetic_blank_model_probe_ms": probes,
        "runtime_capability_package_list_model_init": "PASS",
        "seconv_pgs_vobsub_recognition_e2e": "NOT_RUN_BY_THIS_SMOKE",
        "independent_seconv_e2e": independent_seconv_e2e_status,
        "accuracy_claim": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="built MVForge backend image")
    parser.add_argument(
        "--independent-seconv-e2e-status",
        choices=("PASS", "FAIL", "NOT_RUN"),
        default="NOT_RUN",
        help="status from a separately executed SeConv E2E; this smoke never runs that E2E",
    )
    args = parser.parse_args()
    try:
        report = verify_image(
            args.image,
            independent_seconv_e2e_status=args.independent_seconv_e2e_status,
        )
    except (OSError, RuntimeError, ValueError) as exc:
        print(f"OCR runtime image smoke failed: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
