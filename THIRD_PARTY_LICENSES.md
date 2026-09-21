# Third-Party Software and License Notices

This document records the main third-party software that MVForge deliberately
builds, bundles, or directly depends on. Third-party components remain under
their own licenses.

## Media processing stack

| Component | Version / source | License family | MVForge use |
| --- | --- | --- | --- |
| FFmpeg | 8.1.2 | GPLv3 for the MVForge build | Built with `--enable-gpl --enable-version3`; invoked as a separate executable |
| x264 | Debian build/runtime package | GPL-compatible | Linked into the FFmpeg build |
| x265 | Debian build/runtime package | GPL-compatible | Linked into the FFmpeg build |
| SVT-AV1 | 4.1.0 | BSD-style + patent notice | Built as a shared library for FFmpeg |
| libaom | Debian package | BSD-style | FFmpeg codec library |
| dav1d | Debian package | BSD-style | FFmpeg decoder library |
| libvpx | Debian package | BSD-style | FFmpeg codec library |
| libass | Debian package | ISC-style | Subtitle rendering |
| libbluray | Debian package | LGPL | Blu-ray structure parsing; MVForge does not bundle libaacs/libbdplus |
| libmp3lame | Debian package | LGPL | Audio encoding |
| libopus | Debian package | BSD-style | Audio codec |
| libvorbis | Debian package | BSD-style | Audio codec |
| libsoxr | Debian package | LGPL | Resampling |
| OpenJPEG | Debian package | BSD-style | JPEG 2000 support |

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

These projects use permissive MIT/BSD-style terms. License material for the
custom-built components is retained in the backend image where practical.

## Subtitle and OCR stack

| Component | Version / source | License family | MVForge use |
| --- | --- | --- | --- |
| Subtitle Edit SeConv | 5.1.0-rc15 | MIT | Subtitle conversion helper |
| SkiaSharp native assets | 3.119.4 | MIT + bundled third-party notices | Native rendering dependency used by SeConv |
| HarfBuzzSharp native assets | 8.3.1.5 | MIT wrapper + HarfBuzz notices | Text shaping dependency used by SeConv |
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

Transitive dependencies are not re-licensed by MVForge. Release SBOM
attestations and package manifests should be treated as the authoritative
resolved inventory for a specific artifact.

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
