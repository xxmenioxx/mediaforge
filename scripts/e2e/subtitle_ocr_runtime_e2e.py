#!/usr/bin/env python3
"""Run real MVForge bitmap OCR against disposable French/Korean PGS fixtures."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from typing import Any, Callable, Mapping, Sequence


WRAPPER = '''#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import sys

name = Path(sys.argv[0]).name
root = os.environ["MVFORGE_OCR_E2E_MOUNT_ROOT"]
image = os.environ["MVFORGE_OCR_E2E_IMAGE"]
command = [
    "docker", "run", "--rm", "--network", "none",
    "--volume", f"{root}:{root}",
    "--entrypoint", name, image,
    *sys.argv[1:],
]
result = subprocess.run(command, check=False)
raise SystemExit(result.returncode)
'''

EXPECTED_SUBTITLE_STREAMS = {
    1: {"language": "fra", "title": "synthetic-french"},
    2: {"language": "kor", "title": "synthetic-korean"},
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify_fixture_manifest(fixtures: Path) -> tuple[dict[str, Any], dict[str, str]]:
    manifest = fixtures / "manifest.json"
    if not manifest.is_file():
        raise FileNotFoundError(f"missing fixture manifest: {manifest}")
    metadata = json.loads(manifest.read_text(encoding="utf-8"))
    expected_hashes = metadata.get("sha256")
    if not isinstance(expected_hashes, dict):
        raise ValueError("fixture manifest must contain a sha256 object")
    required_sup = {"french.sup", "korean.sup"}
    if not required_sup.issubset(expected_hashes):
        raise ValueError(
            "fixture manifest is missing required versioned SUP hashes: "
            + ", ".join(sorted(required_sup - set(expected_hashes)))
        )

    root = fixtures.resolve()
    observed_hashes: dict[str, str] = {}
    for relative, expected in expected_hashes.items():
        fixture = (fixtures / relative).resolve()
        if not fixture.is_relative_to(root):
            raise ValueError(f"fixture path escapes issue-419 directory: {relative}")
        if not fixture.is_file():
            raise FileNotFoundError(f"missing versioned synthetic fixture: {fixture}")
        observed = sha256(fixture)
        if observed != expected:
            raise ValueError(f"fixture hash mismatch: {relative}: {observed} != {expected}")
        observed_hashes[relative] = observed
    return metadata, observed_hashes


def run_captured(
    command: Sequence[str],
    *,
    runner: Callable[..., Any] = subprocess.run,
    env: Mapping[str, str] | None = None,
) -> str:
    run_options: dict[str, Any] = {
        "stdout": subprocess.PIPE,
        "stderr": subprocess.PIPE,
        "text": True,
        "check": False,
    }
    if env is not None:
        run_options["env"] = dict(env)
    result = runner(
        list(command),
        **run_options,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"command failed ({result.returncode}): {list(command)!r}: "
            f"{result.stderr.strip()}"
        )
    return result.stdout


def validate_generated_mkv(probe: dict[str, Any], metadata: dict[str, Any]) -> None:
    streams = probe.get("streams")
    if not isinstance(streams, list):
        raise ValueError("ffprobe did not return a stream list for generated MKV")
    by_index = {stream.get("index"): stream for stream in streams}
    if len(streams) != 3 or set(by_index) != {0, 1, 2}:
        raise ValueError(f"generated MKV must contain exactly streams 0, 1, and 2: {streams!r}")
    video = by_index.get(0)
    if not video or video.get("codec_type") != "video" or video.get("codec_name") != "ffv1":
        raise ValueError(f"generated MKV stream 0 must be FFV1 video: {video!r}")

    for index, expected in EXPECTED_SUBTITLE_STREAMS.items():
        stream = by_index.get(index)
        tags = stream.get("tags", {}) if stream else {}
        if (
            not stream
            or stream.get("codec_type") != "subtitle"
            or stream.get("codec_name") != "hdmv_pgs_subtitle"
            or tags.get("language") != expected["language"]
            or tags.get("title") != expected["title"]
        ):
            raise ValueError(
                f"generated MKV stream {index} differs from the expected PGS track: "
                f"stream={stream!r} expected={expected!r}"
            )

    expected_pts = {
        round(float(value), 3)
        for interval in metadata["pgs"]["cuesSeconds"]
        for value in interval
    }
    timestamps_by_stream: dict[int, set[float]] = {index: set() for index in EXPECTED_SUBTITLE_STREAMS}
    for packet in probe.get("packets", []):
        stream_index = packet.get("stream_index")
        if stream_index not in timestamps_by_stream or packet.get("pts_time") is None:
            continue
        timestamps_by_stream[stream_index].add(round(float(packet["pts_time"]), 3))
    for index, timestamps in timestamps_by_stream.items():
        missing = sorted(expected_pts - timestamps)
        if missing:
            raise ValueError(
                f"ffprobe timestamps for subtitle stream {index} are missing expected cue "
                f"boundaries {missing}; observed={sorted(timestamps)}"
            )


def generate_and_probe_mkv(
    ffmpeg: str,
    ffprobe: str,
    fixtures: Path,
    media: Path,
    metadata: dict[str, Any],
    *,
    runner: Callable[..., Any] = subprocess.run,
    env: Mapping[str, str] | None = None,
) -> None:
    """Mux the tracked SUP inputs into a temporary MKV and verify its contract."""
    french = fixtures / "french.sup"
    korean = fixtures / "korean.sup"
    if not french.is_file() or not korean.is_file():
        raise FileNotFoundError("both versioned French and Korean SUP fixtures are required")
    media.parent.mkdir(parents=True, exist_ok=True)
    command = [
        ffmpeg,
        "-y",
        "-copyts",
        "-f", "lavfi",
        "-i", "color=c=black:s=1280x720:r=25:d=6",
        "-i", str(french),
        "-i", str(korean),
        "-map", "0:v:0",
        "-map", "1:s:0",
        "-map", "2:s:0",
        "-c:v", "ffv1",
        "-c:s", "copy",
        "-metadata:s:s:0", "language=fra",
        "-metadata:s:s:0", "title=synthetic-french",
        "-metadata:s:s:1", "language=kor",
        "-metadata:s:s:1", "title=synthetic-korean",
        "-shortest",
        str(media),
    ]
    run_captured(command, runner=runner, env=env)
    if not media.is_file() or media.stat().st_size == 0:
        raise RuntimeError("FFmpeg did not create the temporary two-track PGS MKV")

    probe_command = [
        ffprobe,
        "-v", "error",
        "-show_streams",
        "-show_packets",
        "-show_entries",
        "stream=index,codec_name,codec_type:stream_tags=language,title:packet=stream_index,pts_time",
        "-of", "json",
        str(media),
    ]
    probe_output = run_captured(probe_command, runner=runner, env=env)
    try:
        probe = json.loads(probe_output)
    except json.JSONDecodeError as exc:
        raise ValueError(f"ffprobe returned invalid JSON for generated MKV: {exc}") from exc
    validate_generated_mkv(probe, metadata)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="already-built supported MVForge backend image tag")
    args = parser.parse_args()

    repository = Path(__file__).resolve().parents[2]
    fixtures = repository / "backend/internal/handlers/testdata/issue-419"
    try:
        metadata, fixture_hashes = verify_fixture_manifest(fixtures)
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"invalid or incomplete versioned issue-419 fixtures: {exc}", file=sys.stderr)
        return 2

    inspect = subprocess.run(
        ["docker", "image", "inspect", args.image],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if inspect.returncode != 0:
        print(f"required image is not present: {args.image}\n{inspect.stderr.strip()}", file=sys.stderr)
        return inspect.returncode

    with tempfile.TemporaryDirectory(prefix="mvforge-419-ocr-e2e-") as temporary:
        mount_root = Path(temporary).resolve()
        temporary_fixtures = mount_root / "fixtures"
        temporary_fixtures.mkdir()
        for name in ("french.sup", "korean.sup"):
            shutil.copy2(fixtures / name, temporary_fixtures / name)
        media = mount_root / "two-bitmap-tracks.mkv"
        wrappers = mount_root / "bin"
        wrappers.mkdir()
        for name in ("ffmpeg", "ffprobe", "seconv", "tesseract"):
            wrapper = wrappers / name
            wrapper.write_text(WRAPPER, encoding="utf-8")
            wrapper.chmod(0o755)

        environment = os.environ.copy()
        environment["MVFORGE_OCR_E2E_IMAGE"] = args.image
        environment["MVFORGE_OCR_E2E_MOUNT_ROOT"] = str(mount_root)
        environment["MVFORGE_OCR_E2E_MEDIA"] = str(media)
        environment["MVFORGE_OCR_LIFECYCLE_ROOT"] = str(mount_root / "lifecycle")
        environment["PATH"] = f"{wrappers}{os.pathsep}{environment.get('PATH', '')}"

        try:
            generate_and_probe_mkv(
                str(wrappers / "ffmpeg"),
                str(wrappers / "ffprobe"),
                temporary_fixtures,
                media,
                metadata,
                env=environment,
            )
        except (OSError, RuntimeError, ValueError, KeyError) as exc:
            print(f"failed to generate or validate synthetic PGS MKV: {exc}", file=sys.stderr)
            return 1

        source_hash_before = sha256(media)
        command = [
            "go", "test", "./internal/handlers",
            "-run", "^TestFrenchKoreanBitmapOCRContainerE2E$",
            "-count=1", "-v",
        ]
        print(f"running real container OCR with image {args.image}", flush=True)
        print(
            "versioned fixture SHA-256 verified: "
            + ", ".join(f"{name}={digest}" for name, digest in sorted(fixture_hashes.items())),
            flush=True,
        )
        print(f"generated temporary MKV SHA-256 before OCR: {source_hash_before}", flush=True)
        result = subprocess.run(command, cwd=repository / "backend", env=environment, check=False)
        try:
            source_hash_after = sha256(media)
            _, fixture_hashes_after = verify_fixture_manifest(fixtures)
        except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
            print(f"failed to revalidate source fixtures after OCR: {exc}", file=sys.stderr)
            return 1
        if source_hash_after != source_hash_before:
            print("OCR flow modified the generated temporary MKV", file=sys.stderr)
            return 1
        if fixture_hashes_after != fixture_hashes:
            print("OCR flow modified one or more versioned PGS/PNG fixtures", file=sys.stderr)
            return 1
        print(f"generated temporary MKV SHA-256 after OCR: {source_hash_after}", flush=True)
        print(f"independent SeConv E2E: {'PASS' if result.returncode == 0 else 'FAIL'}", flush=True)
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
