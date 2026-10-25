#!/usr/bin/env python3
"""Run bounded, mock-only Ollama qualification in an isolated Compose project."""

from __future__ import annotations

import argparse
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import uuid
from pathlib import Path, PurePosixPath
from typing import Any
from urllib.parse import urlsplit


BASE_SHA = "8bc3a0c147d34350156e4daf499d989b73d3d179"
OWNER = "ollama-validation"
PROJECT_PREFIX = "mvf-ollama-"
COMPOSE_FILE = Path(__file__).with_name("ollama_validation") / "compose.yaml"
EXPECTED_SERVICES = {"sandbox-init", "backend", "ollama-mock", "test-runner"}
EXPECTED_NETWORK = "private"
EXPECTED_VOLUME = "sandbox-data"
FORBIDDEN_PATHS = ("/volume1", "/volume2", "/mnt/nas", "/nas-media-stack", "/media/raw/movies")


class HarnessError(RuntimeError):
    pass


class Docker:
    def __init__(self, context: str):
        self.context = context
        self.prefix = ["docker", "--context", context]

    def run(self, args: list[str], *, capture: bool = True, check: bool = True) -> str:
        command = self.prefix + args
        result = subprocess.run(command, text=True, capture_output=capture)
        if check and result.returncode != 0:
            combined = "\n".join(part for part in (result.stdout, result.stderr) if part)
            tail = "\n".join(combined.splitlines()[-60:])
            raise HarnessError(f"command failed ({result.returncode}): {command[1:4]}\n{tail}")
        return result.stdout.strip() if capture else ""

    def json(self, args: list[str]) -> Any:
        try:
            return json.loads(self.run(args))
        except json.JSONDecodeError as error:
            raise HarnessError(f"Docker returned invalid JSON for {args[:3]}") from error


def say(message: str) -> None:
    print(f"[ollama-sandbox] {message}", file=sys.stderr, flush=True)


def parse_bytes(value: Any) -> int:
    if isinstance(value, (int, float)):
        return int(value)
    text = str(value).strip().lower()
    match = re.fullmatch(r"(\d+(?:\.\d+)?)(b|k|kb|kib|m|mb|mib|g|gb|gib)?", text)
    if not match:
        raise HarnessError(f"unrecognized resource limit {value!r}")
    amount = float(match.group(1))
    unit = match.group(2) or "b"
    multiplier = 1 if unit == "b" else 1024
    if unit.startswith("m"):
        multiplier = 1024**2
    elif unit.startswith("g"):
        multiplier = 1024**3
    elif unit.startswith("k"):
        multiplier = 1024
    return int(amount * multiplier)


def _labels(value: Any) -> dict[str, str]:
    if isinstance(value, dict):
        return {str(key): str(item) for key, item in value.items()}
    if isinstance(value, list):
        result = {}
        for item in value:
            key, separator, text = str(item).partition("=")
            if separator:
                result[key] = text
        return result
    return {}


def _network_names(service: dict[str, Any]) -> set[str]:
    networks = service.get("networks") or {}
    if isinstance(networks, list):
        return set(networks)
    return set(networks.keys())


def validate_compose_config(
    config: dict[str, Any], *, project: str, run_id: str, source_sha: str,
    source_context: str, checkout_context: str,
) -> None:
    services = config.get("services")
    if not isinstance(services, dict) or set(services) != EXPECTED_SERVICES:
        raise HarnessError("Compose service set does not match the isolated harness")
    networks = config.get("networks") or {}
    if set(networks) != {EXPECTED_NETWORK} or networks[EXPECTED_NETWORK].get("internal") is not True:
        raise HarnessError("Compose must contain exactly one internal private network")
    volumes = config.get("volumes") or {}
    volume = volumes.get(EXPECTED_VOLUME) or {}
    if volume.get("name") != f"{project}-data":
        raise HarnessError("Compose data volume name is not run-scoped")
    expected_labels = {
        "mvforge.sandbox.owner": OWNER,
        "mvforge.sandbox.run-id": run_id,
        "mvforge.sandbox.source-sha": source_sha,
        "mvforge.sandbox.mode": "mock",
    }
    if _labels(volume.get("labels")) != {key: expected_labels[key] for key in expected_labels if key != "mvforge.sandbox.mode"}:
        raise HarnessError("Compose data volume is missing its ownership labels")
    if _labels((networks[EXPECTED_NETWORK]).get("labels")) != {key: expected_labels[key] for key in expected_labels if key != "mvforge.sandbox.mode"}:
        raise HarnessError("Compose private network is missing its ownership labels")

    expected_volume_services = {"sandbox-init", "backend", "test-runner"}
    expected_image_names = {
        "backend": f"mvforge-ollama-validation-{run_id}-backend",
        "ollama-mock": f"mvforge-ollama-validation-{run_id}-mock",
        "test-runner": f"mvforge-ollama-validation-{run_id}-tests",
        "sandbox-init": f"mvforge-ollama-validation-{run_id}-mock",
    }
    for service_name, service in services.items():
        if service.get("image") != expected_image_names[service_name]:
            raise HarnessError(f"{service_name} image is not uniquely named for this run")
        if service.get("privileged") is True or service.get("network_mode") == "host":
            raise HarnessError(f"{service_name} requests an unsafe runtime mode")
        if service.get("devices") or service.get("device_cgroup_rules"):
            raise HarnessError(f"{service_name} requests device access")
        if service.get("ports"):
            raise HarnessError("sandbox services must not publish host ports")
        if _network_names(service) != {EXPECTED_NETWORK}:
            raise HarnessError(f"{service_name} is attached outside the private network")
        if set(service.get("cap_drop") or []) != {"ALL"}:
            raise HarnessError(f"{service_name} must drop all Linux capabilities")
        cap_add = set(service.get("cap_add") or [])
        if cap_add:
            raise HarnessError(f"{service_name} requests an unexpected capability")
        security = set(service.get("security_opt") or [])
        if "no-new-privileges:true" not in security:
            raise HarnessError(f"{service_name} does not disable privilege escalation")
        if service.get("read_only") is not True:
            raise HarnessError(f"{service_name} root filesystem is writable")
        pids_limit = int(service.get("pids_limit") or 0)
        memory = parse_bytes(service.get("mem_limit") or 0)
        cpus = float(service.get("cpus") or 0)
        if pids_limit <= 0 or pids_limit > 256 or memory <= 0 or memory > 4 * 1024**3 or cpus <= 0 or cpus > 2:
            raise HarnessError(f"{service_name} is missing bounded CPU, memory, or PID limits")
        for mount in service.get("volumes") or []:
            if not isinstance(mount, dict) or mount.get("type") != "volume" or mount.get("source") != EXPECTED_VOLUME or mount.get("target") != "/media":
                raise HarnessError(f"{service_name} contains an unexpected mount")
        has_data_volume = bool(service.get("volumes"))
        if has_data_volume != (service_name in expected_volume_services):
            raise HarnessError(f"{service_name} has unexpected persistent storage")
        if service_name in {"backend", "test-runner"} and service.get("user") != "65532:65532":
            raise HarnessError(f"{service_name} must run as the dedicated unprivileged user")
        service_labels = _labels(service.get("labels"))
        if any(service_labels.get(key) != value for key, value in expected_labels.items()):
            raise HarnessError(f"{service_name} is missing run ownership labels")
        build = service.get("build") or {}
        context = str(build.get("context") or "")
        if service_name == "sandbox-init":
            if build:
                raise HarnessError("volume initializer must reuse the mock image")
        else:
            expected_context = str(Path(source_context, "backend").resolve()) if service_name == "backend" else checkout_context
            if context != expected_context:
                raise HarnessError(f"{service_name} build context is not the expected source")
            build_labels = _labels(build.get("labels"))
            for key, value in expected_labels.items():
                if build_labels.get(key) != value:
                    raise HarnessError(f"{service_name} image build is missing source ownership labels")
        serialized = json.dumps(service, sort_keys=True).lower()
        if any(marker in serialized for marker in ("/var/run/docker.sock", *FORBIDDEN_PATHS)):
            raise HarnessError(f"{service_name} references a forbidden host path")

    backend_env = services["backend"].get("environment") or {}
    if backend_env.get("MVFORGE_DB_PATH") != "/media/config/mvforge.db":
        raise HarnessError("backend SQLite path is not the sandbox volume")
    if backend_env.get("MVFORGE_AI_MODEL_LIFECYCLE_ENABLED") != "false":
        raise HarnessError("mock mode must keep real model lifecycle disabled")
    runner_env = services["test-runner"].get("environment") or {}
    if runner_env.get("OLLAMA_SANDBOX_ENDPOINT") != "http://ollama-mock:11434":
        raise HarnessError("test runner does not point exclusively at the Ollama mock")
    if runner_env.get("MVFORGE_API_URL") != "http://backend:8080":
        raise HarnessError("test runner does not use the sandbox backend")
    if Path(source_context).resolve() == Path(checkout_context).resolve():
        raise HarnessError("backend build context must be a separate exact-SHA source snapshot")


def _context_host(docker: Docker) -> str:
    inspect = docker.json(["context", "inspect", docker.context])
    try:
        return str(inspect[0]["Endpoints"]["docker"]["Host"])
    except (KeyError, IndexError, TypeError) as error:
        raise HarnessError("Docker context has no inspectable daemon endpoint") from error


def _validate_source_sha(checkout: Path, docker: Docker, sha: str) -> None:
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise HarnessError("base SHA must be a full lowercase Git commit SHA")
    resolved = subprocess.run(["git", "rev-parse", f"{sha}^{{commit}}"], cwd=checkout, text=True, capture_output=True)
    if resolved.returncode != 0 or resolved.stdout.strip() != sha:
        raise HarnessError("requested base SHA is not available in this checkout")
    server_version = docker.run(["version", "--format", "{{.Server.Version}}"])
    if not server_version:
        raise HarnessError("selected Docker context has no reachable engine")


def _extract_git_archive(checkout: Path, sha: str, destination: Path) -> None:
    archive = subprocess.run(["git", "archive", "--format=tar", sha], cwd=checkout, capture_output=True)
    if archive.returncode != 0:
        raise HarnessError("could not create the exact-SHA backend source snapshot")
    destination.mkdir(mode=0o700, parents=True, exist_ok=False)
    try:
        with tarfile.open(fileobj=io.BytesIO(archive.stdout), mode="r:") as source:
            root = destination.resolve()
            links: list[tarfile.TarInfo] = []
            for member in source.getmembers():
                relative = PurePosixPath(member.name)
                if relative.is_absolute() or not relative.parts or ".." in relative.parts:
                    raise HarnessError("Git source archive contains a path outside the snapshot")
                target = destination.joinpath(*relative.parts)
                if member.isdir():
                    target.mkdir(mode=0o755, parents=True, exist_ok=True)
                    continue
                target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                if not target.parent.resolve().is_relative_to(root):
                    raise HarnessError("Git source archive parent escaped the snapshot")
                if member.issym():
                    links.append(member)
                    continue
                if not member.isfile():
                    raise HarnessError("Git source archive contains an unsupported filesystem entry")
                content = source.extractfile(member)
                if content is None:
                    raise HarnessError("Git source archive contains an unreadable file")
                with content, target.open("xb") as output:
                    shutil.copyfileobj(content, output)
                target.chmod(member.mode & 0o777)
            for member in links:
                relative = PurePosixPath(member.name)
                target = destination.joinpath(*relative.parts)
                link = Path(member.linkname)
                resolved_link = (target.parent / link).resolve(strict=False)
                if link.is_absolute() or not resolved_link.is_relative_to(root):
                    raise HarnessError("Git source archive contains a symlink outside the snapshot")
                target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                if not target.parent.resolve().is_relative_to(root):
                    raise HarnessError("Git source archive symlink parent escaped the snapshot")
                target.symlink_to(member.linkname)
    except (tarfile.TarError, OSError, ValueError) as error:
        raise HarnessError("exact-SHA source snapshot could not be safely extracted") from error


def _write_env(path: Path, values: dict[str, str]) -> None:
    for value in values.values():
        if "\n" in value or "\r" in value:
            raise HarnessError("Compose environment values cannot contain line breaks")
    path.write_text("".join(f"{key}={value}\n" for key, value in values.items()), encoding="utf-8")
    path.chmod(0o600)


def _compose(docker: Docker, project: str, env_file: Path, args: list[str]) -> str:
    return docker.run([
        "compose", "--project-name", project, "--env-file", str(env_file),
        "-f", str(COMPOSE_FILE), *args,
    ])


def _inspect_containers(docker: Docker, project: str) -> list[dict[str, Any]]:
    ids = docker.run(["ps", "-aq", "--filter", f"label=com.docker.compose.project={project}"])
    return docker.json(["inspect", *ids.split()]) if ids else []


def _inspect_named(docker: Docker, kind: str, name: str) -> dict[str, Any]:
    value = docker.json([kind, "inspect", name])
    if not isinstance(value, list) or len(value) != 1:
        raise HarnessError(f"expected one {kind} resource named for this run")
    return value[0]


def _expected_labels(run_id: str, source_sha: str, mode: str = "mock") -> dict[str, str]:
    return {
        "mvforge.sandbox.owner": OWNER,
        "mvforge.sandbox.run-id": run_id,
        "mvforge.sandbox.source-sha": source_sha,
        "mvforge.sandbox.mode": mode,
    }


def inspect_effective_resources(
    docker: Docker, *, project: str, run_id: str, source_sha: str,
    image_names: dict[str, str], require_all_services: bool = True,
) -> dict[str, Any]:
    containers = _inspect_containers(docker, project)
    expected = _expected_labels(run_id, source_sha)
    by_service: dict[str, dict[str, Any]] = {}
    for container in containers:
        config = container.get("Config") or {}
        labels = config.get("Labels") or {}
        if any(labels.get(key) != value for key, value in expected.items()):
            raise HarnessError("a Compose project container has unexpected ownership labels")
        service = labels.get("com.docker.compose.service")
        if not service or service in by_service:
            raise HarnessError("unexpected or duplicate Compose service container")
        by_service[service] = container
        if container.get("Image") != (docker.json(["image", "inspect", image_names[service]])[0].get("Id")):
            raise HarnessError(f"{service} container image ID differs from its built image")
        image_labels = docker.json(["image", "inspect", image_names[service]])[0].get("Config", {}).get("Labels") or {}
        if any(image_labels.get(key) != value for key, value in expected.items()):
            raise HarnessError(f"{service} image source/owner labels do not match this run")
        host = container.get("HostConfig") or {}
        if host.get("Privileged") or host.get("NetworkMode") != f"{project}-{EXPECTED_NETWORK}":
            raise HarnessError(f"{service} effective Docker isolation is unsafe")
        if host.get("ReadonlyRootfs") is not True or set(host.get("CapDrop") or []) != {"ALL"}:
            raise HarnessError(f"{service} effective rootfs/capability restrictions differ")
        if host.get("CapAdd"):
            raise HarnessError(f"{service} has an effective added capability")
        if host.get("Devices") or host.get("DeviceRequests") or host.get("PortBindings"):
            raise HarnessError(f"{service} has device access or a published port")
        if not any("no-new-privileges:true" in value for value in host.get("SecurityOpt") or []):
            raise HarnessError(f"{service} lacks no-new-privileges")
        if int(host.get("PidsLimit") or 0) <= 0 or int(host.get("PidsLimit") or 0) > 256:
            raise HarnessError(f"{service} effective PID limit is missing or excessive")
        if int(host.get("Memory") or 0) <= 0 or int(host.get("Memory") or 0) > 4 * 1024**3:
            raise HarnessError(f"{service} effective memory limit is missing or excessive")
        if int(host.get("NanoCpus") or 0) <= 0 or int(host.get("NanoCpus") or 0) > 2_000_000_000:
            raise HarnessError(f"{service} effective CPU limit is missing or excessive")
        networks = (container.get("NetworkSettings") or {}).get("Networks") or {}
        if set(networks) != {f"{project}-{EXPECTED_NETWORK}"}:
            raise HarnessError(f"{service} is connected to an unexpected network")
        mounts = container.get("Mounts") or []
        allowed_targets = {"/media"} if service in {"sandbox-init", "backend", "test-runner"} else set()
        for mount in mounts:
            destination = mount.get("Destination")
            if destination == "/tmp" and mount.get("Type") == "tmpfs":
                continue
            if destination not in allowed_targets:
                raise HarnessError(f"{service} has an unexpected effective mount")
        if service in {"sandbox-init", "backend", "test-runner"}:
            data_mounts = [mount for mount in mounts if mount.get("Destination") == "/media"]
            if len(data_mounts) != 1 or data_mounts[0].get("Type") != "volume" or data_mounts[0].get("Name") != f"{project}-data" or data_mounts[0].get("RW") is not True:
                raise HarnessError(f"{service} does not use only the run-scoped data volume")
        elif mounts:
            raise HarnessError("Ollama mock must have no persistent mounts")
        env = config.get("Env") or []
        environment = dict(item.split("=", 1) for item in env if "=" in item)
        for item in env:
            if any(marker in item.lower() for marker in (*FORBIDDEN_PATHS, "/var/run/docker.sock", "host.docker.internal")):
                raise HarnessError(f"{service} container environment references a forbidden endpoint/path")
        if service == "backend" and environment.get("MVFORGE_DB_PATH") != "/media/config/mvforge.db":
            raise HarnessError("effective backend SQLite path is not isolated")
        if service == "test-runner" and environment.get("OLLAMA_SANDBOX_ENDPOINT") != "http://ollama-mock:11434":
            raise HarnessError("effective test endpoint does not resolve to the mock")
    if require_all_services and set(by_service) != EXPECTED_SERVICES:
        raise HarnessError("created container set differs from the expected service set")

    volume = _inspect_named(docker, "volume", f"{project}-data")
    if any((volume.get("Labels") or {}).get(key) != value for key, value in expected.items() if key != "mvforge.sandbox.mode"):
        raise HarnessError("effective data volume ownership labels do not match")
    network = _inspect_named(docker, "network", f"{project}-{EXPECTED_NETWORK}")
    if network.get("Internal") is not True or any((network.get("Labels") or {}).get(key) != value for key, value in expected.items() if key != "mvforge.sandbox.mode"):
        raise HarnessError("effective network is not the isolated run-scoped network")
    network_container_ids = set((network.get("Containers") or {}).keys())
    expected_container_ids = {container.get("Id") for container in containers}
    if not network_container_ids.issubset(expected_container_ids):
        raise HarnessError("private network contains a container outside this Compose project")
    running_ids = {container.get("Id") for container in containers if (container.get("State") or {}).get("Running") is True}
    if not running_ids.issubset(network_container_ids):
        raise HarnessError("a running Compose service is not attached to the isolated network")
    return {
        "containerIds": {name: container["Id"] for name, container in by_service.items()},
        "imageIds": {name: docker.json(["image", "inspect", image_names[name]])[0]["Id"] for name in image_names},
        "networkId": network["Id"],
        "volumeName": volume["Name"],
    }


def _wait_health(docker: Docker, container_id: str, timeout_seconds: int = 180) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        state = docker.json(["inspect", container_id])[0].get("State") or {}
        health = ((state.get("Health") or {}).get("Status"))
        if health == "healthy":
            return
        if state.get("Status") in {"exited", "dead"}:
            raise HarnessError("sandbox service exited before passing its health check")
        time.sleep(2)
    raise HarnessError("sandbox service health check timed out")


def _write_report(path: Path, report: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def _cleanup(docker: Docker, project: str, run_id: str, source_sha: str, env_file: Path, image_names: dict[str, str]) -> list[str]:
    retained: list[str] = []
    expected = _expected_labels(run_id, source_sha)
    containers = _inspect_containers(docker, project)
    for container in containers:
        labels = (container.get("Config") or {}).get("Labels") or {}
        if any(labels.get(key) != value for key, value in expected.items()):
            retained.append(f"container:{container.get('Id', 'unknown')}")
    volumes = docker.run(["volume", "ls", "-q", "--filter", f"label=com.docker.compose.project={project}"]).split()
    for name in volumes:
        item = _inspect_named(docker, "volume", name)
        labels = item.get("Labels") or {}
        if any(labels.get(key) != value for key, value in expected.items() if key != "mvforge.sandbox.mode"):
            retained.append(f"volume:{name}")
    networks = docker.run(["network", "ls", "-q", "--filter", f"label=com.docker.compose.project={project}"]).split()
    for network_id in networks:
        item = _inspect_named(docker, "network", network_id)
        labels = item.get("Labels") or {}
        if any(labels.get(key) != value for key, value in expected.items() if key != "mvforge.sandbox.mode"):
            retained.append(f"network:{network_id}")
    if retained:
        return retained
    for container in containers:
        labels = (container.get("Config") or {}).get("Labels") or {}
        if labels.get("com.docker.compose.oneoff", "").lower() == "true":
            container_id = container["Id"]
            docker.run(["stop", container_id], check=False)
            docker.run(["container", "rm", container_id], check=False)
    _compose(docker, project, env_file, ["down", "--volumes", "--remove-orphans"])
    remaining = _inspect_containers(docker, project)
    if remaining:
        retained.extend(f"container:{item.get('Id', 'unknown')}" for item in remaining)
        return retained
    remaining_volumes = docker.run(["volume", "ls", "-q", "--filter", f"label=com.docker.compose.project={project}"]).split()
    remaining_networks = docker.run(["network", "ls", "-q", "--filter", f"label=com.docker.compose.project={project}"]).split()
    retained.extend(f"volume:{name}" for name in remaining_volumes)
    retained.extend(f"network:{item}" for item in remaining_networks)
    if retained:
        return retained
    for image_name in sorted(set(image_names.values())):
        raw_inspect = docker.run(["image", "inspect", image_name], check=False)
        try:
            inspect = json.loads(raw_inspect) if raw_inspect else []
        except json.JSONDecodeError:
            inspect = []
        if inspect:
            labels = (inspect[0].get("Config") or {}).get("Labels") or {}
            if any(labels.get(key) != value for key, value in expected.items()):
                retained.append(f"image:{image_name}")
                continue
            docker.run(["image", "rm", inspect[0]["Id"]])
    return retained


def validate_real_plan(endpoint: str, model: str) -> dict[str, Any]:
    parsed = urlsplit(endpoint)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise HarnessError("real-plan requires an exact HTTP(S) endpoint without credentials, query, or fragment")
    if parsed.path not in {"", "/"}:
        raise HarnessError("real-plan endpoint must name the Ollama service root")
    if not model or model != model.strip() or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", model):
        raise HarnessError("real-plan requires one exact model name")
    return {
        "mode": "real-plan",
        "result": "PASS",
        "endpoint": f"{parsed.scheme}://{parsed.netloc}",
        "model": model,
        "executionAllowed": False,
        "realInferenceExecuted": False,
        "checks": {
            "explicitEndpointAndModel": "PASS",
            "productionSQLiteOrSettingsRead": "NOT_RUN",
            "hostTelemetry": "PENDING_REAL_ENGINE_OBSERVATION",
            "liveResourceAdmission": "PENDING_APPLICATION_ADMISSION",
            "maximumConcurrentInference": 1,
            "simulatedAdmissionAuthorizesRealInference": False,
            "decision": "NO_GO_UNTIL_LIVE_TELEMETRY_AND_ADMISSION_ARE_VERIFIED",
        },
    }


def run_mock(args: argparse.Namespace) -> dict[str, Any]:
    checkout = Path(__file__).resolve().parents[2]
    context = args.context or subprocess.run(["docker", "context", "show"], text=True, capture_output=True, check=True).stdout.strip()
    docker = Docker(context)
    _validate_source_sha(checkout, docker, args.base_sha)
    engine_host = _context_host(docker)
    remote_engine = not (engine_host.startswith("unix://") or engine_host.startswith("npipe://") or "localhost" in engine_host or "127.0.0.1" in engine_host)
    if remote_engine and not args.allow_remote_engine:
        raise HarnessError("remote Docker context rejected; pass --allow-remote-engine only for a separately authorized isolated engine")

    run_id = uuid.uuid4().hex[:12]
    project = f"{PROJECT_PREFIX}{run_id}"
    image_names = {
        "backend": f"mvforge-ollama-validation-{run_id}-backend",
        "ollama-mock": f"mvforge-ollama-validation-{run_id}-mock",
        "test-runner": f"mvforge-ollama-validation-{run_id}-tests",
        "sandbox-init": f"mvforge-ollama-validation-{run_id}-mock",
    }
    temp_root = Path(tempfile.mkdtemp(prefix=f"{PROJECT_PREFIX}{run_id}-"))
    temp_root.chmod(0o700)
    marker = temp_root / ".mvforge-sandbox-run"
    marker.write_text(json.dumps({"runId": run_id, "project": project, "sourceSha": args.base_sha}) + "\n", encoding="utf-8")
    marker.chmod(0o600)
    source_context = temp_root / "exact-source"
    env_file = temp_root / "compose.env"
    resource_created = False
    images_built: dict[str, str] = {}
    report: dict[str, Any] = {
        "mode": "mock", "runId": run_id, "project": project, "baseSha": args.base_sha,
        "dockerContext": context, "engineEndpoint": engine_host,
        "checks": {}, "tests": [], "resources": {"remoteEngine": remote_engine, "runtimeResourcesRemoved": False},
    }
    runner_id: str | None = None
    say(f"created run scope {run_id} on engine {context}")
    try:
        _extract_git_archive(checkout, args.base_sha, source_context)
        values = {
            "COMPOSE_PROJECT_NAME": project,
            "RUN_ID": run_id,
            "SOURCE_SHA": args.base_sha,
            "SOURCE_CONTEXT": str(source_context.resolve()),
            "BACKEND_CONTEXT": str((source_context / "backend").resolve()),
            "CHECKOUT_CONTEXT": str(checkout.resolve()),
            "BACKEND_IMAGE": image_names["backend"],
            "MOCK_IMAGE": image_names["ollama-mock"],
            "TEST_IMAGE": image_names["test-runner"],
        }
        _write_env(env_file, values)
        config = json.loads(_compose(docker, project, env_file, ["config", "--format", "json"]))
        validate_compose_config(
            config, project=project, run_id=run_id, source_sha=args.base_sha,
            source_context=str(source_context.resolve()), checkout_context=str(checkout.resolve()),
        )
        report["checks"]["renderedComposeIsolation"] = "PASS"
        existing = docker.run(["ps", "-aq", "--filter", f"label=com.docker.compose.project={project}"])
        if existing:
            raise HarnessError("generated Compose project ID is already in use")

        resource_created = True
        say("building backend from the exact requested Git snapshot")
        _compose(docker, project, env_file, ["build", "--pull", "backend"])
        say("building mock and isolated Go test runner")
        _compose(docker, project, env_file, ["build", "--pull", "ollama-mock"])
        _compose(docker, project, env_file, ["build", "--pull", "test-runner"])
        for service, image_name in image_names.items():
            if service == "sandbox-init":
                continue
            image = _inspect_named(docker, "image", image_name)
            labels = image.get("Config", {}).get("Labels") or {}
            if any(labels.get(key) != value for key, value in _expected_labels(run_id, args.base_sha).items()):
                raise HarnessError(f"built {service} image has mismatched run/source identity")
            images_built[service] = image["Id"]
        report["imageIds"] = images_built
        report["checks"]["backendBuiltFromExactSha"] = "PASS"

        _compose(docker, project, env_file, ["create", "sandbox-init", "ollama-mock", "backend", "test-runner"])
        effective = inspect_effective_resources(
            docker, project=project, run_id=run_id, source_sha=args.base_sha,
            image_names=image_names, require_all_services=True,
        )
        report["preStartInspection"] = effective
        report["checks"]["effectiveMountsImagesNetworksPortsAndLimits"] = "PASS"
        say("pre-start Docker inspection passed; initializing only the owned temporary volume")

        init_id = effective["containerIds"]["sandbox-init"]
        docker.run(["start", init_id])
        init_status = docker.run(["wait", init_id])
        if init_status != "0":
            details = docker.run(["logs", init_id], check=False)
            raise HarnessError(f"temporary volume initializer did not exit successfully: {details[-500:]}")
        mock_id = effective["containerIds"]["ollama-mock"]
        docker.run(["start", mock_id])
        _wait_health(docker, mock_id, timeout_seconds=45)
        backend_id = effective["containerIds"]["backend"]
        docker.run(["start", backend_id])
        _wait_health(docker, backend_id, timeout_seconds=180)
        post_start = inspect_effective_resources(
            docker, project=project, run_id=run_id, source_sha=args.base_sha,
            image_names=image_names, require_all_services=True,
        )
        report["postStartInspection"] = post_start
        report["checks"]["postStartIsolation"] = "PASS"

        docker.run(["exec", backend_id, "/usr/bin/test", "-w", "/media/config"])
        docker.run(["exec", backend_id, "/usr/bin/test", "-s", "/media/config/mvforge.db"])
        integrity = docker.run(["exec", backend_id, "/usr/bin/sqlite3", "-readonly", "/media/config/mvforge.db", "PRAGMA integrity_check;"])
        if integrity.strip() != "ok":
            raise HarnessError("sandbox SQLite integrity check failed")
        report["checks"]["sandboxSQLitePathWriteAndIntegrity"] = "PASS"

        runner_output = _compose(docker, project, env_file, [
            "run", "--detach", "--no-deps", "--entrypoint", "/bin/sleep", "test-runner", "900",
        ])
        oneoff_ids = docker.run([
            "ps", "-aq", "--filter", f"label=com.docker.compose.project={project}",
            "--filter", "label=com.docker.compose.oneoff=True",
        ]).split()
        if len(oneoff_ids) != 1:
            raise HarnessError(f"could not identify isolated test runner container ({runner_output})")
        runner_id = oneoff_ids[0]
        runner_info = docker.json(["inspect", runner_id])[0]
        labels = runner_info.get("Config", {}).get("Labels") or {}
        if any(labels.get(key) != value for key, value in _expected_labels(run_id, args.base_sha).items()):
            raise HarnessError("one-off test runner has unexpected ownership labels")
        # Reuse the same effective-resource assertions by temporarily treating this as the service runner.
        runner_mounts = runner_info.get("Mounts") or []
        if len([mount for mount in runner_mounts if mount.get("Destination") == "/media" and mount.get("Name") == f"{project}-data" and mount.get("RW")]) != 1:
            raise HarnessError("one-off test runner does not use the isolated data volume")
        host = runner_info.get("HostConfig") or {}
        if host.get("Privileged") or host.get("NetworkMode") != f"{project}-{EXPECTED_NETWORK}" or host.get("ReadonlyRootfs") is not True or set(host.get("CapDrop") or []) != {"ALL"} or host.get("Devices") or host.get("DeviceRequests") or host.get("PortBindings"):
            raise HarnessError("one-off test runner effective isolation failed")
        if int(host.get("PidsLimit") or 0) > 256 or int(host.get("Memory") or 0) > 4 * 1024**3:
            raise HarnessError("one-off test runner resource limit exceeded")
        report["checks"]["testRunnerEffectiveIsolation"] = "PASS"

        suites = [
            ("mock_network_integration", ["/usr/local/bin/ollama-sandbox.test", "-test.count=1", "-test.run=^TestOllamaSandbox"]),
            ("focused_contract_regressions", [
                "/usr/local/bin/ollama-focused.test", "-test.count=1",
                "-test.run=^(TestSubtitleOCRAIUsesExplicitModelAndLeavesCanonicalApprovalFlowUnchanged|TestSubtitleOCRAIDisabledAndAdmissionUnavailableSendNoModelRequests|TestSubtitleOCRAIColdStartsSelectedModelAndRejectsMalformedResponse|TestSubtitleOCRAIRevalidatesAfterWarmupAndHonorsCancellation|TestSubtitleOCRAIInvalidatesResultsWhenCanonicalTargetChanges|TestTranslateGemmaStructuredOutputSupportsSyntheticASSTranslation|TestExecuteTextTranslationRejectsProtectedASSMutationBeforeOutput|TestExecuteTextTranslationRejectsLaterBatchWithoutPartialOutput|TestOllamaTranslationProviderMalformedResponses|TestOllamaTranslationProviderRejectsTruncatedCompletion|TestOllamaTranslationProviderCancellationDuringRequest|TestOllamaTranslationProviderHTTPStatusFailures|TestOllamaTranslationProviderDoesNotPerformHealthOrModelChecks|TestPublishSubtitleTranslationSingleOutputAndMetadata|TestPublishSubtitleTranslationExistingBothRollsBackOnSecondMemberFailure|TestPublishSubtitleTranslationNewBothRollsBackCreatedMember|TestPublishSubtitleTranslationCancellationRollsBackAndCommittedRunWins)$",
            ]),
        ]
        for name, arguments in suites:
            say(f"running {name}")
            executable, *test_arguments = arguments
            output = docker.run(["exec", runner_id, executable, *test_arguments])
            report["tests"].append({"name": name, "result": "PASS", "summary": output.splitlines()[-1] if output else "ok"})

        report["checks"]["networkUsesMockAndExternalEgressBlocked"] = "PASS"
        report["checks"]["realOllamaConnections"] = "NOT_RUN"
        report["checks"]["productionSQLiteAndMedia"] = "NOT_ACCESSED"
        report["resources"]["containersCreated"] = len(_inspect_containers(docker, project))
        report["resources"]["volume"] = effective["volumeName"]
        report["resources"]["network"] = f"{project}-{EXPECTED_NETWORK}"
        report["resources"]["images"] = sorted(set(images_built.values()))
        report["resources"]["sharedBuildCache"] = "left untouched; Docker BuildKit cache is shared engine state"
    except Exception as error:
        report["result"] = "BLOCKED"
        report["error"] = str(error)
    finally:
        if runner_id:
            try:
                docker.run(["stop", runner_id], check=False)
            except Exception:
                pass
        if resource_created:
            try:
                say("removing only resources whose project and run ownership labels match")
                retained = _cleanup(docker, project, run_id, args.base_sha, env_file, image_names)
                report["resources"]["retainedOwnedResources"] = retained
                report["resources"]["runtimeResourcesRemoved"] = not retained
                if retained:
                    report["cleanupFailure"] = retained
                    report["result"] = "BLOCKED"
            except Exception as error:  # fail safe: leave unverified resources for inspection
                report["resources"]["runtimeResourcesRemoved"] = False
                report["cleanupFailure"] = str(error)
                report["result"] = "BLOCKED"
        else:
            report["resources"]["runtimeResourcesRemoved"] = True
        if marker.exists():
            try:
                marker_payload = json.loads(marker.read_text(encoding="utf-8"))
                safe_temp_root = (
                    not temp_root.is_symlink()
                    and temp_root.name.startswith(f"{PROJECT_PREFIX}{run_id}-")
                    and temp_root.parent.resolve() == Path(tempfile.gettempdir()).resolve()
                    and not marker.is_symlink()
                )
                if safe_temp_root and marker_payload == {"runId": run_id, "project": project, "sourceSha": args.base_sha}:
                    shutil.rmtree(temp_root)
                else:
                    report["cleanupFailure"] = "temporary source snapshot ownership/path check failed"
                    report["result"] = "BLOCKED"
            except (OSError, json.JSONDecodeError):
                report["cleanupFailure"] = "temporary source snapshot retained: ownership marker could not be confirmed"
                report["result"] = "BLOCKED"
    if "result" not in report:
        report["result"] = "PASS"
    return report


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("mock", "real-plan"), default="mock")
    parser.add_argument("--base-sha", default=BASE_SHA)
    parser.add_argument("--context", help="Docker context; defaults to the current context")
    parser.add_argument("--allow-remote-engine", action="store_true", help="Allow an isolated remote Docker Engine; its named volume is created on that engine host")
    parser.add_argument("--endpoint", help="Exact Ollama endpoint for real-plan; never contacted")
    parser.add_argument("--model", help="Exact OCR or Translation model for real-plan")
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        if args.mode == "real-plan":
            if not args.endpoint or not args.model:
                raise HarnessError("real-plan requires --endpoint and --model")
            report = validate_real_plan(args.endpoint, args.model)
        else:
            report = run_mock(args)
        print(json.dumps(report, indent=2, sort_keys=True))
        if report.get("result") != "PASS" or report.get("cleanupFailure"):
            return 1
        return 0
    except (HarnessError, subprocess.CalledProcessError) as error:
        print(json.dumps({"result": "BLOCKED", "error": str(error)}, indent=2), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
