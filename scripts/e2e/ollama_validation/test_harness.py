from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from ollama_validation_sandbox import HarnessError, validate_compose_config, validate_real_plan


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
        plan = validate_real_plan("https://ollama.example.invalid:11434", "qwen2.5:7b")
        self.assertEqual(plan["result"], "PASS")
        self.assertFalse(plan["executionAllowed"])
        self.assertFalse(plan["realInferenceExecuted"])
        self.assertFalse(plan["checks"]["simulatedAdmissionAuthorizesRealInference"])
        for endpoint in ("http://user:secret@ollama.invalid:11434", "https://ollama.invalid/api/chat", "file:///tmp/model"):
            with self.subTest(endpoint=endpoint), self.assertRaises(HarnessError):
                validate_real_plan(endpoint, "qwen2.5:7b")
        with self.assertRaises(HarnessError):
            validate_real_plan("http://ollama.invalid:11434", " qwen2.5:7b ")


if __name__ == "__main__":
    unittest.main()
