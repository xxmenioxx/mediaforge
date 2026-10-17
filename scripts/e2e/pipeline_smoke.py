#!/usr/bin/env python3
"""Run an isolated synthetic MVForge pipeline smoke scenario."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path, PurePosixPath
from typing import Any, Sequence


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_IMAGE = "mvforge-backend:pipeline-smoke"
RUN_PREFIX = "mvforge-e2e-"
MAX_TOTAL_EVIDENCE_BYTES = 1024 * 1024
MAX_JSON_BYTES = 128 * 1024
MAX_LOG_BYTES = 256 * 1024
SUPPORTED_SCENARIOS = {"happy-path"}
FORBIDDEN_HOST_ROOTS = (Path("/volume1"), Path("/volume2"))
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


def generate_fixture(image: str, sandbox: Path) -> None:
    args = [
        "docker", "run", "--rm", "--network", "none", "--read-only",
        "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
        "--user", host_user(),
        "--mount", docker_mount(sandbox),
        "--env", "HOME=/media/config/home", "--env", "TMPDIR=/media/tmp",
        "--entrypoint", "ffmpeg", image,
        "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
        "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12:duration=8",
        "-map", "0:v:0", "-an",
        "-map_metadata", "-1", "-c:v", "libx264", "-preset", "ultrafast",
        "-threads", "2", "-pix_fmt", "yuv420p",
        "-metadata", "title=MVForge synthetic pipeline smoke",
        "/media/raw/synthetic-smoke.mkv",
    ]
    run_command(args, timeout=90)


def probe_media(container: str, media_path: str) -> dict[str, Any]:
    result = run_command(
        [
            "docker", "exec", container, "ffprobe", "-v", "error",
            "-show_entries", "format=duration:stream=codec_type,codec_name,width,height,sample_rate,channels",
            "-of", "json", media_path,
        ],
        timeout=30,
    )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise SmokeFailure(f"ffprobe returned invalid JSON for {media_path}") from exc


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


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


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
                "id", "jobId", "version", "status", "profileVersion", "codecFamily",
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
    if args.scenario not in SUPPORTED_SCENARIOS:
        raise SmokeFailure(f"unsupported scenario {args.scenario!r}; available: {sorted(SUPPORTED_SCENARIOS)}")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", args.image):
        raise SmokeFailure("image must be a simple Docker image reference")

    temp_base = args.temp_base or os.environ.get("RUNNER_TEMP") or tempfile.gettempdir()
    root = create_run_root(temp_base, args.run_id)
    sandbox = root / "sandbox"
    evidence = Evidence(root / "evidence")
    prepare_sandbox(sandbox)
    container = f"mvforge-e2e-{uuid.uuid4().hex[:16]}"
    passed = False
    source_path = sandbox / "raw" / "synthetic-smoke.mkv"
    evidence.write("scenario.log", b"scenario=happy-path\nphase=starting\n", MAX_LOG_BYTES)

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
                "--image", args.image, "--sandbox", str(sandbox),
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
        evidence.json("ffprobe-summary.json", {"fixture": fixture_probe})
        streams = fixture_probe.get("streams", [])
        if not any(stream.get("codec_type") == "video" for stream in streams):
            raise SmokeFailure("synthetic fixture has no video stream")

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
                "preserveSubtitles": False,
                "preserveChapters": False,
                "workerConfig": {
                    "videoEncoder": "libx265",
                    "cadenceMode": "preserve",
                    "fieldStructureMode": "preserve",
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
        api.request(
            "POST", "/api/settings/trackProfiles", expected=(200,), evidence_name="track-profile-setting.json",
            payload={
                "value": {
                    "profiles": [{
                        "key": track_profile_key,
                        "scope": "asset",
                        "trackDispositionVersion": 1,
                        "keepVideoStreams": [0],
                        "keepAudioStreams": [],
                        "keepSubtitleStreams": [],
                        "subtitleDisposition": "remove",
                        "attachmentPolicy": "remove",
                        "chapterPolicy": "remove",
                    }],
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
        if job.get("status") != "completed":
            raise SmokeFailure(f"QueueJob {job_id} ended as {job.get('status')}: {job.get('errorMessage', '')}")

        profile_snapshot = job.get("profileSnapshot")
        if not isinstance(profile_snapshot, dict) or not profile_snapshot:
            raise SmokeFailure("completed QueueJob is missing its frozen profile snapshot")
        if not job.get("profileCapturedAt"):
            raise SmokeFailure("completed QueueJob is missing its profile capture timestamp")
        plan_id = job.get("activeExecutionPlanId")
        if not isinstance(plan_id, int):
            raise SmokeFailure("completed QueueJob is missing its active ExecutionPlan identity")

        plans = api.request("GET", f"/api/queue/jobs/{job_id}/execution-plans", evidence_name="execution-plans.json")
        plan_summary = extract_plan_summary(plans)
        if not any(row.get("id") == plan_id for row in plan_summary["plans"]):
            raise SmokeFailure("active ExecutionPlan is not present in the persisted plan list")
        if not any(row.get("selectedEncoder") == "libx265" for row in plan_summary["plans"]):
            raise SmokeFailure("ExecutionPlan did not select the required portable libx265 encoder")
        evidence.json("frozen-plan-summary.json", {
            "queueJobId": job_id,
            "profileCapturedAt": job.get("profileCapturedAt"),
            "profileSnapshotKeys": sorted(profile_snapshot.keys()),
            "activeExecutionPlanId": plan_id,
            **plan_summary,
        })

        log("phase=result-artifact")
        artifacts = api.request(
            "GET", f"/api/queue/jobs/{job_id}/artifacts", evidence_name="result-artifact.json",
        )
        if not isinstance(artifacts, dict) or not isinstance(artifacts.get("result"), dict):
            raise SmokeFailure("completed QueueJob has no Result artifact")
        result_path = container_media_path(artifacts.get("resultPath"), "Result artifact path")
        run_command(["docker", "exec", container, "/usr/bin/test", "-s", result_path], timeout=15)

        log("phase=validation")
        validation = api.request(
            "POST", f"/api/validation/jobs/{job_id}", payload={}, expected=(200,),
            evidence_name="validation-result.json",
        )
        run_id = validation.get("runId") if isinstance(validation, dict) else None
        if not isinstance(run_id, str) or not run_id:
            raise SmokeFailure("Validation did not persist and return a ValidationRun id")
        validation_run = api.request(
            "GET", f"/api/validation/jobs/{job_id}/runs/{run_id}",
            evidence_name="validation-run.json",
        )
        if not isinstance(validation_run, dict):
            raise SmokeFailure("ValidationRun detail response was not an object")
        if not source_path.is_file() or sha256_file(source_path) != source_hash_before:
            raise SmokeFailure("Worker or Validation changed the synthetic source before publication")

        log("phase=publish-to-isolated-library")
        published = api.request(
            "POST", f"/api/publisher/jobs/{job_id}/publish", expected=(200,),
            payload={"reason": "Synthetic isolated pipeline smoke"},
            evidence_name="publish-result.json",
        )
        published_path = container_media_path(
            published.get("publishedPath") if isinstance(published, dict) else None,
            "published Library path",
        )
        if not published_path.startswith("/media/library/published/"):
            raise SmokeFailure("Publisher wrote outside the synthetic Library destination")
        run_command(["docker", "exec", container, "/usr/bin/test", "-s", published_path], timeout=15)
        published_probe = probe_media(container, published_path)
        evidence.json("ffprobe-summary.json", {"fixture": fixture_probe, "published": published_probe})
        if not any(stream.get("codec_type") == "video" for stream in published_probe.get("streams", [])):
            raise SmokeFailure("published Result has no video stream")
        published_job_list = api.request("GET", "/api/queue/jobs", evidence_name="queue-job-after-publish.json")
        published_job = next(
            (row for row in published_job_list if isinstance(row, dict) and row.get("id") == job_id),
            {},
        )
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
            "resultPath": result_path,
            "publishedPath": published_path,
            "originalArchivedPath": archived_path,
            "originalHashPreserved": True,
            "networkMode": "none",
            "rootFilesystemReadOnly": True,
        })
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
    sandbox = Path(args.sandbox).expanduser().resolve(strict=True)
    run_root = sandbox.parent
    base = validate_temp_base(run_root.parent)
    if run_root.parent != base or not run_root.name.startswith(RUN_PREFIX) or sandbox.name != "sandbox" or not sandbox.is_dir():
        raise SmokeFailure("fixture generation requires the sandbox directory under a run root")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", args.image):
        raise SmokeFailure("image must be a simple Docker image reference")
    docker_mount(sandbox)
    generate_fixture(args.image, sandbox)
    print(f"Generated deterministic synthetic fixture in {sandbox}")
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
    generate.add_argument("--image", default=DEFAULT_IMAGE)
    generate.add_argument("--sandbox", required=True)
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv if argv is not None else sys.argv[1:])
    try:
        if args.command == "run":
            return run_smoke(args)
        return generate_only(args)
    except Exception as exc:
        print(f"pipeline smoke error: {exc}", file=sys.stderr, flush=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
