#!/usr/bin/env python3
"""Run real MVForge bitmap OCR against disposable French/Korean PGS fixtures."""

from __future__ import annotations

import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


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


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="already-built supported MVForge backend image tag")
    args = parser.parse_args()

    repository = Path(__file__).resolve().parents[2]
    fixtures = repository / "backend/internal/handlers/testdata/issue-419"
    manifest = fixtures / "manifest.json"
    if not manifest.is_file():
        print(f"missing fixture manifest: {manifest}", file=sys.stderr)
        return 2

    import json

    metadata = json.loads(manifest.read_text(encoding="utf-8"))
    for relative, expected in metadata["sha256"].items():
        fixture = fixtures / relative
        if not fixture.is_file():
            print(f"missing synthetic fixture: {fixture}", file=sys.stderr)
            return 2
        observed = sha256(fixture)
        if observed != expected:
            print(f"fixture hash mismatch: {relative}: {observed} != {expected}", file=sys.stderr)
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
        media = mount_root / "two-bitmap-tracks.mkv"
        shutil.copy2(fixtures / media.name, media)
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

        command = [
            "go", "test", "./internal/handlers",
            "-run", "^TestFrenchKoreanBitmapOCRContainerE2E$",
            "-count=1", "-v",
        ]
        print(f"running real container OCR with image {args.image}", flush=True)
        print(f"fixture SHA-256 verified: {sha256(media)}", flush=True)
        result = subprocess.run(command, cwd=repository / "backend", env=environment, check=False)
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
