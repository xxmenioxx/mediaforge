from __future__ import annotations

import contextlib
import io
import socket
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from ollama_validation_sandbox import (
    HarnessError,
    validate_compose_config,
    validate_effective_container,
    build_parser,
    validate_real_plan,
    validate_real_preflight,
)


RUN_ID = "012345abcdef"
PROJECT = f"mvf-ollama-{RUN_ID}"
SHA = "8bc3a0c147d34350156e4daf499d989b73d3d179"
OWNER_LABELS = {
    "mvforge.sandbox.owner": "ollama-validation",
    "mvforge.sandbox.run-id": RUN_ID,
    "mvforge.sandbox.source-sha": SHA,
}
RUN_LABELS = {**OWNER_LABELS, "mvforge.sandbox.mode": "mock"}
ROOT = Path("/tmp/mvforge-test")
SOURCE = str(ROOT / "exact-source")
CHECKOUT = str(ROOT / "worktree")


def service(name: str) -> dict:
    image_names = {
        "sandbox-init": f"mvforge-ollama-validation-{RUN_ID}-mock",
        "backend": f"mvforge-ollama-validation-{RUN_ID}-backend",
        "ollama-mock": f"mvforge-ollama-validation-{RUN_ID}-mock",
        "test-runner": f"mvforge-ollama-validation-{RUN_ID}-tests",
    }
    limits = {
        "sandbox-init": (32, 64 * 1024**2, 0.10),
        "backend": (128, 2 * 1024**3, 1.5),
        "ollama-mock": (64, 128 * 1024**2, 0.25),
        "test-runner": (256, 4 * 1024**3, 2),
    }
    pids, memory, cpus = limits[name]
    expected_volume = name in {"sandbox-init", "backend", "test-runner"}
    value = {
        "image": image_names[name],
        "read_only": True,
        "cap_drop": ["ALL"],
        "cap_add": [],
        "security_opt": ["no-new-privileges:true"],
        "pids_limit": pids,
        "mem_limit": str(memory),
        "cpus": cpus,
        "networks": {"private": None},
        "labels": dict(RUN_LABELS),
        "build": {
            "context": str(Path(SOURCE, "backend").resolve()) if name == "backend" else CHECKOUT,
            "labels": dict(RUN_LABELS),
        },
    }
    if name == "sandbox-init":
        value["build"] = {}
        value["user"] = "65532:65532"
    elif name in {"backend", "test-runner"}:
        value["user"] = "65532:65532"
    if expected_volume:
        value["volumes"] = [{"type": "volume", "source": "sandbox-data", "target": "/media"}]
    if name == "backend":
        value["environment"] = {
            "MVFORGE_DB_PATH": "/media/config/mvforge.db",
            "MVFORGE_AI_MODEL_LIFECYCLE_ENABLED": "false",
        }
    elif name == "test-runner":
        value["environment"] = {
            "OLLAMA_SANDBOX_ENDPOINT": "http://ollama-mock:11434",
            "MVFORGE_API_URL": "http://backend:8080",
        }
    return value


def valid_config() -> dict:
    return {
        "services": {name: service(name) for name in ("sandbox-init", "backend", "ollama-mock", "test-runner")},
        "networks": {"private": {"internal": True, "labels": dict(OWNER_LABELS)}},
        "volumes": {"sandbox-data": {"name": f"{PROJECT}-data", "labels": dict(OWNER_LABELS)}},
    }


def effective_network() -> dict:
    return {
        "Name": f"{PROJECT}-private",
        "Internal": True,
        "Labels": dict(OWNER_LABELS),
    }


def effective_test_runner() -> dict:
    return {
        "Image": "sha256:expected-test-runner",
        "Config": {
            "Labels": {
                **RUN_LABELS,
                "com.docker.compose.service": "test-runner",
                "com.docker.compose.oneoff": "True",
            },
            "Env": [
                "OLLAMA_SANDBOX_ENDPOINT=http://ollama-mock:11434",
                "MVFORGE_API_URL=http://backend:8080",
            ],
        },
        "HostConfig": {
            "Privileged": False,
            "NetworkMode": f"{PROJECT}-private",
            "ReadonlyRootfs": True,
            "CapDrop": ["ALL"],
            "CapAdd": [],
            "Devices": [],
            "DeviceRequests": [],
            "PortBindings": None,
            "Binds": None,
            "SecurityOpt": ["no-new-privileges:true"],
            "PidsLimit": 256,
            "Memory": 4 * 1024**3,
            "NanoCpus": 2_000_000_000,
            "Tmpfs": {"/tmp": "rw,noexec,nosuid,nodev,size=67108864,mode=1777"},
        },
        "NetworkSettings": {"Networks": {f"{PROJECT}-private": {}}},
        "Mounts": [
            {"Type": "volume", "Name": f"{PROJECT}-data", "Destination": "/media", "RW": True},
            {"Type": "tmpfs", "Destination": "/tmp", "RW": True},
        ],
    }


class EffectiveContainerTests(unittest.TestCase):
    def validate(self, container=None, network=None) -> None:
        validate_effective_container(
            container or effective_test_runner(),
            service="test-runner",
            project=PROJECT,
            run_id=RUN_ID,
            source_sha=SHA,
            expected_image_id="sha256:expected-test-runner",
            image_labels=RUN_LABELS,
            network=network or effective_network(),
        )

    def test_accepts_isolated_compose_oneoff_test_runner(self) -> None:
        self.validate()

    def test_rejects_extra_mounts_and_docker_socket(self) -> None:
        mutations = (
            lambda container: container["Mounts"].append({
                "Type": "bind", "Source": "/tmp/host-secret", "Destination": "/host", "RW": True,
            }),
            lambda container: container["Mounts"].append({
                "Type": "bind", "Source": "/var/run/docker.sock", "Destination": "/var/run/docker.sock", "RW": True,
            }),
            lambda container: container["HostConfig"].update({"Binds": ["/var/run/docker.sock:/var/run/docker.sock"]}),
            lambda container: container["HostConfig"]["Tmpfs"].update({"/run/secrets": "rw,size=1m"}),
            lambda container: container["Mounts"].append({
                "Type": "tmpfs", "Destination": "/run/secrets", "RW": True,
            }),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                container = effective_test_runner()
                mutate(container)
                with self.assertRaises(HarnessError):
                    self.validate(container)

    def test_rejects_missing_or_unlimited_effective_resource_limits(self) -> None:
        for field in ("PidsLimit", "Memory", "NanoCpus"):
            for value in (None, 0):
                with self.subTest(field=field, value=value):
                    container = effective_test_runner()
                    container["HostConfig"][field] = value
                    with self.assertRaises(HarnessError):
                        self.validate(container)

    def test_rejects_unexpected_capabilities_or_missing_no_new_privileges(self) -> None:
        mutations = (
            lambda host: host.update({"CapAdd": ["SYS_ADMIN"]}),
            lambda host: host.update({"CapDrop": []}),
            lambda host: host.update({"SecurityOpt": []}),
            lambda host: host.update({"ReadonlyRootfs": False}),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                container = effective_test_runner()
                mutate(container["HostConfig"])
                with self.assertRaises(HarnessError):
                    self.validate(container)

    def test_rejects_missing_or_excessive_tmpfs_bound(self) -> None:
        for options in ("rw,noexec,nosuid,nodev", "rw,noexec,nosuid,nodev,size=128m"):
            with self.subTest(options=options):
                container = effective_test_runner()
                container["HostConfig"]["Tmpfs"]["/tmp"] = options
                with self.assertRaises(HarnessError):
                    self.validate(container)

    def test_rejects_unexpected_or_external_networks(self) -> None:
        container = effective_test_runner()
        container["NetworkSettings"]["Networks"]["bridge"] = {}
        with self.assertRaises(HarnessError):
            self.validate(container)

        container = effective_test_runner()
        network = effective_network()
        network["Internal"] = False
        with self.assertRaises(HarnessError):
            self.validate(container, network)


class IsolationConfigTests(unittest.TestCase):
    def validate(self, config: dict) -> None:
        validate_compose_config(
            config, project=PROJECT, run_id=RUN_ID, source_sha=SHA,
            source_context=SOURCE, checkout_context=CHECKOUT,
        )

    def test_accepts_only_run_scoped_internal_configuration(self) -> None:
        self.validate(valid_config())

    def test_rejects_production_bind_mount(self) -> None:
        config = valid_config()
        config["services"]["backend"]["volumes"] = [{
            "type": "bind", "source": "/volume1/media", "target": "/media",
        }]
        with self.assertRaises(HarnessError):
            self.validate(config)

    def test_rejects_docker_socket_and_host_network(self) -> None:
        config = valid_config()
        config["services"]["test-runner"]["volumes"] = [{
            "type": "bind", "source": "/var/run/docker.sock", "target": "/var/run/docker.sock",
        }]
        with self.assertRaises(HarnessError):
            self.validate(config)
        config = valid_config()
        config["services"]["test-runner"]["network_mode"] = "host"
        with self.assertRaises(HarnessError):
            self.validate(config)

    def test_rejects_published_ports_privileged_devices_and_external_network(self) -> None:
        mutations = (
            lambda config: config["services"]["backend"].update({"ports": ["8080:8080"]}),
            lambda config: config["services"]["backend"].update({"privileged": True}),
            lambda config: config["services"]["ollama-mock"].update({"devices": ["/dev/dri:/dev/dri"]}),
            lambda config: config["networks"]["private"].update({"internal": False}),
        )
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                config = valid_config()
                mutate(config)
                with self.assertRaises(HarnessError):
                    self.validate(config)

    def test_rejects_unbounded_resources_or_real_provider_in_mock_mode(self) -> None:
        config = valid_config()
        config["services"]["test-runner"]["mem_limit"] = "8g"
        with self.assertRaises(HarnessError):
            self.validate(config)
        config = valid_config()
        config["services"]["test-runner"]["environment"]["OLLAMA_SANDBOX_ENDPOINT"] = "http://nas-media-stack:11434"
        with self.assertRaises(HarnessError):
            self.validate(config)

    def test_rejects_production_sqlite_or_mismatched_source_context(self) -> None:
        config = valid_config()
        config["services"]["backend"]["environment"]["MVFORGE_DB_PATH"] = "/volume1/app/mvforge.db"
        with self.assertRaises(HarnessError):
            self.validate(config)
        config = valid_config()
        config["services"]["backend"]["build"]["context"] = CHECKOUT
        with self.assertRaises(HarnessError):
            self.validate(config)

    def test_real_plan_is_non_executing_and_requires_exact_declarations(self) -> None:
        plan = validate_real_plan(
            "https://ollama.example.invalid:11434", "qwen2.5:7b", "translation-ass", SHA, RUN_ID,
        )
        self.assertEqual(plan["result"], "BLOCKED")
        self.assertEqual(plan["mode"], "real-preflight")
        self.assertFalse(plan["executionAllowed"])
        self.assertFalse(plan["providerContacted"])
        self.assertFalse(plan["modelWarmupExecuted"])
        self.assertFalse(plan["realInferenceExecuted"])
        self.assertFalse(plan["checks"]["previewOrCallerEvidenceAuthorizesExecution"])
        self.assertEqual(plan["checks"]["decision"], "NO_GO_MISSING_TRUSTED_TELEMETRY_ADMISSION_AND_EGRESS")
        self.assertEqual(plan["sourceSha"], SHA)
        self.assertEqual(plan["runId"], RUN_ID)
        for endpoint in (
            "http://user:secret@ollama.invalid:11434",
            "https://ollama.invalid/api/chat",
            "https://ollama.invalid:11434?model=x",
            "file:///tmp/model",
            "http://ollama.invalid",
        ):
            with self.subTest(endpoint=endpoint), self.assertRaises(HarnessError):
                validate_real_preflight(endpoint, "qwen2.5:7b", "translation-ass", SHA, RUN_ID)
        with self.assertRaises(HarnessError):
            validate_real_preflight("http://ollama.invalid:11434", " qwen2.5:7b ", "translation-ass", SHA, RUN_ID)

    def test_real_preflight_requires_exact_workflow_sha_and_run_identity(self) -> None:
        valid = ("http://ollama.invalid:11434", "translategemma:12b", "translation-ass", SHA, RUN_ID)
        invalid_targets = (
            (*valid[:2], "translation", *valid[3:]),
            (*valid[:3], "0cce40a", valid[4]),
            (*valid[:4], "run-427"),
            (*valid[:2], "publisher", *valid[3:]),
        )
        for target in invalid_targets:
            with self.subTest(target=target), self.assertRaises(HarnessError):
                validate_real_preflight(*target)

    def test_real_preflight_lists_every_unavailable_authority_as_no_go(self) -> None:
        plan = validate_real_preflight(
            "http://ollama.invalid:11434", "fra-ocr:latest", "ocr-ai", SHA, RUN_ID,
        )
        checks = plan["checks"]
        self.assertEqual(plan["result"], "BLOCKED")
        self.assertEqual(checks["hostRamCpuSwapTelemetry"], "UNAVAILABLE")
        self.assertEqual(checks["actualCapacityDomainBinding"], "UNVERIFIED")
        self.assertEqual(checks["ollamaProviderIdentityAndResidency"], "UNAVAILABLE")
        self.assertEqual(checks["criticalServiceActivity"], "UNAVAILABLE")
        self.assertEqual(checks["modelLoadAdmissionLease"], "UNAVAILABLE")
        self.assertEqual(checks["inferenceAdmissionLease"], "UNAVAILABLE")
        self.assertEqual(checks["restrictedExactEndpointEgress"], "UNVERIFIED")
        self.assertEqual(checks["momentOfRunAuthorization"], "UNAVAILABLE")
        self.assertEqual(checks["sandboxImageSourceSha"], "UNVERIFIED")

    def test_real_preflight_never_opens_a_provider_connection(self) -> None:
        with patch("socket.create_connection") as create_connection, patch("socket.socket.connect") as connect:
            report = validate_real_preflight(
                "http://ollama.invalid:11434", "qwen2.5:7b", "translation-srt", SHA, RUN_ID,
            )
        create_connection.assert_not_called()
        connect.assert_not_called()
        self.assertFalse(report["providerContacted"])
        self.assertEqual(report["result"], "BLOCKED")

    def test_real_execution_mode_and_admission_overrides_are_not_exposed(self) -> None:
        parser = build_parser()
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            parser.parse_args(["--mode", "run-real"])
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            parser.parse_args(["--mode", "real-preflight", "--authorize-real"])
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            parser.parse_args(["--mode", "real-preflight", "--admission-status", "ADMITTED"])


if __name__ == "__main__":
    unittest.main()
