# Third-Party Software and License Notices

This document records the main third-party software that MVForge deliberately
builds, bundles, or directly depends on. Third-party components remain under
their own licenses.

The tables below summarize the principal components; they are not a complete,
release-specific license-text bundle. The resolved backend module graph is
described by `backend/go.mod` and `backend/go.sum`. The resolved frontend
package graph, versions, and package-declared license fields are in
`frontend/package-lock.json`. Package metadata is an inventory input, not a
substitute for reviewing the component's license and required notices.

## Media processing stack

| Component | Version / source | License family | MVForge use |
| --- | --- | --- | --- |
| FFmpeg | 8.1.2 | GPLv3 for the MVForge build | Built with `--enable-gpl --enable-version3`; invoked as a separate executable |
| x264 | Debian `libx264-164` package | GPL-2+ | Dynamically linked into FFmpeg; see the package copyright file |
| x265 | Debian `libx265-215` package | GPL-2+ | Dynamically linked into FFmpeg; see the package copyright file |
| SVT-AV1 | 4.1.0 | BSD-2-Clause + patent notice | Built as a shared library; license and patent files are copied into the image |
| libaom | Debian package | BSD-2-Clause | FFmpeg codec library |
| dav1d | Debian package | BSD-2-Clause | FFmpeg decoder library |
| libvpx | Debian package | BSD-3-Clause | FFmpeg codec library |
| libass | Debian package | ISC-style | Subtitle rendering |
| libbluray | Debian package | LGPL-2.1+ | Blu-ray structure parsing; MVForge does not bundle libaacs/libbdplus |
| libmp3lame | Debian package | LGPL-2+ (with file-specific notices) | Audio encoding |
| libopus | Debian package | BSD-3-Clause (with file-specific notices) | Audio codec |
| libvorbis | Debian package | BSD-3-Clause (with file-specific notices) | Audio codec |
| libsoxr | Debian package | LGPL-2.1+ (with file-specific notices) | Resampling |
| OpenJPEG | Debian package | Mixed package notices | JPEG 2000 support |

### FFmpeg licensing note

The MVForge FFmpeg build explicitly enables:

```text
--enable-gpl
--enable-version3
--enable-libx264
--enable-libx265
```

It does **not** enable `--enable-nonfree`. Official FFmpeg licensing
information: https://ffmpeg.org/doxygen/trunk/md_LICENSE.html

## Intel hardware acceleration stack

MVForge builds the Intel media stack from pinned commits in
`backend/Dockerfile`:

- libva
- libva-utils
- gmmlib
- Intel media-driver
- oneVPL / libvpl
- libvpl-tools
- oneVPL Intel GPU runtime

License terms and required notices are specific to each upstream component and
revision. The backend image copies license files for libva, libva-utils,
gmmlib, media-driver, libvpl, and libvpl-tools. The captured issue #57 build
did not contain a distinct license/notice file for the oneVPL Intel GPU
runtime in `/usr/share/licenses/mvforge`; see [the technical audit](docs/compliance/redistribution-audit.md).

## Subtitle and OCR stack

| Component | Version / source | License family | MVForge use |
| --- | --- | --- | --- |
| Subtitle Edit SeConv | 5.1.0-rc15 | MIT (as declared in the inventory) | Subtitle conversion helper; no separate license text is copied to the runtime license directory |
| SkiaSharp native assets | 3.119.4 | MIT + bundled third-party notices (as declared in the inventory) | Native rendering dependency; no separate notices are copied to the runtime license directory |
| HarfBuzzSharp native assets | 8.3.1.5 | MIT wrapper + HarfBuzz notices (as declared in the inventory) | Text shaping dependency; no separate notices are copied to the runtime license directory |
| Tesseract OCR | Debian package | Apache-2.0 | OCR |
| Hunspell + dictionaries | Debian packages | Package-specific open-source terms | Language/spell resources |

## Application dependencies

Primary direct dependencies include:

- Gin and gin-contrib/cors — MIT
- GORM and the GORM SQLite driver — MIT
- React / React DOM — MIT
- Material UI — MIT
- TanStack Query — MIT
- React Router — MIT

Transitive dependencies are not re-licensed by MVForge. The container release
workflow requests SBOM and provenance attestations from BuildKit for each
published image; retain those attestations with the exact image digest and
review any license texts or source obligations they identify. A package list
alone does not satisfy notice or corresponding-source obligations.

## Debian package notices

The backend image is based on Debian and installs runtime packages with APT.
Debian package copyright/license metadata remains available under
`/usr/share/doc/<package>/copyright` when supplied by the package.

Because APT package versions are not currently pinned to an immutable Debian
snapshot, each release must retain an SBOM/package inventory for the exact
image that was produced.

## Source and redistribution

For GPL-covered binaries distributed by MVForge, releases must preserve the
license notices and corresponding-source/build information required by their
licenses. Public release publication remains gated while that process is
being finalized.

Third-party names and trademarks belong to their respective owners.
