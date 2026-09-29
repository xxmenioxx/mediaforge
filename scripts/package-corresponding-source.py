#!/usr/bin/env python3
"""Create a deterministic gzip tar archive from a Docker-exported source tree."""

import gzip
import hashlib
import sys
import tarfile
from pathlib import Path


def fail(message: str) -> None:
    print(f"source archive failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    if len(sys.argv) != 3:
        fail("usage: package-corresponding-source.py SOURCE_DIRECTORY OUTPUT_ARCHIVE")

    source_root = Path(sys.argv[1]).resolve()
    archive_path = Path(sys.argv[2]).resolve()
    checksum_path = Path(f"{archive_path}.sha256")
    if not source_root.is_dir():
        fail(f"source directory does not exist: {source_root}")
    if archive_path.exists() or checksum_path.exists():
        fail("output archive or checksum already exists")

    checksums_path = source_root / "SHA256SUMS"
    if checksums_path.exists():
        fail("source tree already contains SHA256SUMS")
    paths = sorted(source_root.rglob("*"), key=lambda item: item.relative_to(source_root).as_posix())
    if not paths:
        fail("source tree is empty")
    for path in paths:
        if path.is_symlink():
            target = Path(path.readlink())
            if target.is_absolute():
                fail(f"absolute symlink is not permitted: {path.relative_to(source_root)}")
            try:
                path.resolve(strict=False).relative_to(source_root)
            except ValueError:
                fail(f"symlink escapes source tree: {path.relative_to(source_root)}")

    checksum_lines = []
    for path in paths:
        if not path.is_file():
            continue
        digest = hashlib.sha256()
        with path.open("rb") as content:
            for chunk in iter(lambda: content.read(1024 * 1024), b""):
                digest.update(chunk)
        checksum_lines.append(f"{digest.hexdigest()}  {path.relative_to(source_root).as_posix()}\n")
    checksums_path.write_text("".join(checksum_lines), encoding="utf-8")
    paths.append(checksums_path)
    paths.sort(key=lambda item: item.relative_to(source_root).as_posix())

    with archive_path.open("xb") as raw_archive:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw_archive, mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
                for path in paths:
                    relative = path.relative_to(source_root).as_posix()
                    info = archive.gettarinfo(str(path), arcname=relative)
                    info.uid = 0
                    info.gid = 0
                    info.uname = ""
                    info.gname = ""
                    info.mtime = 0
                    if info.isfile():
                        with path.open("rb") as file_content:
                            archive.addfile(info, file_content)
                    else:
                        archive.addfile(info)

    digest = hashlib.sha256()
    with archive_path.open("rb") as archive_content:
        for chunk in iter(lambda: archive_content.read(1024 * 1024), b""):
            digest.update(chunk)
    checksum_path.write_text(f"{digest.hexdigest()}  {archive_path.name}\n", encoding="utf-8")
    print(f"sha256:{digest.hexdigest()}")


if __name__ == "__main__":
    main()
