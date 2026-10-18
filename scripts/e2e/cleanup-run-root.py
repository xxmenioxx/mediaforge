#!/usr/bin/env python3
"""Remove only a pipeline-smoke root created under the approved temp directory."""

from __future__ import annotations

import argparse
import os
import shutil
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from pipeline_smoke import validate_temp_base


PREFIX = "mvforge-e2e-"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--temp-base")
    args = parser.parse_args()
    if not args.run_id or any(char not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-" for char in args.run_id):
        parser.error("run id contains unsupported characters")

    if args.temp_base:
        base = validate_temp_base(args.temp_base)
    elif os.environ.get("GITHUB_ACTIONS", "").lower() == "true" and os.environ.get("RUNNER_TEMP"):
        base = validate_temp_base(os.environ["RUNNER_TEMP"])
    else:
        base = validate_temp_base(tempfile.gettempdir())

    root = base / f"{PREFIX}{args.run_id}"
    if not root.exists():
        return 0
    if root.is_symlink() or root.resolve(strict=True).parent != base or not root.name.startswith(PREFIX):
        parser.error("refusing to remove a path outside the generated run-root boundary")
    shutil.rmtree(root)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
