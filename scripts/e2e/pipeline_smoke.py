#!/usr/bin/env python3
"""Run an isolated synthetic MVForge pipeline smoke scenario."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from collections import Counter
from dataclasses import dataclass, replace
from fractions import Fraction
from pathlib import Path, PurePosixPath
from typing import Any, Sequence


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_IMAGE = "mvforge-backend:pipeline-smoke"
RUN_PREFIX = "mvforge-e2e-"
MAX_TOTAL_EVIDENCE_BYTES = 1024 * 1024
MAX_JSON_BYTES = 128 * 1024
MAX_LOG_BYTES = 256 * 1024
FORBIDDEN_HOST_ROOTS = (Path("/volume1"), Path("/volume2"))


@dataclass(frozen=True)
class SyntheticAudioTrack:
    frequency_hz: int
    language: str
    title: str


@dataclass(frozen=True)
class SyntheticScenario:
    name: str
    frame_rate: str
    duration_seconds: int
    audio_tracks: tuple[SyntheticAudioTrack, ...]
    subtitle_language: str
    subtitle_title: str
    subtitle_text: str
    chapters: tuple[tuple[int, int, str], ...]
    kept_audio_stream_indexes: tuple[int, ...] | None = None
    track_profile_scope: str = "asset"
    track_profile_audio_mode: str | None = None
    track_profile_audio_languages: tuple[str, ...] = ()
    track_profile_subtitle_mode: str | None = None
    track_profile_subtitle_languages: tuple[str, ...] = ()
    expected_resolved_audio_languages: tuple[str, ...] = ()
    expected_resolved_subtitle_language: str | None = None
    subtitle_action: str = "keep"
    subtitle_sidecar_formats: tuple[str, ...] = ()
    source_video_codec: str = "h264"
    output_video_codec: str = "hevc"
    video_width: int = 320
    video_height: int = 180
    fixture_video_source: str | None = None
    fixture_frame_rate: str | None = None
    fixture_input_frame_rate: str | None = None
    fixture_video_filter: str | None = None
    fixture_video_filter_graph: str | None = None
    fixture_interlaced: bool = False
    fixture_field_order: str = "progressive"
    output_field_order: str = "progressive"
    cadence_mode: str = "preserve"
    field_structure_mode: str = "preserve"
    expected_cadence_operation: str | None = None
    expected_cadence_rate: str | None = None
    expected_cadence_field_order: str | None = None
    expected_frame_fidelity_status: str | None = None
    expected_frame_fidelity_rate_status: str | None = None
    fixture_frame_count: int | None = None
    output_frame_count: int | None = None

    @property
    def audio_stream_indexes(self) -> tuple[int, ...]:
        return tuple(range(1, len(self.audio_tracks) + 1))

    @property
    def subtitle_stream_index(self) -> int:
        return len(self.audio_tracks) + 1

    @property
    def expected_stream_counts(self) -> dict[str, int]:
        return {"video": 1, "audio": len(self.audio_tracks), "subtitle": 1}

    @property
    def output_audio_stream_indexes(self) -> tuple[int, ...]:
        if self.kept_audio_stream_indexes is not None:
            return self.kept_audio_stream_indexes
        return self.audio_stream_indexes

    @property
    def expected_output_stream_counts(self) -> dict[str, int]:
        embedded_subtitles = int(self.subtitle_action in {"keep", "keep_and_extract"})
        return {"video": 1, "audio": len(self.output_audio_stream_indexes), "subtitle": embedded_subtitles}

    @property
    def expected_output_audio_tracks(self) -> tuple[SyntheticAudioTrack, ...]:
        return tuple(self.audio_tracks[index - 1] for index in self.output_audio_stream_indexes)


HAPPY_PATH_SCENARIO = SyntheticScenario(
    name="happy-path",
    frame_rate="24000/1001",
    duration_seconds=8,
    audio_tracks=(
        SyntheticAudioTrack(440, "eng", "Synthetic English"),
        SyntheticAudioTrack(880, "spa", "Synthetic Spanish"),
    ),
    subtitle_language="eng",
    subtitle_title="Synthetic English subtitles",
    subtitle_text="MVForge synthetic pipeline subtitle",
    chapters=(
        (0, 2500, "Synthetic chapter 1"),
        (2500, 5500, "Synthetic chapter 2"),
        (5500, 8000, "Synthetic chapter 3"),
    ),
)
TRACK_ACTIONS_SCENARIO = SyntheticScenario(
    name="track-actions",
    frame_rate=HAPPY_PATH_SCENARIO.frame_rate,
    duration_seconds=HAPPY_PATH_SCENARIO.duration_seconds,
    audio_tracks=HAPPY_PATH_SCENARIO.audio_tracks,
    subtitle_language=HAPPY_PATH_SCENARIO.subtitle_language,
    subtitle_title=HAPPY_PATH_SCENARIO.subtitle_title,
    subtitle_text=HAPPY_PATH_SCENARIO.subtitle_text,
    chapters=HAPPY_PATH_SCENARIO.chapters,
    kept_audio_stream_indexes=(1,),
    subtitle_action="extract",
    subtitle_sidecar_formats=("srt", "ass"),
)
LANGUAGE_NORMALIZATION_SCENARIO = replace(
    TRACK_ACTIONS_SCENARIO,
    name="language-normalization",
    audio_tracks=(
        SyntheticAudioTrack(440, "kor", "Synthetic Korean"),
        SyntheticAudioTrack(880, "eng", "Synthetic English"),
    ),
    subtitle_language="fra",
    subtitle_title="Synthetic French subtitles",
    track_profile_audio_mode="languages",
    track_profile_audio_languages=("ko-KR",),
    track_profile_subtitle_mode="languages",
    track_profile_subtitle_languages=("fr-FR",),
    expected_resolved_audio_languages=("kor",),
    expected_resolved_subtitle_language="fra",
    track_profile_scope="path",
)
CADENCE_PROGRESSIVE_SCENARIO = replace(
    HAPPY_PATH_SCENARIO,
    name="cadence-progressive",
    duration_seconds=90,
    chapters=((0, 30000, "Synthetic chapter 1"), (30000, 60000, "Synthetic chapter 2"), (60000, 90000, "Synthetic chapter 3")),
    cadence_mode="auto",
    field_structure_mode="auto",
    fixture_video_source="color=c=gray:s=320x180:r=24000/1001:d=90",
    expected_cadence_operation="preserve",
    expected_cadence_rate="24000/1001",
    expected_frame_fidelity_status="passed",
    expected_frame_fidelity_rate_status="passed",
    fixture_frame_count=2158,
    output_frame_count=2158,
)
CADENCE_IVTC_SCENARIO = replace(
    HAPPY_PATH_SCENARIO,
    name="cadence-ivtc",
    duration_seconds=24,
    chapters=((0, 8000, "Synthetic chapter 1"), (8000, 16000, "Synthetic chapter 2"), (16000, 24000, "Synthetic chapter 3")),
    cadence_mode="auto",
    field_structure_mode="auto",
    fixture_frame_rate="30000/1001",
    fixture_input_frame_rate="24000/1001",
    fixture_video_filter="telecine=first_field=top",
    fixture_interlaced=True,
    fixture_field_order="tb",
    expected_cadence_operation="inverse_telecine",
    expected_cadence_rate="24000/1001",
    expected_cadence_field_order="tff",
    expected_frame_fidelity_status="passed",
    expected_frame_fidelity_rate_status="passed",
    fixture_frame_count=720,
    output_frame_count=576,
)
CADENCE_AMBIGUOUS_SCENARIO = replace(
    HAPPY_PATH_SCENARIO,
    name="cadence-ambiguous",
    duration_seconds=24,
    chapters=((0, 8000, "Synthetic chapter 1"), (8000, 16000, "Synthetic chapter 2"), (16000, 24000, "Synthetic chapter 3")),
    frame_rate="30000/1001",
    fixture_frame_rate="30000/1001",
    fixture_video_filter_graph=(
        "[0:v]setpts=PTS-STARTPTS,telecine=first_field=top[telecined];"
        "[1:v]setpts=PTS-STARTPTS[native];"
        "[telecined][native]concat=n=2:v=1:a=0[smoke_video]"
    ),
    fixture_interlaced=True,
    fixture_field_order="tb",
    cadence_mode="auto",
    field_structure_mode="auto",
    fixture_frame_count=720,
)
SCENARIOS = {
    HAPPY_PATH_SCENARIO.name: HAPPY_PATH_SCENARIO,
    TRACK_ACTIONS_SCENARIO.name: TRACK_ACTIONS_SCENARIO,
    LANGUAGE_NORMALIZATION_SCENARIO.name: LANGUAGE_NORMALIZATION_SCENARIO,
    CADENCE_PROGRESSIVE_SCENARIO.name: CADENCE_PROGRESSIVE_SCENARIO,
    CADENCE_IVTC_SCENARIO.name: CADENCE_IVTC_SCENARIO,
    CADENCE_AMBIGUOUS_SCENARIO.name: CADENCE_AMBIGUOUS_SCENARIO,
}
SUPPORTED_SCENARIOS = frozenset(SCENARIOS)
REQUIRED_FINDING_STATES = {
    "artifact.identity": "PASS",
    "artifact.manifest": "PASS",
    "artifact.integrity": "PASS",
    "execution.binding": "PASS",
    "duration.timeline": "PASS",
    "video.properties": "PASS",
    "audio.tracks_actions": "PASS",
    "subtitles.streams_actions": "PASS",
    "chapters.actions": "PASS",
    # Publisher performs readiness and destination checks after this immutable run.
    "publication.readiness": "NOT_APPLICABLE",
}


def scenario_for(name: str) -> SyntheticScenario:
    try:
        return SCENARIOS[name]
    except KeyError as exc:
        raise SmokeFailure(f"unsupported scenario {name!r}; available: {sorted(SCENARIOS)}") from exc


def track_profile_for_scenario(key: str, scenario: SyntheticScenario) -> dict[str, Any]:
    profile: dict[str, Any] = {
        "key": key,
        "scope": scenario.track_profile_scope,
        "trackDispositionVersion": 1,
        "keepVideoStreams": [0],
        "subtitleDisposition": scenario.subtitle_action,
        "subtitleSidecarFormats": list(scenario.subtitle_sidecar_formats),
        "attachmentPolicy": "remove",
        "chapterPolicy": "keep",
    }
    if scenario.track_profile_audio_mode is not None:
        profile["audioMode"] = scenario.track_profile_audio_mode
        profile["audioLanguages"] = list(scenario.track_profile_audio_languages)
    else:
        profile["keepAudioStreams"] = list(scenario.output_audio_stream_indexes)
    if scenario.track_profile_subtitle_mode is not None:
        profile["subtitleMode"] = scenario.track_profile_subtitle_mode
        profile["subtitleLanguages"] = list(scenario.track_profile_subtitle_languages)
    else:
        profile["keepSubtitleStreams"] = [scenario.subtitle_stream_index]
    return profile
PIPELINE_AUTOMATION_SETTINGS = {
    "autoAnalysisEnabled": False,
    "reviewMode": "automatic",
    "autoExecutionEnabled": True,
    "autoValidationEnabled": False,
    "autoPublisherEnabled": False,
    "publisherOverwriteEnabled": False,
    "publishedJobReconciliationEnabled": False,
    "forceFreshSnapshotBeforeExecution": False,
}
SYNTHETIC_RUNTIME_POLICY = {
    "schemaVersion": 2,
    "mode": "manual",
    "preferredProfile": "desktop_safe",
    "fallbackProfile": "desktop_safe",
    "overrides": {
        "desktop_safe": {
            "minFreeWorkGb": 0,
            "minFreeLibraryGb": 0,
            "allowDirectMode": True,
        },
    },
}
SYNTHETIC_WORKSPACE_SETTINGS = {
    "preferredMode": "copy_to_work_disk",
    "fallbackMode": "direct",
    "allowDirectMode": True,
    "estimateRequiredSpace": True,
}


class SmokeFailure(RuntimeError):
    pass


def is_within(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def validate_temp_base(value: str | Path) -> Path:
    candidate = Path(value).expanduser().absolute()
    if candidate == Path("/") or any(part in {"volume1", "volume2"} for part in candidate.parts):
        raise SmokeFailure("temporary root cannot be the filesystem root or a NAS media path")
    try:
        base = candidate.resolve(strict=True)
    except OSError as exc:
        raise SmokeFailure(f"temporary root does not exist: {candidate}") from exc
    if not base.is_dir():
        raise SmokeFailure(f"temporary root is not a directory: {base}")
    if os.environ.get("GITHUB_ACTIONS", "").lower() == "true" and os.environ.get("RUNNER_TEMP"):
        approved_temp = Path(os.environ["RUNNER_TEMP"]).expanduser().resolve(strict=True)
    else:
        approved_temp = Path(tempfile.gettempdir()).resolve(strict=True)
    if not is_within(base, approved_temp):
        raise SmokeFailure(f"temporary root must be inside the approved temporary directory {approved_temp}")
    repo = REPO_ROOT.resolve()
    if is_within(base, repo):
        raise SmokeFailure("temporary root must be outside the repository checkout")
    if base == Path("/") or any(is_within(base, forbidden) for forbidden in FORBIDDEN_HOST_ROOTS):
        raise SmokeFailure("temporary root cannot be the filesystem root or a NAS media path")
    if any(part in {"volume1", "volume2"} for part in base.parts):
        raise SmokeFailure("temporary root path contains a forbidden NAS root")
    return base


def create_run_root(temp_base: str | Path, run_id: str | None = None) -> Path:
    base = validate_temp_base(temp_base)
    if run_id is None:
        root = Path(tempfile.mkdtemp(prefix=RUN_PREFIX, dir=base))
    else:
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,100}", run_id):
            raise SmokeFailure("run id must contain only letters, digits, dot, underscore, and hyphen")
        root = base / f"{RUN_PREFIX}{run_id}"
        root.mkdir(mode=0o700)
    resolved = root.resolve(strict=True)
    if resolved.parent != base or not resolved.name.startswith(RUN_PREFIX):
        raise SmokeFailure("created run root escaped its approved temporary parent")
    return resolved


class Evidence:
    def __init__(self, directory: Path) -> None:
        self.directory = directory
        self.directory.mkdir(parents=True, exist_ok=True)

    def write(self, name: str, content: bytes, max_file_bytes: int = MAX_JSON_BYTES) -> None:
        path = self.directory / name
        existing_total = sum(
            entry.stat().st_size
            for entry in self.directory.iterdir()
            if entry.is_file() and entry != path
        )
        allowance = max(0, min(max_file_bytes, MAX_TOTAL_EVIDENCE_BYTES - existing_total))
        marker = b"\n[truncated by bounded evidence writer]\n"
        if len(content) > allowance:
            if allowance > len(marker):
                content = content[: allowance - len(marker)] + marker
            else:
                content = content[:allowance]
        path.write_bytes(content)

    def json(self, name: str, value: Any) -> None:
        encoded = (json.dumps(value, indent=2, sort_keys=True, default=str) + "\n").encode()
        self.write(name, encoded)


def docker_mount(sandbox: Path) -> str:
    source = sandbox.resolve(strict=True)
    if "," in str(source):
        raise SmokeFailure("temporary sandbox paths cannot contain commas")
    if any(is_within(source, forbidden) for forbidden in FORBIDDEN_HOST_ROOTS):
        raise SmokeFailure("sandbox cannot be inside a NAS media root")
    return f"type=bind,src={source},dst=/media"


def host_user() -> str:
    if not hasattr(os, "getuid") or not hasattr(os, "getgid"):
        raise SmokeFailure("the isolated Docker smoke requires a POSIX host UID/GID")
    return f"{os.getuid()}:{os.getgid()}"


def runtime_container_args(name: str, image: str, sandbox: Path) -> list[str]:
    env = {
        "HOME": "/media/config/home",
        "XDG_CACHE_HOME": "/media/cache/xdg",
        "XDG_CONFIG_HOME": "/media/config/xdg",
        "XDG_DATA_HOME": "/media/config/data",
        "XDG_STATE_HOME": "/media/config/state",
        "TMPDIR": "/media/tmp",
        "TMP": "/media/tmp",
        "TEMP": "/media/tmp",
        "MVFORGE_API_HOST": "127.0.0.1",
        "MVFORGE_API_PORT": "8080",
        "MVFORGE_DB_PATH": "/media/config/mvforge.db",
        "MVFORGE_OCR_LIFECYCLE_ROOT": "/media/ocr/lifecycle",
        "MVFORGE_OCR_PERSISTENT_ROOT": "/media/ocr/persistent",
    }
    args = [
        "docker", "run", "--detach", "--name", name,
        "--network", "none", "--read-only", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges:true", "--user", host_user(), "--pids-limit", "256",
        "--memory", "3g", "--cpus", "2",
        "--mount", docker_mount(sandbox),
    ]
    for key, value in env.items():
        args.extend(("--env", f"{key}={value}"))
    args.append(image)
    return args


def validate_container_inspect(data: dict[str, Any], sandbox: Path) -> None:
    host = data.get("HostConfig") or {}
    if host.get("NetworkMode") != "none":
        raise SmokeFailure("container network is not isolated")
    if host.get("ReadonlyRootfs") is not True:
        raise SmokeFailure("container root filesystem is writable")
    if host.get("Binds") not in (None, []):
        raise SmokeFailure("unexpected legacy Docker bind mounts are configured")
    cap_drop = {str(value).upper() for value in host.get("CapDrop", [])}
    if "ALL" not in cap_drop:
        raise SmokeFailure("container does not drop all Linux capabilities")
    if "no-new-privileges:true" not in (host.get("SecurityOpt") or []):
        raise SmokeFailure("container does not disable privilege escalation")
    if host.get("Devices") or host.get("DeviceRequests"):
        raise SmokeFailure("container has an unexpected host device or GPU request")
    if host.get("PortBindings") not in (None, {}):
        raise SmokeFailure("container unexpectedly publishes host ports")
    if (data.get("Config") or {}).get("User") != host_user():
        raise SmokeFailure("container is not running as the host user that owns the isolated sandbox")

    mounts = data.get("Mounts") or []
    if len(mounts) != 1:
        raise SmokeFailure("container must have exactly one host mount")
    mount = mounts[0]
    if mount.get("Type") != "bind" or mount.get("Destination") != "/media":
        raise SmokeFailure("the only host mount must be the temporary sandbox at /media")
    if Path(str(mount.get("Source", ""))).resolve() != sandbox.resolve():
        raise SmokeFailure("container mount source does not match this run's sandbox")
    if mount.get("RW") is not True:
        raise SmokeFailure("temporary sandbox mount must be writable for this smoke run")


def run_command(
    args: Sequence[str],
    *,
    timeout: int = 120,
    cwd: Path = REPO_ROOT,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        list(args), cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        timeout=timeout, check=False,
    )
    if check and result.returncode != 0:
        detail = result.stdout[-4000:]
        raise SmokeFailure(f"command failed ({result.returncode}): {list(args)!r}\n{detail}")
    return result


def ensure_image(image: str, build_if_missing: bool) -> None:
    inspected = run_command(["docker", "image", "inspect", image], timeout=30, check=False)
    if inspected.returncode == 0:
        return
    if not build_if_missing:
        raise SmokeFailure(f"Docker image is unavailable: {image}")
    print(f"Building {image} from backend/Dockerfile", flush=True)
    result = subprocess.run(
        ["docker", "build", "--tag", image, "--file", "backend/Dockerfile", "backend"],
        cwd=REPO_ROOT,
        check=False,
    )
    if result.returncode != 0:
        raise SmokeFailure(f"docker build failed with exit code {result.returncode}")


def generate_fixture(
    image: str,
    sandbox: Path,
    scenario: SyntheticScenario,
    *,
    video_filter: str | None = None,
    interlaced: bool = False,
) -> None:
    fixture_directory = sandbox / "fixtures"
    fixture_directory.mkdir(parents=True, exist_ok=True)
    (fixture_directory / "synthetic-smoke.srt").write_text(
        "1\n00:00:01,000 --> 00:00:02,500\n"
        f"{scenario.subtitle_text}\n",
        encoding="utf-8",
    )
    chapter_metadata = [";FFMETADATA1"]
    for start, end, title in scenario.chapters:
        chapter_metadata.extend((
            "",
            "[CHAPTER]",
            "TIMEBASE=1/1000",
            f"START={start}",
            f"END={end}",
            f"title={title}",
        ))
    (fixture_directory / "synthetic-smoke.ffmeta").write_text(
        "\n".join(chapter_metadata) + "\n",
        encoding="utf-8",
    )

    effective_video_filter = video_filter if video_filter is not None else scenario.fixture_video_filter
    effective_interlaced = interlaced or scenario.fixture_interlaced
    input_rate = scenario.frame_rate if scenario.fixture_input_frame_rate is None else scenario.fixture_input_frame_rate
    video_source = scenario.fixture_video_source or (
        f"testsrc2=size={scenario.video_width}x{scenario.video_height}:rate={input_rate}:duration={scenario.duration_seconds}"
    )
    args = [
        "docker", "run", "--rm", "--network", "none", "--read-only",
        "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
        "--user", host_user(),
        "--mount", docker_mount(sandbox),
        "--env", "HOME=/media/config/home", "--env", "TMPDIR=/media/tmp",
        "--entrypoint", "ffmpeg", image,
        "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
        "-f", "lavfi", "-i", video_source,
    ]
    if scenario.fixture_video_filter_graph:
        segment_seconds = scenario.duration_seconds // 2
        args[-1] = f"testsrc2=size={scenario.video_width}x{scenario.video_height}:rate=24000/1001:duration={segment_seconds}"
        args.extend((
            "-f", "lavfi", "-i",
            f"testsrc2=size={scenario.video_width}x{scenario.video_height}:rate=30000/1001:duration={segment_seconds}",
        ))
    for track in scenario.audio_tracks:
        args.extend((
            "-f", "lavfi", "-i",
            f"sine=frequency={track.frequency_hz}:sample_rate=48000:duration={scenario.duration_seconds}",
        ))
    first_audio_input_index = 2 if scenario.fixture_video_filter_graph else 1
    subtitle_input_index = first_audio_input_index + len(scenario.audio_tracks)
    chapter_input_index = subtitle_input_index + 1
    args.extend(("-i", "/media/fixtures/synthetic-smoke.srt", "-f", "ffmetadata", "-i", "/media/fixtures/synthetic-smoke.ffmeta"))
    if scenario.fixture_video_filter_graph:
        args.extend(("-filter_complex", scenario.fixture_video_filter_graph, "-map", "[smoke_video]"))
    else:
        args.extend(("-map", "0:v:0"))
    for audio_input_index in range(1, len(scenario.audio_tracks) + 1):
        args.extend(("-map", f"{first_audio_input_index + audio_input_index - 1}:a:0"))
    args.extend((
        "-map", f"{subtitle_input_index}:s:0",
        "-map_chapters", str(chapter_input_index),
    ))
    if effective_video_filter:
        args.extend(("-vf", effective_video_filter))
    args.extend((
        "-c:v", "libx264", "-profile:v", "high", "-preset", "ultrafast",
        "-threads", "1", "-pix_fmt", "yuv420p",
        "-c:a", "aac", "-b:a", "64k", "-c:s", "srt",
        "-fflags", "+bitexact",
        "-metadata", "title=MVForge synthetic pipeline smoke",
        "-metadata", "creation_time=2000-01-01T00:00:00Z",
    ))
    if effective_interlaced:
        args.extend(("-flags", "+ilme+ildct", "-top", "1"))
    for index, track in enumerate(scenario.audio_tracks):
        args.extend((
            f"-metadata:s:a:{index}", f"language={track.language}",
            f"-metadata:s:a:{index}", f"title={track.title}",
        ))
    args.extend((
        "-metadata:s:s:0", f"language={scenario.subtitle_language}",
        "-metadata:s:s:0", f"title={scenario.subtitle_title}",
        "-disposition:a:0", "default",
        "-disposition:a:1", "0",
        "-disposition:s:0", "default",
        "/media/raw/synthetic-smoke.mkv",
    ))
    run_command(args, timeout=90)


def probe_media(container: str, media_path: str) -> dict[str, Any]:
    result = run_command(
        [
            "docker", "exec", container, "ffprobe", "-v", "error",
            "-count_frames", "-show_chapters",
            "-show_entries",
            "format=duration:stream=index,codec_type,codec_name,width,height,sample_rate,channels,avg_frame_rate,r_frame_rate,field_order,pix_fmt,nb_read_frames:stream_tags=language,title:chapter=id,start_time,end_time:chapter_tags=title",
            "-of", "json", media_path,
        ],
        timeout=30,
    )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise SmokeFailure(f"ffprobe returned invalid JSON for {media_path}") from exc


def assert_media_probe(
    probe: Any,
    scenario: SyntheticScenario,
    label: str,
    *,
    expected_video_codec: str | None = None,
    output: bool = False,
    require_frame_count: bool = True,
) -> dict[str, Any]:
    if not isinstance(probe, dict):
        raise SmokeFailure(f"{label} ffprobe evidence is not an object")
    streams = probe.get("streams")
    if not isinstance(streams, list):
        raise SmokeFailure(f"{label} ffprobe evidence has no stream list")
    counts = Counter(
        str(stream.get("codec_type"))
        for stream in streams
        if isinstance(stream, dict) and stream.get("codec_type")
    )
    actual_counts = {
        kind: counts.get(kind, 0)
        for kind in ("video", "audio", "subtitle")
    }
    expected_counts = scenario.expected_output_stream_counts if output else scenario.expected_stream_counts
    if actual_counts != expected_counts or len(streams) != sum(expected_counts.values()):
        raise SmokeFailure(
            f"{label} stream counts are {actual_counts} across {len(streams)} streams; "
            f"expected {expected_counts}"
        )
    video = next(stream for stream in streams if isinstance(stream, dict) and stream.get("codec_type") == "video")
    rate_value = scenario.frame_rate if output or scenario.fixture_frame_rate is None else scenario.fixture_frame_rate
    expected_rate = Fraction(rate_value)
    for field in ("avg_frame_rate", "r_frame_rate"):
        try:
            actual_rate = Fraction(str(video.get(field, "")))
        except (ValueError, ZeroDivisionError) as exc:
            raise SmokeFailure(f"{label} video has no usable {field}") from exc
        if actual_rate != expected_rate:
            raise SmokeFailure(f"{label} video {field}={actual_rate}, expected {expected_rate}")
    expected_field_order = scenario.output_field_order if output else scenario.fixture_field_order
    if video.get("field_order") != expected_field_order:
        raise SmokeFailure(f"{label} video field_order={video.get('field_order')!r}, expected {expected_field_order!r}")
    if video.get("width") != scenario.video_width or video.get("height") != scenario.video_height:
        raise SmokeFailure(
            f"{label} video geometry is {video.get('width')}x{video.get('height')}; "
            f"expected {scenario.video_width}x{scenario.video_height}"
        )
    if expected_video_codec and video.get("codec_name") != expected_video_codec:
        raise SmokeFailure(
            f"{label} video codec={video.get('codec_name')!r}, expected {expected_video_codec!r}"
        )
    expected_frame_count = scenario.output_frame_count if output else scenario.fixture_frame_count
    if require_frame_count and expected_frame_count is not None:
        try:
            actual_frame_count = int(video.get("nb_read_frames", video.get("nb_frames", "")))
        except (TypeError, ValueError) as exc:
            raise SmokeFailure(f"{label} video has no counted frame total") from exc
        if actual_frame_count != expected_frame_count:
            raise SmokeFailure(f"{label} video frame count={actual_frame_count}, expected {expected_frame_count}")
    expected_audio_tracks = scenario.expected_output_audio_tracks if output else scenario.audio_tracks
    actual_audio_tracks = [stream for stream in streams if isinstance(stream, dict) and stream.get("codec_type") == "audio"]
    for index, (actual, expected) in enumerate(zip(actual_audio_tracks, expected_audio_tracks), start=1):
        tags = actual.get("tags")
        if (
            not isinstance(tags, dict)
            or tags.get("language") != expected.language
            or (not output and tags.get("title") != expected.title)
        ):
            raise SmokeFailure(
                f"{label} audio track {index} metadata is {tags!r}; "
                f"expected language={expected.language!r}"
                + (f", title={expected.title!r}" if not output else "")
            )
    expected_subtitle_languages = (
        [scenario.subtitle_language]
        if not output or scenario.subtitle_action in {"keep", "keep_and_extract"}
        else []
    )
    actual_subtitle_tracks = [
        stream for stream in streams
        if isinstance(stream, dict) and stream.get("codec_type") == "subtitle"
    ]
    if len(actual_subtitle_tracks) != len(expected_subtitle_languages):
        raise SmokeFailure(
            f"{label} subtitle stream count is {len(actual_subtitle_tracks)}; "
            f"expected {len(expected_subtitle_languages)}"
        )
    for index, (actual, expected_language) in enumerate(
        zip(actual_subtitle_tracks, expected_subtitle_languages), start=1,
    ):
        tags = actual.get("tags")
        if not isinstance(tags, dict) or tags.get("language") != expected_language:
            raise SmokeFailure(
                f"{label} subtitle track {index} metadata is {tags!r}; "
                f"expected language={expected_language!r}"
            )
    chapters = probe.get("chapters")
    if not isinstance(chapters, list) or len(chapters) != len(scenario.chapters):
        actual = len(chapters) if isinstance(chapters, list) else "unavailable"
        raise SmokeFailure(
            f"{label} chapter count is {actual}; expected exactly {len(scenario.chapters)}"
        )
    expected_chapter_titles = [title for _, _, title in scenario.chapters]
    actual_chapter_titles = [
        chapter.get("tags", {}).get("title") if isinstance(chapter, dict) else None
        for chapter in chapters
    ]
    if actual_chapter_titles != expected_chapter_titles:
        raise SmokeFailure(
            f"{label} chapter titles are {actual_chapter_titles!r}; expected {expected_chapter_titles!r}"
        )
    media_format = probe.get("format")
    try:
        duration = float(media_format.get("duration")) if isinstance(media_format, dict) else 0.0
    except (TypeError, ValueError):
        duration = 0.0
    if duration <= 0 or abs(duration - scenario.duration_seconds) > 2:
        raise SmokeFailure(f"{label} duration={duration}, outside the existing 2-second smoke tolerance")
    return {
        "streamCounts": actual_counts,
        "audioTracks": [
            {
                "language": stream.get("tags", {}).get("language"),
                "title": stream.get("tags", {}).get("title"),
            }
            for stream in actual_audio_tracks
        ],
        "subtitleTracks": [
            {
                "language": stream.get("tags", {}).get("language"),
                "title": stream.get("tags", {}).get("title"),
            }
            for stream in actual_subtitle_tracks
        ],
        "chapterCount": len(chapters),
        "chapterTitles": actual_chapter_titles,
        "durationSeconds": duration,
        "videoFrameRate": str(expected_rate),
        "videoFrameCount": int(video["nb_read_frames"]) if str(video.get("nb_read_frames", "")).isdigit() else None,
        "videoFieldOrder": video.get("field_order"),
        "videoCodec": video.get("codec_name"),
    }


def summarize_probe(probe: Any) -> dict[str, Any]:
    if not isinstance(probe, dict):
        return {"error": "ffprobe evidence unavailable"}
    stream_fields = (
        "index", "codec_type", "codec_name", "width", "height", "sample_rate", "channels",
        "avg_frame_rate", "r_frame_rate", "field_order", "pix_fmt", "nb_read_frames",
    )
    streams = []
    for stream in probe.get("streams", []):
        if not isinstance(stream, dict):
            continue
        summary = {key: stream.get(key) for key in stream_fields if key in stream}
        tags = stream.get("tags")
        if isinstance(tags, dict):
            summary["tags"] = {key: tags[key] for key in ("language", "title") if key in tags}
        streams.append(summary)
    chapters = []
    for chapter in probe.get("chapters", []):
        if not isinstance(chapter, dict):
            continue
        summary = {key: chapter.get(key) for key in ("id", "start_time", "end_time") if key in chapter}
        tags = chapter.get("tags")
        if isinstance(tags, dict) and "title" in tags:
            summary["title"] = tags["title"]
        chapters.append(summary)
    media_format = probe.get("format")
    return {
        "format": {
            key: media_format[key]
            for key in ("duration", "size", "bit_rate", "format_name")
            if isinstance(media_format, dict) and key in media_format
        },
        "streams": streams,
        "chapters": chapters,
    }


def result_artifact_evidence(result_artifact: dict[str, Any], result_path: str) -> dict[str, Any]:
    primary = (result_artifact.get("artifacts") or {}).get("primary")
    return {
        "kind": result_artifact.get("kind"),
        "generatedAt": result_artifact.get("generatedAt"),
        "resultIdentity": result_artifact.get("resultIdentity"),
        "executionBinding": result_artifact.get("executionBinding"),
        "artifactManifest": result_artifact.get("artifacts"),
        "primary": primary,
        "resultReportPath": result_path,
        "outputProbe": summarize_probe(result_artifact.get("outputProbe")),
    }


def assert_subtitle_sidecar_content(path: Path, format_name: str, expected_text: str) -> None:
    try:
        content = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        raise SmokeFailure(f"{format_name.upper()} subtitle sidecar is missing or not UTF-8: {path}") from exc
    if expected_text not in content:
        raise SmokeFailure(f"{format_name.upper()} subtitle sidecar lost the synthetic cue text: {path}")
    if format_name == "srt":
        cue = re.search(
            r"(?m)^\s*1\s*\r?\n\d{2}:\d{2}:\d{2}[,.]\d{3}\s+-->\s+"
            r"\d{2}:\d{2}:\d{2}[,.]\d{3}\s*\r?\n[^\r\n]+",
            content,
        )
        if cue is None:
            raise SmokeFailure(f"SRT subtitle sidecar has no valid first cue: {path}")
    elif format_name == "ass":
        if not all(marker in content for marker in ("[Script Info]", "[Events]", "Format:", "Dialogue:")):
            raise SmokeFailure(f"ASS subtitle sidecar has no valid script/events/dialogue structure: {path}")
    else:
        raise SmokeFailure(f"unsupported required subtitle sidecar format {format_name!r}")


def assert_result_artifact_manifest(
    result_artifact: dict[str, Any],
    job: dict[str, Any],
    scenario: SyntheticScenario,
    sandbox: Path,
) -> dict[str, Any]:
    artifact_manifest = result_artifact.get("artifacts")
    if not isinstance(artifact_manifest, dict):
        raise SmokeFailure("Result artifact has no artifact manifest")
    track_snapshot = job.get("trackProfileSnapshot")
    resolved = track_snapshot.get("resolvedTrackPlan") if isinstance(track_snapshot, dict) else None
    if not isinstance(resolved, dict):
        raise SmokeFailure("Result artifact cannot be checked without the frozen track plan")
    planned = resolved.get("sidecarOutputs")
    if not isinstance(planned, list):
        raise SmokeFailure("frozen track plan has no sidecar output list")
    expected_keys = []
    for sidecar in planned:
        if not isinstance(sidecar, dict):
            raise SmokeFailure("frozen track plan contains a malformed sidecar output")
        try:
            expected_keys.append((int(sidecar["streamIndex"]), str(sidecar["format"]).lower()))
        except (KeyError, TypeError, ValueError) as exc:
            raise SmokeFailure(f"frozen sidecar output is missing stream/format identity: {sidecar!r}") from exc
        if sidecar.get("language") != scenario.subtitle_language:
            raise SmokeFailure(
                f"frozen sidecar language is {sidecar.get('language')!r}; "
                f"expected {scenario.subtitle_language!r}"
            )
    scenario_keys = sorted((scenario.subtitle_stream_index, value.lower()) for value in scenario.subtitle_sidecar_formats)
    if sorted(expected_keys) != scenario_keys:
        raise SmokeFailure(f"frozen sidecar plan is {sorted(expected_keys)!r}; expected {scenario_keys!r}")

    raw_entries = artifact_manifest.get("sidecars")
    if raw_entries is None:
        entries = []
    elif isinstance(raw_entries, list):
        entries = raw_entries
    else:
        raise SmokeFailure("Result artifact manifest sidecars are not a list")
    actual_keys = []
    artifact_ids = []
    summaries = []
    for entry in entries:
        if not isinstance(entry, dict) or entry.get("type") != "subtitle_sidecar":
            raise SmokeFailure(f"Result artifact manifest contains an unexpected required artifact: {entry!r}")
        try:
            key = (int(entry["streamIndex"]), str(entry["format"]).lower())
        except (KeyError, TypeError, ValueError) as exc:
            raise SmokeFailure(f"Result sidecar manifest entry has no stream/format identity: {entry!r}") from exc
        artifact_id = entry.get("artifactId")
        if not isinstance(artifact_id, str) or not artifact_id.strip():
            raise SmokeFailure(f"Result sidecar manifest entry has no artifact ID: {entry!r}")
        if entry.get("status") != "ready":
            raise SmokeFailure(f"required Result sidecar {artifact_id} is not ready: {entry.get('status')!r}")
        if entry.get("language") != scenario.subtitle_language:
            raise SmokeFailure(
                f"Result sidecar {artifact_id} language is {entry.get('language')!r}; "
                f"expected {scenario.subtitle_language!r}"
            )
        staged_path = container_media_path(entry.get("stagedPath"), f"{key[1].upper()} Result sidecar path")
        host_path = host_path_for_media(sandbox, staged_path, f"{key[1].upper()} Result sidecar path")
        if not host_path.is_file() or host_path.stat().st_size == 0:
            raise SmokeFailure(f"required Result sidecar {artifact_id} is missing or empty: {staged_path}")
        assert_subtitle_sidecar_content(host_path, key[1], scenario.subtitle_text)
        actual_keys.append(key)
        artifact_ids.append(artifact_id)
        summaries.append({
            "artifactId": artifact_id,
            "streamIndex": key[0],
            "format": key[1],
            "language": entry.get("language"),
            "status": entry.get("status"),
            "stagedPath": staged_path,
            "sizeBytes": host_path.stat().st_size,
            "sha256": sha256_file(host_path),
        })
    if sorted(actual_keys) != sorted(expected_keys):
        raise SmokeFailure(
            f"Result sidecar manifest identities are {sorted(actual_keys)!r}; expected exactly {sorted(expected_keys)!r}"
        )
    if len(artifact_ids) != len(set(artifact_ids)):
        raise SmokeFailure("Result sidecar manifest contains duplicate artifact IDs")
    return {"requiredArtifactIds": sorted(artifact_ids), "sidecars": summaries}


def assert_published_sidecars(
    published_job: dict[str, Any],
    manifest_summary: dict[str, Any],
    scenario: SyntheticScenario,
    sandbox: Path,
    published_media_path: str,
) -> list[dict[str, Any]]:
    artifacts = published_job.get("subtitleArtifacts")
    if not isinstance(artifacts, list):
        if manifest_summary["requiredArtifactIds"]:
            raise SmokeFailure("published QueueJob has no subtitle artifact records")
        return []
    sidecars = [entry for entry in artifacts if isinstance(entry, dict) and entry.get("type") == "subtitle_sidecar"]
    if len(sidecars) != len(artifacts):
        raise SmokeFailure("published QueueJob contains an unexpected required artifact type")
    expected_ids = manifest_summary["requiredArtifactIds"]
    actual_ids = [entry.get("artifactId") for entry in sidecars]
    if any(not isinstance(value, str) or not value for value in actual_ids):
        raise SmokeFailure("published sidecar record has no artifact ID")
    if sorted(actual_ids) != expected_ids:
        raise SmokeFailure(f"published sidecar IDs are {sorted(actual_ids)!r}; expected {expected_ids!r}")
    expected_by_id = {
        item.get("artifactId"): item
        for item in manifest_summary["sidecars"]
        if isinstance(item, dict) and isinstance(item.get("artifactId"), str)
    }
    published_media_path = container_media_path(published_media_path, "published media path")
    if not published_media_path.startswith("/media/library/published/"):
        raise SmokeFailure(f"Publisher wrote the synthetic media outside the isolated Library: {published_media_path}")
    media_base = str(PurePosixPath(published_media_path).with_suffix(""))
    summaries = []
    for entry in sidecars:
        format_name = str(entry.get("format", "")).lower()
        language = str(entry.get("language", "")).strip().lower()
        if language != scenario.subtitle_language or format_name not in {value.lower() for value in scenario.subtitle_sidecar_formats}:
            raise SmokeFailure(f"published subtitle identity is outside the synthetic scenario: {entry!r}")
        disposition = ".forced" if entry.get("forced") else ".default" if entry.get("default") else ""
        if str(entry.get("role", "")).lower() == "sdh":
            disposition += ".sdh"
        published_path = f"{media_base}.{language}{disposition}.{format_name}"
        exposed_path = entry.get("publishedPath")
        if exposed_path:
            exposed_path = container_media_path(exposed_path, "published subtitle sidecar path")
            if exposed_path != published_path:
                raise SmokeFailure(
                    f"Publisher sidecar path {exposed_path} differs from the delivered manifest path {published_path}"
                )
        if not published_path.startswith("/media/library/published/"):
            raise SmokeFailure(f"Publisher wrote a required sidecar outside the isolated Library: {published_path}")
        published_host_path = host_path_for_media(sandbox, published_path, "published subtitle sidecar path")
        if not published_host_path.is_file() or published_host_path.stat().st_size == 0:
            raise SmokeFailure(f"Publisher did not deliver required subtitle sidecar {published_path}")
        expected = expected_by_id.get(entry.get("artifactId"))
        if not isinstance(expected, dict) or sha256_file(published_host_path) != expected.get("sha256"):
            raise SmokeFailure(f"published subtitle sidecar differs from the Result artifact: {published_path}")
        format_name = str(entry.get("format", "")).lower()
        assert_subtitle_sidecar_content(published_host_path, format_name, scenario.subtitle_text)
        summaries.append({
            "artifactId": entry.get("artifactId"),
            "format": format_name,
            "publishedPath": published_path,
            "apiPublishedPath": exposed_path or "",
            "sizeBytes": published_host_path.stat().st_size,
        })
    return summaries


def validation_run_evidence(detail: dict[str, Any]) -> dict[str, Any]:
    run = detail.get("run")
    if not isinstance(run, dict):
        return {"error": "ValidationRun detail unavailable"}
    run_fields = (
        "runId", "jobId", "executionNumber", "executionPlanId", "executionPlanVersion",
        "executionPlanFingerprint", "resultArtifactId", "resultGenerationToken",
        "resultGenerationVersion", "resultOutputRevision", "schemaVersion", "validatorVersion",
        "rulesetId", "rulesetVersion", "policySchemaVersion", "policyVersion", "policyPreset",
        "policyFingerprint", "actorSource", "actorId", "outcome", "score", "createdAt",
    )
    finding_fields = (
        "key", "state", "severity", "disposition", "actionable", "overrideable",
        "expected", "actual", "explanation", "evidenceReferences",
    )
    findings = [
        {key: finding.get(key) for key in finding_fields if key in finding}
        for finding in run.get("findings", [])
        if isinstance(finding, dict)
    ]
    return {
        "run": {key: run[key] for key in run_fields if key in run} | {"findings": findings},
        "effectiveHandoff": detail.get("effectiveHandoff"),
        "policyCurrent": detail.get("policyCurrent"),
    }


class API:
    def __init__(self, container: str, evidence: Evidence) -> None:
        self.container = container
        self.evidence = evidence

    def request(
        self,
        method: str,
        path: str,
        *,
        payload: dict[str, Any] | None = None,
        expected: tuple[int, ...] = (200,),
        evidence_name: str | None = None,
    ) -> Any:
        args = [
            "docker", "exec", self.container, "curl", "--silent", "--show-error",
            "--connect-timeout", "2", "--max-time", "30", "--request", method,
            "--header", "Accept: application/json",
            "--write-out", "\n%{http_code}",
        ]
        if payload is not None:
            args.extend(("--header", "Content-Type: application/json", "--data-binary", json.dumps(payload)))
        args.append(f"http://127.0.0.1:8080{path}")
        response = run_command(args, timeout=40)
        try:
            body_text, status_text = response.stdout.rsplit("\n", 1)
            status = int(status_text.strip())
        except (ValueError, TypeError) as exc:
            raise SmokeFailure(f"could not parse HTTP status for {method} {path}") from exc
        try:
            body: Any = json.loads(body_text) if body_text.strip() else {}
        except json.JSONDecodeError:
            body = {"raw": body_text[:MAX_JSON_BYTES].decode("utf-8", "replace") if isinstance(body_text, bytes) else body_text[:MAX_JSON_BYTES]}
        if evidence_name:
            self.evidence.json(evidence_name, body)
        if status not in expected:
            raise SmokeFailure(f"HTTP {status} for {method} {path}: {json.dumps(body, default=str)[:3000]}")
        return body


def container_media_path(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.startswith("/media/"):
        raise SmokeFailure(f"{label} must resolve inside the isolated /media filesystem")
    normalized = PurePosixPath(value)
    if ".." in normalized.parts:
        raise SmokeFailure(f"{label} contains a path traversal")
    return str(normalized)


def host_path_for_media(sandbox: Path, media_path: str, label: str) -> Path:
    normalized = PurePosixPath(container_media_path(media_path, label))
    relative = normalized.relative_to("/media")
    host_path = (sandbox / Path(*relative.parts)).resolve(strict=True)
    if not is_within(host_path, sandbox.resolve(strict=True)):
        raise SmokeFailure(f"{label} escaped the isolated sandbox")
    return host_path


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def assert_frozen_snapshots(job: dict[str, Any], scenario: SyntheticScenario) -> dict[str, Any]:
    profile = job.get("profileSnapshot")
    if not isinstance(profile, dict) or not profile:
        raise SmokeFailure("completed QueueJob is missing its frozen profile snapshot")
    if profile.get("preserveSubtitles") is not True or profile.get("preserveChapters") is not True:
        raise SmokeFailure("frozen ProfileSnapshot does not require subtitle and chapter preservation")
    if not job.get("profileCapturedAt"):
        raise SmokeFailure("completed QueueJob is missing its profile capture timestamp")

    track_snapshot = job.get("trackProfileSnapshot")
    if not isinstance(track_snapshot, dict) or not track_snapshot:
        raise SmokeFailure("completed QueueJob is missing its frozen track snapshot")
    resolved = track_snapshot.get("resolvedTrackPlan")
    if not isinstance(resolved, dict):
        raise SmokeFailure("frozen track snapshot has no resolvedTrackPlan")
    audio_streams = resolved.get("audioStreams")
    subtitle_streams = resolved.get("subtitleStreams")
    audio_indexes = sorted(
        int(stream["streamIndex"])
        for stream in audio_streams
        if isinstance(stream, dict) and "streamIndex" in stream
    ) if isinstance(audio_streams, list) else []
    if audio_indexes != list(scenario.output_audio_stream_indexes):
        raise SmokeFailure(
            f"frozen audio stream selection is {audio_indexes}; expected {list(scenario.output_audio_stream_indexes)}"
        )
    if scenario.expected_resolved_audio_languages:
        selected_languages = sorted(
            str(stream.get("language"))
            for stream in audio_streams
            if isinstance(stream, dict) and stream.get("language") is not None
        ) if isinstance(audio_streams, list) else []
        expected_languages = sorted(scenario.expected_resolved_audio_languages)
        if selected_languages != expected_languages:
            raise SmokeFailure(
                f"frozen audio languages are {selected_languages!r}; expected canonical {expected_languages!r}"
            )
    removed_streams = resolved.get("removedAudioStreams")
    removed_audio_indexes = sorted(
        int(stream["streamIndex"])
        for stream in removed_streams
        if isinstance(stream, dict) and "streamIndex" in stream
    ) if isinstance(removed_streams, list) else []
    expected_removed_indexes = sorted(set(scenario.audio_stream_indexes) - set(scenario.output_audio_stream_indexes))
    if removed_audio_indexes != expected_removed_indexes:
        raise SmokeFailure(
            f"frozen removed audio streams are {removed_audio_indexes}; expected {expected_removed_indexes}"
        )
    if not isinstance(subtitle_streams, list) or len(subtitle_streams) != 1:
        raise SmokeFailure("frozen track snapshot must select exactly one subtitle stream")
    subtitle = subtitle_streams[0]
    if not isinstance(subtitle, dict) or (
        subtitle.get("streamIndex") != scenario.subtitle_stream_index or subtitle.get("action") != scenario.subtitle_action
    ):
        raise SmokeFailure(f"frozen subtitle action is unexpected: {subtitle!r}")
    if (
        scenario.expected_resolved_subtitle_language is not None
        and subtitle.get("language") != scenario.expected_resolved_subtitle_language
    ):
        raise SmokeFailure(
            f"frozen subtitle language is {subtitle.get('language')!r}; "
            f"expected canonical {scenario.expected_resolved_subtitle_language!r}"
        )
    sidecar_outputs = resolved.get("sidecarOutputs")
    if not isinstance(sidecar_outputs, list):
        raise SmokeFailure("frozen track snapshot has no sidecar output list")
    actual_sidecar_keys = sorted(
        (int(sidecar["streamIndex"]), str(sidecar["format"]).lower())
        for sidecar in sidecar_outputs
        if isinstance(sidecar, dict) and "streamIndex" in sidecar and "format" in sidecar
    )
    expected_sidecar_keys = sorted(
        (scenario.subtitle_stream_index, value.lower()) for value in scenario.subtitle_sidecar_formats
    )
    if actual_sidecar_keys != expected_sidecar_keys or len(actual_sidecar_keys) != len(sidecar_outputs):
        raise SmokeFailure(
            f"frozen sidecar outputs are {actual_sidecar_keys!r}; expected exactly {expected_sidecar_keys!r}"
        )
    if resolved.get("chapterPolicy") != "keep" or resolved.get("chaptersKept") is not True:
        raise SmokeFailure("frozen track snapshot does not require preserving chapters")
    return {
        "profileCapturedAt": job.get("profileCapturedAt"),
        "profileSnapshotKeys": sorted(profile.keys()),
        "trackProfileSnapshotKeys": sorted(track_snapshot.keys()),
        "audioStreamIndexes": audio_indexes,
        "audioLanguages": sorted(
            str(stream.get("language")) for stream in audio_streams
            if isinstance(stream, dict) and stream.get("language") is not None
        ) if isinstance(audio_streams, list) else [],
        "removedAudioStreamIndexes": removed_audio_indexes,
        "subtitleStreamIndex": subtitle.get("streamIndex"),
        "subtitleLanguage": subtitle.get("language"),
        "subtitleAction": subtitle.get("action"),
        "sidecarOutputs": actual_sidecar_keys,
        "chapterPolicy": resolved.get("chapterPolicy"),
        "chaptersKept": resolved.get("chaptersKept"),
    }


def assert_cadence_execution(
    job: dict[str, Any], plan: dict[str, Any], scenario: SyntheticScenario,
) -> dict[str, Any] | None:
    if scenario.name == "cadence-ambiguous":
        raise SmokeFailure("ambiguous cadence must be rejected before ExecutionPlan or FFmpeg execution")
    if not scenario.name.startswith("cadence-"):
        return None
    profile = job.get("profileSnapshot")
    worker = profile.get("workerConfig") if isinstance(profile, dict) else None
    if not isinstance(worker, dict):
        raise SmokeFailure("cadence scenario QueueJob has no frozen worker configuration")
    if worker.get("cadenceMode") != scenario.cadence_mode:
        raise SmokeFailure(f"frozen cadenceMode={worker.get('cadenceMode')!r}, expected {scenario.cadence_mode!r}")
    if worker.get("fieldStructureMode") != scenario.field_structure_mode:
        raise SmokeFailure(
            f"frozen fieldStructureMode={worker.get('fieldStructureMode')!r}, expected {scenario.field_structure_mode!r}"
        )
    operation = worker.get("effectiveCadenceOperation")
    output_rate = worker.get("effectiveOutputFrameRate")
    filters = str(worker.get("videoFilters") or "").lower()
    if scenario.expected_cadence_operation is not None and operation != scenario.expected_cadence_operation:
        raise SmokeFailure(
            f"frozen cadence operation={operation!r}, expected {scenario.expected_cadence_operation!r}"
        )
    if scenario.expected_cadence_rate is not None and output_rate != scenario.expected_cadence_rate:
        raise SmokeFailure(
            f"frozen cadence output frame rate={output_rate!r}, expected {scenario.expected_cadence_rate!r}"
        )
    if scenario.expected_cadence_field_order is not None and worker.get("effectiveCadenceFieldOrder") != scenario.expected_cadence_field_order:
        raise SmokeFailure(
            f"frozen IVTC field order={worker.get('effectiveCadenceFieldOrder')!r}, "
            f"expected {scenario.expected_cadence_field_order!r}"
        )
    cadence_filters = ("fieldmatch=", "decimate", "fps=", "bwdif=", "yadif=")
    if scenario.name == "cadence-progressive":
        if operation not in {None, "", "preserve"} or any(token in filters for token in cadence_filters):
            raise SmokeFailure(f"progressive preserve unexpectedly changed cadence: {operation!r}, {filters!r}")
    elif scenario.name == "cadence-ivtc":
        for token in ("fieldmatch=order=tff", "decimate", "setfield=prog"):
            if token not in filters:
                raise SmokeFailure(f"frozen IVTC chain is missing {token!r}: {filters!r}")

    if plan.get("jobId") != job.get("id") or plan.get("id") != job.get("activeExecutionPlanId"):
        raise SmokeFailure("cadence Queue snapshot is not bound to the active ExecutionPlan")
    if isinstance(job.get("profileVersion"), int) and plan.get("profileVersion") != job.get("profileVersion"):
        raise SmokeFailure("active ExecutionPlan profile version differs from the frozen QueueJob")

    notes = job.get("notes")
    notes_text = "\n".join(notes) if isinstance(notes, list) else str(notes or "")
    commands = re.findall(r"(?m)Conversion command:\s*(ffmpeg[^\r\n]*)", notes_text)
    if not commands:
        raise SmokeFailure("QueueJob notes do not expose the executed FFmpeg conversion command")
    try:
        command_args = shlex.split(commands[-1])
    except ValueError as exc:
        raise SmokeFailure("executed FFmpeg command in QueueJob notes is not parseable") from exc
    command_filter = ""
    for option in ("-vf", "-filter:v"):
        if option in command_args:
            command_filter = command_args[command_args.index(option) + 1].lower()
            break
    if scenario.name == "cadence-ivtc":
        for token in ("fieldmatch=order=tff", "decimate", "setfield=prog"):
            if token not in command_filter:
                raise SmokeFailure(f"executed FFmpeg chain is missing {token!r}: {command_filter!r}")
        if "-fps_mode" not in command_args or command_args[command_args.index("-fps_mode") + 1] != "cfr":
            raise SmokeFailure("executed IVTC command did not apply the frozen constant-frame-rate output mode")
    elif any(token in command_filter for token in cadence_filters):
        raise SmokeFailure(f"executed FFmpeg command unexpectedly transformed progressive preserve: {command_filter!r}")
    return {
        "queueJobId": job.get("id"),
        "executionPlanId": plan.get("id"),
        "profileVersion": plan.get("profileVersion"),
        "cadenceMode": worker.get("cadenceMode"),
        "fieldStructureMode": worker.get("fieldStructureMode"),
        "operation": operation,
        "effectiveOutputFrameRate": output_rate,
        "effectiveOutputFrameRateUnknown": worker.get("effectiveOutputFrameRateUnknown"),
        "fieldOrder": worker.get("effectiveCadenceFieldOrder"),
        "frozenVideoFilters": worker.get("videoFilters"),
        "executedVideoFilter": command_filter,
        "commandOutputMode": command_args[command_args.index("-fps_mode") + 1] if "-fps_mode" in command_args else None,
    }


def assert_ambiguous_cadence_preflight(
    queued: dict[str, Any],
    job: dict[str, Any],
    plans: Any,
    artifacts: dict[str, Any],
    validation_history: dict[str, Any],
    publications: dict[str, Any],
    backend_log: str,
) -> dict[str, Any]:
    if queued.get("status") != "queued" or queued.get("stage") != "queued":
        raise SmokeFailure("ambiguous cadence Queue request did not remain lightweight queued intent")
    if any(queued.get(key) not in (None, {}) for key in (
        "profileSnapshot", "trackProfileSnapshot", "activeExecutionPlanId", "profileCapturedAt",
    )):
        raise SmokeFailure("ambiguous cadence Queue request unexpectedly contained a frozen execution snapshot")

    if job.get("status") != "failed" or job.get("stage") != "failed":
        raise SmokeFailure(f"ambiguous cadence preflight did not fail closed: {job.get('status')!r}/{job.get('stage')!r}")
    stage_history = job.get("stageHistory")
    stages = [row.get("stage") for row in stage_history if isinstance(row, dict)] if isinstance(stage_history, list) else []
    if not stages or stages[-1] != "failed" or any(stage in {"processing", "completed", "published"} for stage in stages):
        raise SmokeFailure(f"ambiguous cadence reached execution instead of failing at preflight: {stages!r}")

    error = str(job.get("errorMessage") or "")
    required_error_evidence = (
        "claim-time Queue HEVC preflight",
        "effective output FPS is unknown",
        "inverse telecine is unresolved because an explicit or validated field order is required",
        "review required",
    )
    missing_error_evidence = [part for part in required_error_evidence if part not in error]
    if missing_error_evidence:
        raise SmokeFailure(f"ambiguous cadence failure lacks resolver/preflight evidence {missing_error_evidence!r}: {error!r}")

    if job.get("profileSnapshot") not in (None, {}) or job.get("trackProfileSnapshot") not in (None, {}):
        raise SmokeFailure("claim-time cadence preflight froze Profile or Track state before rejecting the Queue job")
    profile_resolution = job.get("profileResolution")
    if profile_resolution is not None and not isinstance(profile_resolution, dict):
        raise SmokeFailure("ambiguous cadence preflight returned an invalid profile resolution")
    if isinstance(profile_resolution, dict):
        output_rate = profile_resolution.get("effectiveOutputFrameRate")
        if output_rate not in (None, "", "unknown"):
            raise SmokeFailure(f"ambiguous cadence preflight persisted a guessed expected FPS: {output_rate!r}")
        if profile_resolution.get("effectiveOutputFrameRateUnknown") is False:
            raise SmokeFailure("ambiguous cadence preflight marked its unresolved expected FPS as known")
    if job.get("profileCapturedAt") or job.get("activeExecutionPlanId") is not None:
        raise SmokeFailure("claim-time cadence preflight created frozen execution authority before rejecting the job")
    if not isinstance(plans, list) or any(
        isinstance(plan, dict) and plan.get("jobId") == job.get("id") for plan in plans
    ):
        raise SmokeFailure("ambiguous cadence preflight created an ExecutionPlan")

    if artifacts.get("result") is not None or artifacts.get("resultPath"):
        raise SmokeFailure("ambiguous cadence preflight unexpectedly created a Result artifact")
    if job.get("outputPath") not in (None, ""):
        raise SmokeFailure("ambiguous cadence preflight unexpectedly created a worker output path")

    notes = job.get("notes")
    notes_text = "\n".join(notes) if isinstance(notes, list) else str(notes or "")
    if "Conversion command:" in notes_text or re.search(r"(?i)\bffmpeg\b", notes_text):
        raise SmokeFailure("ambiguous cadence unexpectedly launched a worker FFmpeg conversion")

    if job.get("publishedPath") not in (None, "") or job.get("publishedAt") is not None or job.get("publishedSizeBytes") not in (None, 0):
        raise SmokeFailure("ambiguous cadence preflight unexpectedly published content")
    publication_items = publications.get("items") if isinstance(publications, dict) else None
    if not isinstance(publication_items, list):
        raise SmokeFailure("Publisher did not return its isolated publication list")
    matching_publications = [
        row.get("publication")
        for row in publication_items
        if isinstance(row, dict)
        and isinstance(row.get("publication"), dict)
        and row["publication"].get("queueJobId") == job.get("id")
    ]
    if matching_publications:
        raise SmokeFailure("ambiguous cadence unexpectedly created a Publisher record")

    claim_failure_events = []
    for line in backend_log.splitlines():
        start = line.find("{")
        if start < 0:
            continue
        try:
            event = json.loads(line[start:])
        except json.JSONDecodeError:
            continue
        if (
            isinstance(event, dict)
            and event.get("event") == "auto_worker_claim_failed"
            and event.get("error") == error
        ):
            claim_failure_events.append(event)
    if len(claim_failure_events) != 1:
        raise SmokeFailure("backend logs do not contain the unique structured auto_worker_claim_failed event")

    if job.get("validationStatus") not in (None, "") or job.get("validationReport") not in (None, {}):
        raise SmokeFailure("ambiguous cadence preflight unexpectedly persisted a Validation verdict")
    items = validation_history.get("items") if isinstance(validation_history, dict) else None
    matching_history = [
        row for row in items
        if isinstance(row, dict) and row.get("id") == job.get("id")
    ] if isinstance(items, list) else []
    if len(matching_history) != 1:
        raise SmokeFailure("read-only Validation history did not return exactly the failed synthetic Queue job")
    history_row = matching_history[0]
    if history_row.get("status") != "failed":
        raise SmokeFailure("Validation history does not show the Queue preflight rejection")
    if history_row.get("validationStatus") not in (None, ""):
        raise SmokeFailure("failed ambiguous cadence Queue job has a non-empty Validation status")
    if history_row.get("validationRunId") not in (None, "") or history_row.get("validationOutcome") not in (None, ""):
        raise SmokeFailure("failed ambiguous cadence Queue job unexpectedly has a ValidationRun or outcome")

    return {
        "queueJobId": job.get("id"),
        "queueIntent": {"status": queued.get("status"), "stage": queued.get("stage"), "frozenSnapshot": "absent"},
        "preflight": {
            "status": "PASS: expected negative rejection",
            "queueStatus": job.get("status"),
            "stage": job.get("stage"),
            "cadenceResolution": "unresolved; review required",
            "effectiveOutputFrameRate": "unknown",
            "errorCode": job.get("errorCode"),
            "structuredEvent": "auto_worker_claim_failed",
            "evidence": error,
        },
        "workerExecution": {
            "stageHistory": stages,
            "ffmpegConversion": "NOT_RUN",
            "workerOutputPath": "absent",
        },
        "executionPlan": "NOT_CREATED",
        "result": "NOT_CREATED",
        "outputFfprobe": "NOT_RUN",
        "publication": "NOT_CREATED",
        "validation": {
            "validationRun": "NOT_RUN_BY_DESIGN",
            "validationStatus": job.get("validationStatus"),
            "reason": "ValidationRun requires completed execution and a canonical Result; pre-Queue review is reported by cadence preflight",
        },
        "frameFidelity": "NOT_RUN_BY_DESIGN",
        "overallAcceptance": "PASS: all expected negative preflight assertions passed",
    }


def assert_cadence_validation(validation: dict[str, Any], scenario: SyntheticScenario) -> dict[str, Any] | None:
    if scenario.name == "cadence-ambiguous":
        raise SmokeFailure("ambiguous cadence is rejected before execution; ValidationRun must remain NOT_RUN")
    if scenario.expected_frame_fidelity_status is None:
        return None
    report = validation.get("report")
    fidelity = report.get("frameFidelity") if isinstance(report, dict) else None
    if not isinstance(fidelity, dict):
        raise SmokeFailure("Validation Result has no Frame Fidelity report for cadence scenario")
    if fidelity.get("status") != scenario.expected_frame_fidelity_status:
        raise SmokeFailure(
            f"Frame Fidelity status={fidelity.get('status')!r}, expected {scenario.expected_frame_fidelity_status!r}"
        )
    fields = fidelity.get("fields")
    frame_rate = fields.get("frameRate") if isinstance(fields, dict) else None
    rate_status = frame_rate.get("status") if isinstance(frame_rate, dict) else None
    if scenario.expected_frame_fidelity_rate_status and rate_status != scenario.expected_frame_fidelity_rate_status:
        raise SmokeFailure(
            f"Frame Fidelity frameRate status={rate_status!r}, expected {scenario.expected_frame_fidelity_rate_status!r}"
        )
    return {"status": fidelity.get("status"), "frameRateStatus": rate_status, "expected": fidelity.get("expected"), "actual": fidelity.get("actual")}


def assert_result_validation_binding(
    job: dict[str, Any],
    plan: dict[str, Any],
    result: dict[str, Any],
    validation_run: dict[str, Any],
) -> dict[str, Any]:
    identity = result.get("resultIdentity")
    binding = result.get("executionBinding")
    if not isinstance(identity, dict) or not isinstance(binding, dict):
        raise SmokeFailure("Result is missing canonical identity or frozen execution binding")
    plan_id = job.get("activeExecutionPlanId")
    if not isinstance(plan_id, int) or plan.get("id") != plan_id or plan.get("jobId") != job.get("id"):
        raise SmokeFailure("active ExecutionPlan does not belong to the completed QueueJob")
    expected_execution = job.get("executionNumber")
    if (
        binding.get("executionNumber") != expected_execution
        or binding.get("executionPlanId") != plan_id
        or binding.get("executionPlanVersion") != plan.get("version")
        or not binding.get("executionPlanFingerprint")
        or not binding.get("frozenQueueFingerprint")
    ):
        raise SmokeFailure("Result executionBinding does not identify the active frozen plan and snapshots")
    identity_fields = {
        "resultArtifactId": identity.get("artifactId"),
        "resultGenerationToken": identity.get("generationToken"),
        "resultGenerationVersion": identity.get("generationVersion"),
        "resultOutputRevision": identity.get("outputRevision"),
        "executionNumber": binding.get("executionNumber"),
        "executionPlanId": binding.get("executionPlanId"),
        "executionPlanVersion": binding.get("executionPlanVersion"),
        "executionPlanFingerprint": binding.get("executionPlanFingerprint"),
    }
    for key, value in identity_fields.items():
        if value in (None, ""):
            raise SmokeFailure(f"Result identity/binding is missing {key}")
    expected_validation_fields = {
        "resultArtifactId": identity.get("artifactId"),
        "resultGenerationToken": identity.get("generationToken"),
        "resultGenerationVersion": identity.get("generationVersion"),
        "resultOutputRevision": identity.get("outputRevision"),
        "executionNumber": binding.get("executionNumber"),
        "executionPlanId": binding.get("executionPlanId"),
        "executionPlanVersion": binding.get("executionPlanVersion"),
        "executionPlanFingerprint": binding.get("executionPlanFingerprint"),
    }
    mismatches = {
        key: {"result": expected, "validationRun": validation_run.get(key)}
        for key, expected in expected_validation_fields.items()
        if validation_run.get(key) != expected
    }
    if mismatches:
        raise SmokeFailure(f"ValidationRun is not bound to the current Result and plan: {mismatches}")
    return {
        "resultIdentity": identity_fields,
        "executionBinding": {
            "frozenQueueFingerprint": binding.get("frozenQueueFingerprint"),
        },
    }


def assert_required_findings(findings: Any, scenario: SyntheticScenario | None = None) -> dict[str, str]:
    if not isinstance(findings, list):
        raise SmokeFailure("ValidationRun has no findings list")
    by_key: dict[str, list[dict[str, Any]]] = {}
    for finding in findings:
        if isinstance(finding, dict) and isinstance(finding.get("key"), str):
            by_key.setdefault(finding["key"], []).append(finding)
    states: dict[str, str] = {}
    required_states = dict(REQUIRED_FINDING_STATES)
    if scenario is not None and scenario.subtitle_sidecar_formats:
        required_states["sidecars.package"] = "PASS"
    for key, expected_state in required_states.items():
        matches = by_key.get(key, [])
        if len(matches) != 1:
            raise SmokeFailure(f"ValidationRun has {len(matches)} findings for required key {key!r}")
        state = matches[0].get("state")
        states[key] = str(state)
        if state != expected_state:
            evidence = {
                "state": state,
                "expected": matches[0].get("expected"),
                "actual": matches[0].get("actual"),
                "explanation": matches[0].get("explanation"),
            }
            raise SmokeFailure(f"required ValidationRun finding {key}={state}; expected {expected_state}: {evidence}")
    return states


def validation_finding_state_signature(findings: Any) -> list[tuple[str, str]]:
    if not isinstance(findings, list):
        raise SmokeFailure("Validation response has no findings list")
    signature: list[tuple[str, str]] = []
    seen: set[str] = set()
    for finding in findings:
        if not isinstance(finding, dict):
            raise SmokeFailure("Validation response contains a malformed finding")
        key, state = finding.get("key"), finding.get("state")
        if not isinstance(key, str) or not key or not isinstance(state, str) or not state:
            raise SmokeFailure("Validation response contains a finding without key/state")
        if key in seen:
            raise SmokeFailure(f"Validation response contains duplicate finding {key}")
        seen.add(key)
        signature.append((key, state))
    return sorted(signature)


def assert_publisher_binding(
    response: Any,
    job_id: int,
    execution_number: Any,
    result_artifact_id: str,
    validation_run_id: str,
) -> dict[str, Any]:
    items = response.get("items") if isinstance(response, dict) else None
    if not isinstance(items, list):
        raise SmokeFailure("Publisher did not return its publication list")
    publications = [
        row.get("publication")
        for row in items
        if isinstance(row, dict)
        and isinstance(row.get("publication"), dict)
        and row["publication"].get("queueJobId") == job_id
    ]
    if len(publications) != 1:
        raise SmokeFailure(f"expected one Publisher record for QueueJob {job_id}, found {len(publications)}")
    publication = publications[0]
    expected = {
        "executionNumber": execution_number,
        "resultArtifactId": result_artifact_id,
        "validationRunId": validation_run_id,
    }
    mismatches = {
        key: {"expected": value, "actual": publication.get(key)}
        for key, value in expected.items()
        if publication.get(key) != value
    }
    if str(publication.get("state", "")).lower() != "published":
        mismatches["state"] = {"expected": "published", "actual": publication.get("state")}
    if mismatches:
        raise SmokeFailure(f"Publisher record is not bound to the validated Result generation: {mismatches}")
    return {
        key: publication.get(key)
        for key in (
            "publicationId", "queueJobId", "executionNumber", "resultArtifactId",
            "validationRunId", "state", "destinationPath", "originalArchiveStatus",
        )
    }


def assert_publisher_blocked(
    publications: Any,
    job_id: int,
    execution_number: int,
    artifact_id: str,
    validation_run_id: str,
) -> dict[str, Any]:
    items = publications.get("items") if isinstance(publications, dict) else None
    if not isinstance(items, list):
        raise SmokeFailure("Publisher did not return its publication list after the incomplete-package request")
    matching = [
        row.get("publication")
        for row in items
        if isinstance(row, dict)
        and isinstance(row.get("publication"), dict)
        and row["publication"].get("queueJobId") == job_id
        and row["publication"].get("executionNumber") == execution_number
    ]
    if len(matching) != 1:
        raise SmokeFailure(f"expected one Publisher record for QueueJob {job_id}, found {len(matching)}")
    publication = matching[0]
    expected = {"resultArtifactId": artifact_id, "validationRunId": validation_run_id}
    mismatches = {
        key: {"expected": value, "actual": publication.get(key)}
        for key, value in expected.items()
        if publication.get(key) != value
    }
    if str(publication.get("state", "")).lower() != "blocked":
        mismatches["state"] = {"expected": "blocked", "actual": publication.get("state")}
    failure = str(publication.get("failureSummary") or "")
    if "sidecar" not in failure.lower():
        mismatches["failureSummary"] = {"expected": "required sidecar failure", "actual": failure}
    if mismatches:
        raise SmokeFailure(f"incomplete package did not block the bound Publisher record: {mismatches}")
    return {
        "publicationId": publication.get("publicationId"),
        "state": publication.get("state"),
        "failureSummary": failure,
    }


def assert_manual_publish_accepted(response: Any) -> tuple[str, str]:
    if not isinstance(response, dict) or response.get("status") != "accepted":
        raise SmokeFailure("manual Publish did not return a durable accepted request")
    publication_id = response.get("publicationId")
    request_id = response.get("requestId")
    if not isinstance(publication_id, str) or not publication_id.strip():
        raise SmokeFailure("accepted manual Publish response is missing its publication identity")
    if not isinstance(request_id, str) or not request_id.strip():
        raise SmokeFailure("accepted manual Publish response is missing its request identity")
    return publication_id, request_id


def manual_publication_attempt_state(publication: dict[str, Any], request_id: str) -> str:
    """Ignore a prior blocked attempt until this accepted request becomes current."""
    if publication.get("manualRequestId") != request_id:
        return "pending"
    state = str(publication.get("state", "")).lower()
    request_state = str(publication.get("manualRequestStatus", "")).lower()
    if state == "published":
        return "published"
    if request_state == "failed" or (
        state in {"failed", "blocked"} and request_state not in {"accepted", "processing"}
    ):
        return "failed"
    return "pending"


def extract_plan_summary(plans: Any) -> dict[str, Any]:
    if not isinstance(plans, list) or not plans:
        raise SmokeFailure("the completed QueueJob has no persisted ExecutionPlan")
    summary = []
    for plan in plans:
        if not isinstance(plan, dict):
            continue
        summary.append({
            key: plan.get(key)
            for key in (
                "id", "jobId", "version", "status", "profileVersion", "libraryId",
                "destinationRoot", "destinationPath", "publishMode", "codecFamily",
                "selectedEncoder", "bitDepth", "pixelFormat", "qualityMode", "qualityValue",
                "runtimeProfile", "workspaceMode", "waitingState", "approvalMode", "approvalStatus",
                "decisionReasons", "warnings", "outputPath",
            )
        })
    if not summary:
        raise SmokeFailure("the ExecutionPlan response contains no plan objects")
    return {"count": len(summary), "plans": summary}


def prepare_sandbox(sandbox: Path) -> None:
    for relative in (
        "raw", "library/published", "staging", "archive", "reports", "config",
        "fixtures", "tmp", "cache/xdg", "ocr/lifecycle", "ocr/persistent",
        "config/home", "config/xdg", "config/data", "config/state",
    ):
        (sandbox / relative).mkdir(parents=True, exist_ok=True)


def run_smoke(args: argparse.Namespace) -> int:
    scenario = scenario_for(args.scenario)
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", args.image):
        raise SmokeFailure("image must be a simple Docker image reference")

    temp_base = args.temp_base or os.environ.get("RUNNER_TEMP") or tempfile.gettempdir()
    root = create_run_root(temp_base, args.run_id)
    sandbox = root / "sandbox"
    evidence = Evidence(root / "evidence")
    prepare_sandbox(sandbox)
    container = f"mvforge-e2e-{uuid.uuid4().hex[:16]}"
    passed = False
    cadence_summary: dict[str, Any] | None = None
    cadence_validation_summary: dict[str, Any] | None = None
    source_path = sandbox / "raw" / "synthetic-smoke.mkv"
    evidence.write("scenario.log", f"scenario={scenario.name}\nphase=starting\n".encode(), MAX_LOG_BYTES)

    def log(message: str) -> None:
        print(message, flush=True)
        current = (evidence.directory / "scenario.log").read_bytes() if (evidence.directory / "scenario.log").exists() else b""
        evidence.write("scenario.log", current + (message + "\n").encode(), MAX_LOG_BYTES)

    try:
        run_command(["docker", "info"], timeout=30)
        ensure_image(args.image, build_if_missing=not args.no_build)
        log("phase=generate-synthetic-fixture")
        run_command(
            [
                "bash", str(REPO_ROOT / "scripts/e2e/generate-fixtures.sh"),
                "--image", args.image, "--sandbox", str(sandbox), "--scenario", scenario.name,
            ],
            timeout=120,
        )
        if not source_path.is_file() or source_path.stat().st_size == 0:
            raise SmokeFailure("FFmpeg did not create the synthetic fixture")
        source_hash_before = sha256_file(source_path)

        log("phase=start-isolated-backend")
        run_command(runtime_container_args(container, args.image, sandbox), timeout=60)
        inspect_result = run_command(["docker", "inspect", container], timeout=30)
        inspect_data = json.loads(inspect_result.stdout)[0]
        validate_container_inspect(inspect_data, sandbox)
        evidence.json("container-isolation.json", {
            "networkMode": inspect_data["HostConfig"].get("NetworkMode"),
            "readonlyRootfs": inspect_data["HostConfig"].get("ReadonlyRootfs"),
            "mounts": [
                {"type": item.get("Type"), "source": "<isolated-run-root>", "destination": item.get("Destination"), "rw": item.get("RW")}
                for item in inspect_data.get("Mounts", [])
            ],
            "publishedPorts": inspect_data["HostConfig"].get("PortBindings"),
            "deviceRequests": inspect_data["HostConfig"].get("DeviceRequests"),
        })

        api = API(container, evidence)
        deadline = time.monotonic() + 90
        while True:
            try:
                api.request("GET", "/health", evidence_name="health.json")
                break
            except SmokeFailure:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(2)

        fixture_probe = probe_media(container, "/media/raw/synthetic-smoke.mkv")
        evidence.json("ffprobe-summary.json", {"fixture": summarize_probe(fixture_probe)})
        fixture_summary = assert_media_probe(
            fixture_probe, scenario, "synthetic fixture", expected_video_codec=scenario.source_video_codec,
        )
        evidence.json("ffprobe-summary.json", {
            "fixture": summarize_probe(fixture_probe),
            "fixtureSummary": fixture_summary,
        })

        log("phase=create-isolated-profile-and-library")
        profile = api.request(
            "POST", "/api/profiles", expected=(201,), evidence_name="profile.json",
            payload={
                "name": "E2E synthetic libx265",
                "scope": "global",
                "description": "Generated by the isolated pipeline smoke harness",
                "container": "mkv",
                "videoCodec": "x265",
                "codecFamily": "hevc",
                "preferredEncoder": "libx265",
                "allowedEncoders": ["libx265"],
                "bitDepth": 8,
                "pixelFormat": "yuv420p",
                "audioCodec": "copy",
                "qualityMode": "crf",
                "qualityValue": 28,
                "preserveHdr": False,
                "preserveSubtitles": True,
                "preserveChapters": True,
                "workerConfig": {
                    "videoEncoder": "libx265",
                    "cadenceMode": scenario.cadence_mode,
                    "fieldStructureMode": scenario.field_structure_mode,
                    "cadenceFieldOrder": "auto",
                },
            },
        )
        profile_id = profile.get("id") if isinstance(profile, dict) else None
        if not isinstance(profile_id, int):
            raise SmokeFailure("profile creation did not return an integer id")

        library = api.request(
            "POST", "/api/libraries", expected=(201,), evidence_name="library.json",
            payload={
                "name": "E2E synthetic Library",
                "sourcePath": "/media/raw",
                "destinationPath": "/media/library/published",
                "type": "movie",
                "validationRules": {},
            },
        )
        library_id = library.get("id") if isinstance(library, dict) else None
        if not isinstance(library_id, int):
            raise SmokeFailure("library creation did not return an integer id")

        track_profile_key = "pipeline-smoke-tracks"
        track_profile = track_profile_for_scenario(track_profile_key, scenario)
        api.request(
            "POST", "/api/settings/trackProfiles", expected=(200,), evidence_name="track-profile-setting.json",
            payload={
                "value": {
                    "profiles": [track_profile],
                },
            },
        )
        api.request(
            "POST", "/api/settings/originalRetentionPolicy", expected=(200,),
            evidence_name="original-retention-setting.json",
            payload={
                "value": {
                    "keepOriginalsDays": 30,
                    "autoDeleteEnabled": False,
                    "processedOriginalsPath": "/media/archive",
                },
            },
        )
        api.request(
            "POST", "/api/settings/pipelineAutomation", expected=(200,),
            evidence_name="pipeline-automation-setting.json",
            payload={"value": PIPELINE_AUTOMATION_SETTINGS},
        )
        api.request(
            "POST", "/api/settings/runtimePolicy", expected=(200,),
            evidence_name="runtime-policy-setting.json",
            payload={"value": SYNTHETIC_RUNTIME_POLICY},
        )
        api.request(
            "POST", "/api/settings/workspace", expected=(200,),
            evidence_name="workspace-setting.json",
            payload={"value": SYNTHETIC_WORKSPACE_SETTINGS},
        )

        log("phase=queue-and-wait-for-real-worker")
        queued = api.request(
            "POST", "/api/queue/jobs", expected=(201,), evidence_name="queue-created.json",
            payload={
                "mediaPath": "/media/raw/synthetic-smoke.mkv",
                "libraryId": library_id,
                "profileId": profile_id,
                "trackProfileKey": track_profile_key,
                "processingMode": "full_encode",
                "notes": "Synthetic isolated pipeline smoke; no production media",
            },
        )
        job_id = queued.get("id") if isinstance(queued, dict) else None
        if not isinstance(job_id, int):
            raise SmokeFailure("Queue creation did not return an integer job id")

        terminal = {"completed", "failed", "canceled"}
        deadline = time.monotonic() + args.timeout
        job: dict[str, Any] = {}
        while time.monotonic() < deadline:
            jobs = api.request("GET", "/api/queue/jobs")
            job = next((row for row in jobs if isinstance(row, dict) and row.get("id") == job_id), {})
            if job:
                evidence.json("queue-job.json", job)
                log(f"job_status={job.get('status')} job_stage={job.get('stage')}")
                if job.get("status") in terminal:
                    break
            time.sleep(2)
        else:
            plan_detail = "unavailable"
            try:
                plans = api.request(
                    "GET", f"/api/queue/jobs/{job_id}/execution-plans",
                    evidence_name="execution-plans-at-timeout.json",
                )
                plan_summary = extract_plan_summary(plans)
                plan_detail = json.dumps(plan_summary, sort_keys=True)[:3000]
            except Exception as plan_error:
                evidence.json("execution-plans-at-timeout-error.json", {"error": str(plan_error)[:2000]})
                plan_detail = f"plan query failed: {type(plan_error).__name__}: {plan_error}"
            raise SmokeFailure(
                f"QueueJob {job_id} did not finish within {args.timeout} seconds; execution plans: {plan_detail}"
            )
        if scenario.name == "cadence-ambiguous":
            if job.get("status") != "failed":
                raise SmokeFailure(
                    "ambiguous cadence did not receive the expected preflight rejection; "
                    f"QueueJob ended as {job.get('status')}/{job.get('stage')}: {job.get('errorMessage', '')}"
                )
            log("phase=claim-time-cadence-preflight")
            plans = api.request(
                "GET", f"/api/queue/jobs/{job_id}/execution-plans",
                evidence_name="execution-plans.json",
            )
            artifacts = api.request(
                "GET", f"/api/queue/jobs/{job_id}/artifacts",
                evidence_name="result-artifact-state.json",
            )
            validation_history = api.request(
                "GET", f"/api/validation/history?q={job_id}&limit=100",
                evidence_name="validation-history.json",
            )
            publications = api.request(
                "GET", "/api/publisher/publications",
                evidence_name="publisher-publications.json",
            )
            logs = run_command(["docker", "logs", "--tail", "1000", container], timeout=30, check=False)
            if logs.returncode != 0:
                raise SmokeFailure(
                    f"could not read backend logs for claim-time rejection: {str(logs.stderr or '')[-2000:]}"
                )
            backend_log = logs.stdout
            evidence.write("backend-container.log", backend_log.encode("utf-8", "replace"), MAX_LOG_BYTES)
            fail_closed_summary = assert_ambiguous_cadence_preflight(
                queued, job, plans, artifacts, validation_history, publications, backend_log,
            )
            evidence.json("cadence-preflight-summary.json", fail_closed_summary)
            log("component=claim-time-cadence-preflight outcome=PASS result=expected-negative")
            log("component=ExecutionPlan outcome=PASS result=NOT_CREATED")
            log("component=worker-FFmpeg-conversion outcome=PASS result=NOT_RUN")
            log("component=Result-and-output-ffprobe outcome=PASS result=NOT_CREATED")
            log("component=Publisher outcome=PASS result=NOT_CREATED")
            log("component=ValidationRun outcome=PASS result=NOT_RUN_BY_DESIGN")
            log("phase=passed")
            passed = True
            print(
                "CADENCE_SMOKE_SUMMARY "
                + json.dumps(fail_closed_summary, sort_keys=True, separators=(",", ":")),
                flush=True,
            )
            print("PIPELINE_SMOKE_PASSED", flush=True)
            return 0
        if job.get("status") != "completed":
            raise SmokeFailure(f"QueueJob {job_id} ended as {job.get('status')}: {job.get('errorMessage', '')}")

        evidence.json("track-snapshot.json", job.get("trackProfileSnapshot"))
        frozen_snapshot_summary = assert_frozen_snapshots(job, scenario)
        plan_id = job.get("activeExecutionPlanId")
        if not isinstance(plan_id, int):
            raise SmokeFailure("completed QueueJob is missing its active ExecutionPlan identity")

        plans = api.request("GET", f"/api/queue/jobs/{job_id}/execution-plans")
        plan_summary = extract_plan_summary(plans)
        evidence.json("execution-plans.json", plan_summary)
        active_plans = [
            row for row in plans
            if isinstance(row, dict) and row.get("id") == plan_id and row.get("jobId") == job_id
        ] if isinstance(plans, list) else []
        if len(active_plans) != 1:
            raise SmokeFailure("active ExecutionPlan is not uniquely present for this QueueJob")
        active_plan = active_plans[0]
        if active_plan.get("selectedEncoder") != "libx265":
            raise SmokeFailure("ExecutionPlan did not select the required portable libx265 encoder")
        cadence_summary = assert_cadence_execution(job, active_plan, scenario)
        if cadence_summary is not None:
            evidence.json("cadence-execution-summary.json", cadence_summary)
        evidence.json("frozen-plan-summary.json", {
            "queueJobId": job_id,
            "profileCapturedAt": job.get("profileCapturedAt"),
            "frozenSnapshots": frozen_snapshot_summary,
            "activeExecutionPlanId": plan_id,
            **plan_summary,
        })

        log("phase=result-artifact")
        artifacts = api.request(
            "GET", f"/api/queue/jobs/{job_id}/artifacts",
        )
        if not isinstance(artifacts, dict) or not isinstance(artifacts.get("result"), dict):
            raise SmokeFailure("completed QueueJob has no Result artifact")
        result_artifact = artifacts["result"]
        evidence.json("result-artifact-manifest.json", result_artifact.get("artifacts"))
        primary_output = (result_artifact.get("artifacts") or {}).get("primary")
        if not isinstance(primary_output, dict) or primary_output.get("status") != "ready":
            raise SmokeFailure("Result artifact has no ready primary media output")
        result_path = container_media_path(primary_output.get("path"), "Result primary media path")
        evidence.json("result-artifact.json", result_artifact_evidence(result_artifact, artifacts.get("resultPath", "")))
        run_command(["docker", "exec", container, "/usr/bin/test", "-s", result_path], timeout=15)
        result_hash_before_publish = sha256_file(host_path_for_media(sandbox, result_path, "Result artifact path"))
        result_probe = result_artifact.get("outputProbe")
        result_path_probe = probe_media(container, result_path)
        evidence.json("ffprobe-summary.json", {
            "fixture": summarize_probe(fixture_probe),
            "resultArtifact": summarize_probe(result_probe),
            "resultFile": summarize_probe(result_path_probe),
        })
        manifest_summary = assert_result_artifact_manifest(result_artifact, job, scenario, sandbox)
        evidence.json("result-artifact-manifest-summary.json", manifest_summary)
        result_summary = assert_media_probe(
            result_probe, scenario, "Result artifact", expected_video_codec=scenario.output_video_codec,
            output=True, require_frame_count=False,
        )
        result_path_summary = assert_media_probe(
            result_path_probe, scenario, "Result file", expected_video_codec=scenario.output_video_codec,
            output=True,
        )
        evidence.json("ffprobe-summary.json", {
            "fixture": summarize_probe(fixture_probe),
            "resultArtifact": summarize_probe(result_probe),
            "resultFile": summarize_probe(result_path_probe),
            "fixtureSummary": fixture_summary,
            "resultSummary": result_summary,
            "resultFileSummary": result_path_summary,
        })

        log("phase=validation")
        validation = api.request(
            "POST", f"/api/validation/jobs/{job_id}", payload={}, expected=(200,),
            evidence_name="validation-result.json",
        )
        cadence_validation_summary = assert_cadence_validation(validation, scenario)
        if cadence_validation_summary is not None:
            evidence.json("cadence-validation-summary.json", cadence_validation_summary)
        run_id = validation.get("runId") if isinstance(validation, dict) else None
        if not isinstance(run_id, str) or not run_id:
            raise SmokeFailure("Validation did not persist and return a ValidationRun id")
        validation_run = api.request(
            "GET", f"/api/validation/jobs/{job_id}/runs/{run_id}",
        )
        if not isinstance(validation_run, dict):
            raise SmokeFailure("ValidationRun detail response was not an object")
        evidence.json("validation-run.json", validation_run_evidence(validation_run))
        persisted_run = validation_run.get("run")
        if not isinstance(persisted_run, dict) or persisted_run.get("runId") != run_id:
            raise SmokeFailure("ValidationRun detail does not contain the requested immutable run")
        if validation.get("outcome") != "PASS" or persisted_run.get("outcome") != "PASS":
            raise SmokeFailure(
                f"Validation outcome is not PASS: response={validation.get('outcome')!r}, "
                f"persisted={persisted_run.get('outcome')!r}"
            )
        if validation_finding_state_signature(validation.get("findings")) != validation_finding_state_signature(
            persisted_run.get("findings")
        ):
            raise SmokeFailure("Validation response finding states differ from the persisted ValidationRun")
        finding_states = assert_required_findings(persisted_run.get("findings"), scenario)
        evidence.json("required-validation-findings.json", finding_states)
        binding_summary = assert_result_validation_binding(job, active_plan, result_artifact, persisted_run)
        if validation_run.get("effectiveHandoff") != "ALLOWED":
            raise SmokeFailure(
                f"ValidationRun handoff is {validation_run.get('effectiveHandoff')!r}, expected ALLOWED"
            )
        if not source_path.is_file() or sha256_file(source_path) != source_hash_before:
            raise SmokeFailure("Worker or Validation changed the synthetic source before publication")

        incomplete_package_summary = None
        if scenario.subtitle_sidecar_formats:
            log("phase=publisher-reject-incomplete-package")
            ass_sidecar = next(
                item for item in manifest_summary["sidecars"] if item.get("format") == "ass"
            )
            ass_path = host_path_for_media(sandbox, ass_sidecar["stagedPath"], "ASS Result sidecar path")
            ass_content = ass_path.read_bytes()
            incomplete_package_summary = None
            blocked_publication_id = None
            try:
                ass_path.unlink()
                incomplete_request = api.request(
                    "POST", f"/api/publisher/jobs/{job_id}/publish", expected=(202,),
                    payload={"reason": "Synthetic isolated smoke incomplete-package negative case"},
                    evidence_name="incomplete-package-request-accepted.json",
                )
                blocked_publication_id, _ = assert_manual_publish_accepted(incomplete_request)
                blocked_deadline = time.monotonic() + 120
                last_publisher_state = None
                while time.monotonic() < blocked_deadline:
                    publications = api.request("GET", "/api/publisher/publications")
                    items = publications.get("items") if isinstance(publications, dict) else None
                    matching = [
                        row.get("publication")
                        for row in items or []
                        if isinstance(row, dict)
                        and isinstance(row.get("publication"), dict)
                        and row["publication"].get("publicationId") == blocked_publication_id
                    ]
                    if len(matching) > 1:
                        raise SmokeFailure(
                            f"Publisher returned duplicate records for incomplete package {blocked_publication_id}"
                        )
                    if matching:
                        last_publisher_state = matching[0].get("state")
                        if str(last_publisher_state).lower() == "blocked":
                            evidence.json("publisher-incomplete-package-record.json", publications)
                            incomplete_package_summary = assert_publisher_blocked(
                                publications,
                                job_id,
                                result_artifact["executionBinding"].get("executionNumber"),
                                result_artifact["resultIdentity"].get("artifactId"),
                                run_id,
                            )
                            break
                    time.sleep(1)
                if incomplete_package_summary is None:
                    raise SmokeFailure(
                        "Publisher did not block the incomplete package after its accepted request; "
                        f"last state={last_publisher_state!r}"
                    )
                if incomplete_package_summary.get("publicationId") != blocked_publication_id:
                    raise SmokeFailure("Publisher changed the incomplete-package publication identity")
                evidence.json("publisher-incomplete-package-summary.json", incomplete_package_summary)
            finally:
                ass_path.write_bytes(ass_content)
            if not isinstance(blocked_publication_id, str) or not blocked_publication_id:
                raise SmokeFailure("blocked Publisher record has no publication identity for audited retry")
            log("phase=restore-package-and-retry-publication")
            accepted = api.request(
                "POST", f"/api/publisher/publications/{blocked_publication_id}/actions", expected=(202,),
                payload={
                    "action": "retry",
                    "reason": "Synthetic sidecar restored after incomplete-package negative case",
                },
                evidence_name="publish-retry-accepted.json",
            )
            expected_publication_id, expected_request_id = assert_manual_publish_accepted(accepted)
            if expected_publication_id != blocked_publication_id:
                raise SmokeFailure("Publisher retry changed the blocked publication identity")
        else:
            log("phase=publish-to-isolated-library")
            accepted = api.request(
                "POST", f"/api/publisher/jobs/{job_id}/publish", expected=(202,),
                payload={"reason": "Synthetic isolated pipeline smoke"},
                evidence_name="publish-accepted.json",
            )
            expected_publication_id, expected_request_id = assert_manual_publish_accepted(accepted)
        publish_deadline = time.monotonic() + 120
        publications: Any = None
        while time.monotonic() < publish_deadline:
            publications = api.request("GET", "/api/publisher/publications")
            rows = publications.get("items") if isinstance(publications, dict) else None
            matching = [
                row.get("publication")
                for row in rows or []
                if isinstance(row, dict)
                and isinstance(row.get("publication"), dict)
                and row["publication"].get("publicationId") == expected_publication_id
            ]
            if matching:
                publication = matching[0]
                attempt_state = manual_publication_attempt_state(publication, expected_request_id)
                if attempt_state == "published":
                    break
                if attempt_state == "failed":
                    state = str(publication.get("state", "")).lower()
                    reason = publication.get("failureSummary") or publication.get("holdReason") or state
                    raise SmokeFailure(f"accepted Publisher request failed before delivery: {reason}")
            time.sleep(1)
        else:
            raise SmokeFailure("durably accepted Publisher request did not reach Published within 120 seconds")
        evidence.json("publisher-publications.json", publications)
        published_path = container_media_path(
            publication.get("destinationPath"),
            "published Library path",
        )
        if not published_path.startswith("/media/library/published/"):
            raise SmokeFailure("Publisher wrote outside the synthetic Library destination")
        run_command(["docker", "exec", container, "/usr/bin/test", "-s", published_path], timeout=15)
        published_hash = sha256_file(host_path_for_media(sandbox, published_path, "published Library path"))
        if published_hash != result_hash_before_publish:
            raise SmokeFailure("published package bytes differ from the Result validated by this ValidationRun")
        published_probe = probe_media(container, published_path)
        published_summary = assert_media_probe(
            published_probe, scenario, "published package", expected_video_codec=scenario.output_video_codec,
            output=True,
        )
        evidence.json("ffprobe-summary.json", {
            "fixture": fixture_probe,
            "resultArtifact": summarize_probe(result_probe),
            "resultFile": summarize_probe(result_path_probe),
            "published": summarize_probe(published_probe),
            "fixtureSummary": fixture_summary,
            "resultSummary": result_summary,
            "resultFileSummary": result_path_summary,
            "publishedSummary": published_summary,
        })
        publication_summary = assert_publisher_binding(
            publications,
            job_id,
            result_artifact["executionBinding"].get("executionNumber"),
            result_artifact["resultIdentity"].get("artifactId"),
            run_id,
        )
        if publication_summary.get("destinationPath") != published_path:
            raise SmokeFailure("Publisher record destination differs from the delivered package path")
        published_job_list = api.request("GET", "/api/queue/jobs", evidence_name="queue-job-after-publish.json")
        published_job = next(
            (row for row in published_job_list if isinstance(row, dict) and row.get("id") == job_id),
            {},
        )
        if published_job.get("publicationId") != publication_summary.get("publicationId"):
            raise SmokeFailure("QueueJob does not reference the Publisher record that delivered this package")
        if str(published_job.get("publicationState", "")).lower() != "published":
            raise SmokeFailure(f"QueueJob publication state is {published_job.get('publicationState')!r}")
        if published_job.get("publishedPath") != published_path:
            raise SmokeFailure("QueueJob published path differs from the delivered package path")
        published_sidecars = assert_published_sidecars(
            published_job, manifest_summary, scenario, sandbox, published_path,
        )
        evidence.json("published-sidecars.json", published_sidecars)
        archived_path = container_media_path(
            published_job.get("originalArchivedPath"), "archived original path",
        )
        if not archived_path.startswith("/media/archive/"):
            raise SmokeFailure("Publisher archived the original outside the isolated Archive")
        archived_host_path = (sandbox / PurePosixPath(archived_path).relative_to("/media")).resolve(strict=True)
        if not is_within(archived_host_path, sandbox.resolve()):
            raise SmokeFailure("archived original path escaped the temporary sandbox")
        if sha256_file(archived_host_path) != source_hash_before:
            raise SmokeFailure("archived original differs from the generated source fixture")
        if source_path.exists():
            raise SmokeFailure("Publisher did not move the original into the configured isolated Archive")

        evidence.json("smoke-summary.json", {
            "scenario": args.scenario,
            "queueJobId": job_id,
            "executionPlanId": plan_id,
            "validationRunId": run_id,
            "resultArtifactId": result_artifact["resultIdentity"].get("artifactId"),
            "resultGenerationToken": result_artifact["resultIdentity"].get("generationToken"),
            "resultOutputRevision": result_artifact["resultIdentity"].get("outputRevision"),
            "requiredFindings": finding_states,
            "publisher": publication_summary,
            "resultPath": result_path,
            "publishedPath": published_path,
            "resultAndPublishedPackageSha256": result_hash_before_publish,
            "resultRequiredArtifactIds": manifest_summary["requiredArtifactIds"],
            "publishedSidecars": published_sidecars,
            "streamCounts": published_summary["streamCounts"],
            "resultChapterCount": result_summary["chapterCount"],
            "resultChapterTitles": result_summary["chapterTitles"],
            "chapterCount": published_summary["chapterCount"],
            "incompletePackagePublisherBlock": incomplete_package_summary,
            "originalArchivedPath": archived_path,
            "originalHashPreserved": True,
            "networkMode": "none",
            "rootFilesystemReadOnly": True,
        })
        if scenario.name.startswith("cadence-"):
            print(
                "CADENCE_SMOKE_SUMMARY "
                + json.dumps({
                    "scenario": scenario.name,
                    "queueJobId": job_id,
                    "executionPlanId": plan_id,
                    "cadence": cadence_summary,
                    "result": result_path_summary,
                    "frameFidelity": cadence_validation_summary,
                }, sort_keys=True, separators=(",", ":")),
                flush=True,
            )
        log("phase=passed")
        passed = True
        print("PIPELINE_SMOKE_PASSED", flush=True)
        return 0
    except Exception as exc:
        log(f"failure={type(exc).__name__}: {exc}")
        evidence.json("failure.json", {"scenario": args.scenario, "errorType": type(exc).__name__, "error": str(exc)[:12000]})
        try:
            logs = run_command(["docker", "logs", "--tail", "1000", container], timeout=30, check=False)
            evidence.write("backend-container.log", logs.stdout.encode("utf-8", "replace"), MAX_LOG_BYTES)
        except Exception as log_error:
            evidence.json("backend-container-log-error.json", {"error": str(log_error)[:2000]})
        print(f"PIPELINE_SMOKE_FAILED; bounded evidence: {evidence.directory}", file=sys.stderr, flush=True)
        raise
    finally:
        run_command(["docker", "rm", "--force", container], timeout=30, check=False)
        if sandbox.exists():
            shutil.rmtree(sandbox)
        if passed:
            shutil.rmtree(root)


def generate_only(args: argparse.Namespace) -> int:
    scenario = scenario_for(args.scenario)
    sandbox = Path(args.sandbox).expanduser().resolve(strict=True)
    run_root = sandbox.parent
    base = validate_temp_base(run_root.parent)
    if run_root.parent != base or not run_root.name.startswith(RUN_PREFIX) or sandbox.name != "sandbox" or not sandbox.is_dir():
        raise SmokeFailure("fixture generation requires the sandbox directory under a run root")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", args.image):
        raise SmokeFailure("image must be a simple Docker image reference")
    docker_mount(sandbox)
    generate_fixture(args.image, sandbox, scenario)
    print(f"Generated deterministic {scenario.name} fixture in {sandbox}")
    return 0


def generate_cadence_fixture_only(args: argparse.Namespace) -> int:
    sandbox = Path(args.sandbox).expanduser().resolve(strict=True)
    run_root = sandbox.parent
    base = validate_temp_base(run_root.parent)
    if run_root.parent != base or not run_root.name.startswith(RUN_PREFIX) or sandbox.name != "sandbox" or not sandbox.is_dir():
        raise SmokeFailure("fixture generation requires the sandbox directory under a run root")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", args.image):
        raise SmokeFailure("image must be a simple Docker image reference")
    docker_mount(sandbox)
    generate_fixture(
        args.image, sandbox, HAPPY_PATH_SCENARIO,
        video_filter="telecine=first_field=top", interlaced=True,
    )
    print(f"Generated deterministic hard-telecine analysis fixture in {sandbox}")
    return 0


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)

    run = commands.add_parser("run", help="run one isolated end-to-end scenario")
    run.add_argument("--scenario", default="happy-path")
    run.add_argument("--image", default=DEFAULT_IMAGE)
    run.add_argument("--run-id")
    run.add_argument("--temp-base")
    run.add_argument("--timeout", type=int, default=300)
    run.add_argument("--no-build", action="store_true")

    generate = commands.add_parser("generate", help="generate the synthetic fixture only")
    generate.add_argument("--scenario", default="happy-path")
    generate.add_argument("--image", default=DEFAULT_IMAGE)
    generate.add_argument("--sandbox", required=True)

    cadence = commands.add_parser("generate-cadence", help="generate the multistream hard-telecine analysis fixture only")
    cadence.add_argument("--image", default=DEFAULT_IMAGE)
    cadence.add_argument("--sandbox", required=True)
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv if argv is not None else sys.argv[1:])
    try:
        if args.command == "run":
            return run_smoke(args)
        if args.command == "generate-cadence":
            return generate_cadence_fixture_only(args)
        return generate_only(args)
    except Exception as exc:
        print(f"pipeline smoke error: {exc}", file=sys.stderr, flush=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
