FROM golang:1.25-trixie AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/mvforge-api ./cmd/api \
    && CGO_ENABLED=0 go build -o /out/mvforge-hw-probe ./cmd/hwprobe

FROM debian:trixie-slim AS intel-media

ARG LIBVA_COMMIT=217da1c28336d6a7e9c0c4cb8f1c303968a675f1
ARG LIBVA_UTILS_COMMIT=1a13ae13382b7b548f3a7e8035e1d7df66662c0a
ARG GMMLIB_COMMIT=acee26b150043058f722920c8090187426e4abba
ARG MEDIA_DRIVER_COMMIT=c6ab0c941edfedf3ce79fa4dd7756f3369902e8d
ARG LIBVPL_COMMIT=c45b5d786bf7cdabbe49ff1bab78693ad78feb78
ARG LIBVPL_TOOLS_COMMIT=ad9deecaf0ee76f689373b55fe620c843e3c8a4d
ARG VPL_GPU_COMMIT=e61f5633b0afee74639285e8d201fb803aeacb2f

ENV INTEL_PREFIX=/opt/intel
ENV INTEL_LIBDIR=/opt/intel/lib/x86_64-linux-gnu
ENV PKG_CONFIG_PATH=/opt/intel/lib/x86_64-linux-gnu/pkgconfig:/opt/intel/lib/pkgconfig
ENV CMAKE_PREFIX_PATH=/opt/intel
ENV LD_LIBRARY_PATH=/opt/intel/lib/x86_64-linux-gnu:/opt/intel/lib

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential ca-certificates cmake git git-lfs meson ninja-build pkg-config \
       autoconf automake libtool libdrm-dev libx11-dev libxext-dev libxfixes-dev \
       wayland-protocols libwayland-dev python3 \
    && git lfs install --system \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --filter=blob:none https://github.com/intel/libva.git /src/libva \
    && cd /src/libva && git checkout "${LIBVA_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${LIBVA_COMMIT}" \
    && meson setup build --prefix="${INTEL_PREFIX}" --libdir="lib/x86_64-linux-gnu" \
       -Ddriverdir="${INTEL_LIBDIR}/dri" -Dwith_x11=no -Dwith_wayland=no \
    && meson compile -C build \
    && meson install -C build

RUN git clone --filter=blob:none https://github.com/intel/libva-utils.git /src/libva-utils \
    && cd /src/libva-utils && git checkout "${LIBVA_UTILS_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${LIBVA_UTILS_COMMIT}" \
    && meson setup build --prefix="${INTEL_PREFIX}" --libdir="lib/x86_64-linux-gnu" \
    && meson compile -C build \
    && meson install -C build

RUN git clone --filter=blob:none https://github.com/intel/gmmlib.git /src/gmmlib \
    && cd /src/gmmlib && git checkout "${GMMLIB_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${GMMLIB_COMMIT}" \
    && cmake -S . -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="${INTEL_PREFIX}" \
       -DCMAKE_INSTALL_LIBDIR="lib/x86_64-linux-gnu" \
    && cmake --build build --parallel "$(nproc)" \
    && cmake --install build

RUN git clone --filter=blob:none https://github.com/intel/media-driver.git /src/media-driver \
    && cd /src/media-driver && git checkout "${MEDIA_DRIVER_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${MEDIA_DRIVER_COMMIT}" \
    && cmake -S . -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="${INTEL_PREFIX}" \
       -DCMAKE_INSTALL_LIBDIR="lib/x86_64-linux-gnu" \
       -DMEDIA_VERSION="25.2.6" \
    && cmake --build build --parallel "$(nproc)" \
    && cmake --install build

RUN git clone --filter=blob:none https://github.com/intel/libvpl.git /src/libvpl \
    && cd /src/libvpl && git checkout "${LIBVPL_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${LIBVPL_COMMIT}" \
    && cmake -S . -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="${INTEL_PREFIX}" \
       -DCMAKE_INSTALL_LIBDIR="lib/x86_64-linux-gnu" \
       -DBUILD_TOOLS=ON -DBUILD_EXAMPLES=OFF -DBUILD_TESTS=OFF \
    && cmake --build build --parallel "$(nproc)" \
    && cmake --install build

RUN git clone --filter=blob:none https://github.com/intel/libvpl-tools.git /src/libvpl-tools \
    && cd /src/libvpl-tools && git checkout "${LIBVPL_TOOLS_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${LIBVPL_TOOLS_COMMIT}" \
    && cmake -S . -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release -DCMAKE_PREFIX_PATH="${INTEL_PREFIX}" \
       -DTOOLS_ENABLE_SCREEN_CAPTURE=OFF -DTOOLS_ENABLE_RENDER=OFF \
       -DTOOLS_ENABLE_OPENCL=OFF -DTOOLS_ENABLE_X11=OFF \
       -DBUILD_EXPERIMENTAL=OFF -DBUILD_TESTS=OFF \
    && cmake --build build --target vpl-inspect --parallel "$(nproc)" \
    && install -m 0755 build/vpl-inspect "${INTEL_PREFIX}/bin/vpl-inspect"

RUN git clone https://github.com/oneapi-src/oneVPL-intel-gpu.git /src/vpl-gpu \
    && cd /src/vpl-gpu && git checkout "${VPL_GPU_COMMIT}" \
    && test "$(git rev-parse HEAD)" = "${VPL_GPU_COMMIT}" \
    && git lfs pull \
    && cmake -S . -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="${INTEL_PREFIX}" \
       -DCMAKE_INSTALL_LIBDIR="lib/x86_64-linux-gnu" \
       -DBUILD_RUNTIME=ON -DBUILD_TOOLS=OFF -DBUILD_TESTS=OFF \
    && cmake --build build --parallel "$(nproc)" \
    && cmake --install build \
    && test -f "${INTEL_LIBDIR}/dri/iHD_drv_video.so" \
    && test -x "${INTEL_PREFIX}/bin/vpl-inspect" \
    && find "${INTEL_PREFIX}" -type f \( -name 'libvpl.so*' -o -name 'libmfx-gen.so*' \) | grep .

FROM debian:trixie-slim AS ffmpeg

ARG FFMPEG_VERSION=8.1.2
ARG FFMPEG_SHA256=464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c
ARG SVT_AV1_VERSION=4.1.0
ARG SVT_AV1_SHA256=6c4c0c44ff0ba3d136d6f57f3a707f9de8e9c866f50f809c1d22a43f0d8c9583

COPY --from=intel-media /opt/intel/ /opt/intel/

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential ca-certificates cmake curl nasm ninja-build pkg-config xz-utils yasm \
       libaom-dev libass-dev libbluray-dev libdav1d-dev libdrm-dev \
       libfontconfig1-dev libfreetype6-dev libfribidi-dev libmp3lame-dev \
       libopenjp2-7-dev libopus-dev libsoxr-dev \
       libvorbis-dev libvpx-dev libwebp-dev libx264-dev \
       libx265-dev libzimg-dev ocl-icd-opencl-dev \
    && curl -fsSL --retry 5 --retry-all-errors -o /tmp/svt-av1.tar.gz \
       "https://gitlab.com/AOMediaCodec/SVT-AV1/-/archive/v${SVT_AV1_VERSION}/SVT-AV1-v${SVT_AV1_VERSION}.tar.gz" \
    && echo "${SVT_AV1_SHA256}  /tmp/svt-av1.tar.gz" | sha256sum -c - \
    && mkdir -p /src/svt-av1 \
    && tar -xzf /tmp/svt-av1.tar.gz -C /src/svt-av1 --strip-components=1 \
    && cmake -S /src/svt-av1 -B /src/svt-av1/build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release \
       -DCMAKE_INSTALL_PREFIX=/opt/ffmpeg \
       -DBUILD_APPS=OFF \
       -DBUILD_TESTING=OFF \
       -DBUILD_SHARED_LIBS=ON \
    && cmake --build /src/svt-av1/build --parallel "$(nproc)" \
    && cmake --install /src/svt-av1/build \
    && curl -fsSL --retry 5 --retry-all-errors -o /tmp/ffmpeg.tar.xz \
       "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
    && echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c - \
    && mkdir -p /src \
    && tar -xJf /tmp/ffmpeg.tar.xz -C /src --strip-components=1 \
    && cd /src \
    && PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig:/opt/intel/lib/x86_64-linux-gnu/pkgconfig:/opt/intel/lib/pkgconfig \
       LD_LIBRARY_PATH=/opt/intel/lib/x86_64-linux-gnu:/opt/intel/lib ./configure \
       --prefix=/opt/ffmpeg \
       --disable-debug \
       --disable-doc \
       --disable-static \
       --enable-shared \
       --enable-gpl \
       --enable-version3 \
       --enable-libx264 \
       --enable-libx265 \
       --enable-libaom \
       --enable-libdav1d \
       --enable-libsvtav1 \
       --enable-libvpx \
       --enable-libass \
       --enable-libfreetype \
       --enable-libfontconfig \
       --enable-libfribidi \
       --enable-libmp3lame \
       --enable-libopus \
       --enable-libvorbis \
       --enable-libsoxr \
       --enable-libzimg \
       --enable-libwebp \
       --enable-libopenjpeg \
       --enable-libbluray \
       --enable-libvpl \
       --disable-libmfx \
       --enable-vaapi \
       --enable-libdrm \
       --enable-opencl \
    && make -j"$(nproc)" \
    && make install \
    && LD_LIBRARY_PATH=/opt/ffmpeg/lib:/opt/intel/lib/x86_64-linux-gnu:/opt/intel/lib \
       /opt/ffmpeg/bin/ffmpeg -hide_banner -buildconf 2>&1 | grep -F -- "--enable-libvpl" \
    && LD_LIBRARY_PATH=/opt/ffmpeg/lib:/opt/intel/lib/x86_64-linux-gnu:/opt/intel/lib \
       /opt/ffmpeg/bin/ffmpeg -hide_banner -buildconf 2>&1 | grep -F -- "--disable-libmfx" \
    && strip /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe \
    && rm -rf /var/lib/apt/lists/* /tmp/ffmpeg.tar.xz /tmp/svt-av1.tar.gz /src

FROM debian:trixie-slim AS seconv

ARG TARGETARCH
ARG SECONV_VERSION=5.1.0-rc15
ARG SECONV_SHA256_AMD64=e06126665545979edf2d4e859b135008c4e25458c629d29723d61d4ccca849e7
ARG SECONV_SHA256_ARM64=beeb323a53119d8f7719fd8abf43143287fbce202d4b58eeb1f7a7c42ed964d5
ARG SKIASHARP_VERSION=3.119.4
ARG SKIASHARP_SHA256=fae0554059b1107ef7888e46c20bdfb548401ef7a7a6f7391ad4fadc7432d50a
ARG HARFBUZZSHARP_VERSION=8.3.1.5
ARG HARFBUZZSHARP_SHA256=6b95b9cddec035d0a85accd18a29c42038ac7714d366a004494de7f0b9b66157

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && case "${TARGETARCH}" in \
         amd64) archive="SeConv-Linux-x64.tar.gz"; checksum="${SECONV_SHA256_AMD64}"; runtime="linux-x64" ;; \
         arm64) archive="SeConv-Linux-ARM64.tar.gz"; checksum="${SECONV_SHA256_ARM64}"; runtime="linux-arm64" ;; \
         *) echo "Unsupported SeConv architecture: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && curl -fsSL -o /tmp/seconv.tar.gz "https://github.com/SubtitleEdit/subtitleedit/releases/download/v${SECONV_VERSION}/${archive}" \
    && echo "${checksum}  /tmp/seconv.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/seconv.tar.gz -C /usr/local/bin ./seconv \
    && chmod 0755 /usr/local/bin/seconv \
    && curl -fsSL -o /tmp/skiasharp.nupkg "https://api.nuget.org/v3-flatcontainer/skiasharp.nativeassets.linux/${SKIASHARP_VERSION}/skiasharp.nativeassets.linux.${SKIASHARP_VERSION}.nupkg" \
    && curl -fsSL -o /tmp/harfbuzzsharp.nupkg "https://api.nuget.org/v3-flatcontainer/harfbuzzsharp.nativeassets.linux/${HARFBUZZSHARP_VERSION}/harfbuzzsharp.nativeassets.linux.${HARFBUZZSHARP_VERSION}.nupkg" \
    && echo "${SKIASHARP_SHA256}  /tmp/skiasharp.nupkg" | sha256sum -c - \
    && echo "${HARFBUZZSHARP_SHA256}  /tmp/harfbuzzsharp.nupkg" | sha256sum -c - \
    && mkdir -p /out/native \
    && unzip -p /tmp/skiasharp.nupkg "runtimes/${runtime}/native/libSkiaSharp.so" > /out/native/libSkiaSharp.so \
    && unzip -p /tmp/harfbuzzsharp.nupkg "runtimes/${runtime}/native/libHarfBuzzSharp.so" > /out/native/libHarfBuzzSharp.so \
    && chmod 0755 /out/native/*.so

FROM debian:trixie-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl libaom3 libass9 libbluray2 libdav1d7 libdrm2 \
       libfontconfig1 libfreetype6 libfribidi0 libmp3lame0 libopenjp2-7 \
       libopus0 libsoxr0 libsqlite3-0 \
       libvorbis0a libvorbisenc2 libvpx9 libwebp7 libx264-164 libx265-215 \
       libx11-6 libxext6 libxfixes3 libwayland-client0 \
       libzimg2 ocl-icd-libopencl1 sqlite3 tesseract-ocr \
       tesseract-ocr-eng tesseract-ocr-jpn tesseract-ocr-spa tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/mvforge-api /usr/local/bin/mvforge-api
COPY --from=build /out/mvforge-hw-probe /usr/local/bin/mvforge-hw-probe
COPY --from=ffmpeg /opt/ffmpeg/bin/ /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/lib/ /opt/ffmpeg/lib/
COPY --from=intel-media /opt/intel/ /opt/intel/
COPY --from=seconv /usr/local/bin/seconv /usr/local/bin/seconv
COPY --from=seconv /out/native/ /usr/local/lib/

RUN printf '%s\n' /opt/intel/lib/x86_64-linux-gnu /opt/intel/lib /opt/ffmpeg/lib /usr/local/lib \
      > /etc/ld.so.conf.d/mvforge-media.conf \
    && ldconfig

ENV LIBVA_DRIVER_NAME=iHD
ENV LIBVA_DRIVERS_PATH=/opt/intel/lib/x86_64-linux-gnu/dri
ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib:/opt/intel/lib/x86_64-linux-gnu:/opt/intel/lib:/usr/local/lib
ENV PATH=/opt/ffmpeg/bin:/opt/intel/bin:${PATH}

RUN ffmpeg -hide_banner -loglevel error \
      -f lavfi -i "testsrc2=size=720x480:rate=30000/1001:duration=1" \
      -vf "fieldmatch=order=tff,decimate,bwdif=mode=send_frame:parity=auto:deint=interlaced,crop=704:460:8:10,format=yuv420p10le" \
      -an -c:v libx265 -preset ultrafast -f null - \
    && ffmpeg -hide_banner -loglevel error \
      -f lavfi -i "testsrc2=size=320x240:rate=24:duration=0.25" \
      -an -c:v libsvtav1 -preset 12 -f null - \
    && ffmpeg -hide_banner -loglevel error \
      -f lavfi -i "sine=frequency=1000:duration=0.1" \
      -af "aresample=ochl=stereo,loudnorm=I=-18:TP=-2:LRA=11" \
      -f null -

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health >/dev/null || exit 1

CMD ["mvforge-api"]
