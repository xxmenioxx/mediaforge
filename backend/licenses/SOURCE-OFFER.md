# Backend image corresponding-source bundle

For each future versioned container release, `scripts/build-corresponding-source.sh`
builds `mvforge-corresponding-source.tar.gz` from the release commit's backend
Dockerfile. The same archive contains the pinned FFmpeg and SVT-AV1 source
archives; the exact Debian x264/x265 and zimg sources; and the exact Debian
libbluray, FriBidi, LAME, and SoXR sources used for pinned GPL/LGPL runtime
libraries. It also
contains the build Dockerfile, the versioned source-build and archive-packaging
scripts, backend license inventory, and runtime notice files. `SHA256SUMS`
covers the archive contents; a sidecar checksum covers the archive itself.

The backend image is built with FFmpeg 8.1.2 (`464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c`)
and SVT-AV1 4.1.0 (`6c4c0c44ff0ba3d136d6f57f3a707f9de8e9c866f50f809c1d22a43f0d8c9583`).
The x264 binary package is pinned to
`2:0.164.3108+git31e19f9-2+b1` with source package
`2:0.164.3108+git31e19f9-2`; x265 is pinned to `4.1-2` for both binary and
source. The libbluray package is pinned to `1:1.3.4-1+b2` with source package
`1:1.3.4-1`; LAME is pinned to `3.100-6+b3` with source package `3.100-6`;
SoXR is pinned to `0.1.3-4+b2` with source package `0.1.3-4`. The same binary
package versions are pinned in the FFmpeg build and backend runtime stages.
FriBidi is pinned to `1.0.16-1` (source `1.0.16-1`), and zimg is pinned to
`3.0.5+ds1-1+b2` (source `3.0.5+ds1-1`).
Dockerfile arguments and archive checks are the authoritative build inputs;
this file is a readable index.

The source target also retains the exact Intel source commit references in
`backend/Dockerfile`, while the Intel component license files are copied from
those same checkouts. The source archive is a separate release asset to avoid
adding compiler sources to the runtime image.

The release workflow must attach the matching archive and its SHA256 file to
the same GitHub Release as the backend/web image digest list and license
inventories. Its version tag identifies the source commit; the digest list
binds the distributed image to that release evidence. Until those assets are
generated, attached, and independently checked, the public release gate must
remain closed. This document describes the process and is not itself a source
offer for an already-distributed image.
