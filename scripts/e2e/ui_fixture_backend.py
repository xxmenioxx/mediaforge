#!/usr/bin/env python3
"""Serve read-only synthetic API state for the nightly browser journeys."""

from __future__ import annotations

import argparse
import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit


FIXTURE_TIME = "2026-01-01T00:00:00Z"
FIXTURE_MEDIA_PATH = "/synthetic/nightly-fixture/episode.mkv"


class FixtureState:
    scenario = "review"
    api_requests: list[dict[str, str]] = []
    unexpected: list[str] = []
    queue_mutations: list[str] = []

    @classmethod
    def reset(cls, scenario: str) -> None:
        cls.scenario = scenario
        cls.api_requests = []
        cls.unexpected = []
        cls.queue_mutations = []


def queue_job() -> dict[str, object]:
    passed = FixtureState.scenario == "passed"
    validation_status = "passed" if passed else "pending"
    validation_outcome = "PASS" if passed else "REVIEW_REQUIRED"
    run_id = "nightly-fixture-pass" if passed else "nightly-fixture-review"
    return {
        "id": 28701,
        "sourceAssetRecordId": 8701,
        "executionNumber": 1,
        "batchId": "nightly-fixture-batch",
        "batchName": "Synthetic nightly fixture",
        "batchPosition": 1,
        "mediaPath": FIXTURE_MEDIA_PATH,
        "publishMode": "standard",
        "libraryId": 1,
        "profileId": 1,
        "profileVersion": 1,
        "profileSnapshot": {},
        "audioProfileKey": "",
        "trackProfileKey": "",
        "processingMode": "full_encode",
        "priority": 0,
        "queuePosition": 1,
        "status": "completed",
        "stage": "completed",
        "stageHistory": [],
        "progress": 100,
        "workerName": "fixture-worker",
        "outputPath": "/synthetic/nightly-fixture/output.mkv",
        "plannedPublishedPath": "/synthetic/nightly-fixture/library/episode.mkv",
        "errorMessage": "",
        "notes": "Synthetic read-only browser fixture; no job was executed.",
        "validationStatus": validation_status,
        "validationScore": 100 if passed else 0,
        "validationReport": {},
        "validationRunId": run_id,
        "validationOutcome": validation_outcome,
        "publishedPath": "",
        "replacementTargetPath": "",
        "originalArchivedPath": "",
        "createdAt": FIXTURE_TIME,
        "updatedAt": FIXTURE_TIME,
        "finishedAt": FIXTURE_TIME,
    }


def asset() -> dict[str, object]:
    return {
        "id": 8701,
        "sourceGroupId": 87,
        "logicalGroupPath": "nightly-fixture",
        "sourcePath": "/synthetic/nightly-fixture",
        "libraryId": 1,
        "libraryName": "Synthetic Library",
        "path": FIXTURE_MEDIA_PATH,
        "relativePath": "episode.mkv",
        "groupPath": "/synthetic/nightly-fixture",
        "fileName": "episode.mkv",
        "extension": "mkv",
        "sizeBytes": 1024,
        "modifiedAt": FIXTURE_TIME,
        "status": "unprocessed",
        "missing": False,
        "review": {"requiresReview": False, "reason": "", "source": "", "tags": [], "updatedAt": FIXTURE_TIME},
        "metadata": {"categories": [], "tags": [], "updatedAt": FIXTURE_TIME},
        "conversion": {},
    }


def inventory() -> dict[str, object]:
    fixture_asset = asset()
    review = fixture_asset["review"]
    metadata = fixture_asset["metadata"]
    asset_path = {
        "id": "synthetic-nightly-fixture-path",
        "name": "episode.mkv",
        "path": FIXTURE_MEDIA_PATH,
        "relativePath": "episode.mkv",
        "displayPath": "episode.mkv",
        "isLogicalGroupRoot": True,
        "fileCount": 1,
        "assetCount": 1,
        "sizeBytes": 1024,
        "totalSizeBytes": 1024,
        "assets": [fixture_asset],
    }
    logical_group = {
        "id": "synthetic-nightly-fixture",
        "name": "nightly-fixture",
        "path": "/synthetic/nightly-fixture",
        "relativePath": "nightly-fixture",
        "fileCount": 1,
        "assetCount": 1,
        "pathCount": 1,
        "sizeBytes": 1024,
        "totalSizeBytes": 1024,
        "assetPaths": [asset_path],
    }
    source_group = {
        "id": 87,
        "name": "Synthetic nightly fixture",
        "relativePath": "nightly-fixture",
        "sourcePath": "/synthetic/nightly-fixture",
        "fileCount": 1,
        "assetCount": 1,
        "titleCount": 1,
        "pathCount": 1,
        "sizeBytes": 1024,
        "totalSizeBytes": 1024,
        "logicalGroups": [logical_group],
    }
    group = {
        "id": "synthetic-nightly-fixture",
        "libraryId": 1,
        "libraryName": "Synthetic Library",
        "path": "/synthetic/nightly-fixture",
        "relativePath": "nightly-fixture",
        "status": "unprocessed",
        "fileCount": 1,
        "sizeBytes": 1024,
        "modifiedAt": FIXTURE_TIME,
        "assets": [fixture_asset],
        "review": review,
        "pathReview": review,
        "metadata": metadata,
        "pathMetadata": metadata,
    }
    return {
        "sourceGroups": [source_group],
        "raw": [fixture_asset],
        "unprocessed": [fixture_asset],
        "library": [],
        "converted": [],
        "unverified": [],
        "accepted": [],
        "missing": [],
        "unprocessedGroups": [group],
        "rawGroups": [group],
        "libraryGroups": [],
        "convertedGroups": [],
        "unverifiedGroups": [],
        "acceptedGroups": [],
        "reports": {},
        "sync": {},
    }


class Handler(BaseHTTPRequestHandler):
    server_version = "MVForgeNightlyFixture/1.0"

    def log_message(self, _format: str, *_args: object) -> None:
        return

    def _headers(self) -> None:
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        origin = self.headers.get("Origin", "")
        if origin.startswith("http://127.0.0.1:"):
            self.send_header("Access-Control-Allow-Origin", origin)
            self.send_header("Vary", "Origin")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")

    def _json(self, status: int, value: object) -> None:
        body = json.dumps(value, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self._headers()
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self._headers()
        self.end_headers()

    def do_POST(self) -> None:
        parsed = urlsplit(self.path)
        if parsed.path == "/__fixture/reset":
            length = min(int(self.headers.get("Content-Length", "0")), 1024)
            try:
                payload = json.loads(self.rfile.read(length))
            except (json.JSONDecodeError, UnicodeDecodeError):
                self._json(400, {"error": "invalid fixture request"})
                return
            scenario = payload.get("scenario") if isinstance(payload, dict) else None
            if scenario not in {"review", "passed"}:
                self._json(400, {"error": "unsupported fixture scenario"})
                return
            FixtureState.reset(scenario)
            self._json(200, {"scenario": scenario})
            return

        if parsed.path.startswith("/api/"):
            FixtureState.api_requests.append({"method": "POST", "path": parsed.path})
            if parsed.path == "/api/assets/effective-configurations":
                self._json(200, {})
                return
            if parsed.path.startswith("/api/queue/") or parsed.path.startswith("/api/publisher/"):
                FixtureState.queue_mutations.append(f"POST {parsed.path}")
            FixtureState.unexpected.append(f"POST {parsed.path}")
            self._json(405, {"error": "nightly UI fixture is read-only"})
            return

        self._json(404, {"error": "not found"})

    def do_GET(self) -> None:
        parsed = urlsplit(self.path)
        path = parsed.path
        if path == "/__health":
            self._json(200, {"status": "ready"})
            return
        if path == "/__fixture/metrics":
            self._json(200, {
                "requests": FixtureState.api_requests,
                "unexpected": FixtureState.unexpected,
                "queueMutations": FixtureState.queue_mutations,
            })
            return

        if path.startswith("/api/"):
            FixtureState.api_requests.append({"method": "GET", "path": path})
        job = queue_job()
        if path == "/api/settings":
            value: object = []
        elif path == "/api/queue/jobs":
            value = [job]
        elif path == "/api/assets":
            value = inventory()
        elif path in {"/api/libraries", "/api/profiles", "/api/profile-assignments", "/api/asset-scope-configurations"}:
            value = []
        elif path == "/api/system/runtime":
            value = {"status": "ready", "workers": []}
        elif path == "/api/scan":
            value = {"found": False, "snapshot": None, "status": "missing", "requiresAnalysis": True, "staleComponents": []}
        elif path == "/api/scan/operations":
            value = {"operations": []}
        elif re.fullmatch(r"/api/assets/\d+/thumbnail", path):
            self._json(404, {"error": "synthetic fixture has no source thumbnail"})
            return
        elif path == "/api/validation/review":
            query = parse_qs(parsed.query)
            value = {"items": [job], "total": 1, "limit": 25, "offset": int((query.get("offset") or ["0"])[0])}
        elif path == "/api/publisher/eligibility":
            passed = FixtureState.scenario == "passed"
            candidate = {"job": job, "status": "ALLOWED"}
            value = {"items": [candidate] if passed else [], "limit": 100}
        elif path == "/api/publisher/publications":
            value = {"items": [], "limit": 100}
        else:
            if path.startswith("/api/"):
                FixtureState.unexpected.append(f"GET {path}")
            self._json(404 if path.startswith("/api/") else 404, {"error": "fixture route not found"})
            return
        self._json(200, value)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    args = parser.parse_args()
    if args.host not in {"127.0.0.1", "localhost"}:
        parser.error("fixture backend must bind to loopback")
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
