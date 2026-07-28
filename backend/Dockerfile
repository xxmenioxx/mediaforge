FROM golang:1.25-trixie AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/mvforge-api ./cmd/api

FROM debian:trixie-slim AS ffmpeg

ARG FFMPEG_VERSION=8.1.2
ARG FFMPEG_SHA256=464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c
ARG SVT_AV1_VERSION=4.1.0
ARG SVT_AV1_SHA256=6c4c0c44ff0ba3d136d6f57f3a707f9de8e9c866f50f809c1d22a43f0d8c9583

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential ca-certificates cmake curl nasm ninja-build pkg-config xz-utils yasm \
       libaom-dev libass-dev libbluray-dev libdav1d-dev libdrm-dev \
       libfontconfig1-dev libfreetype6-dev libfribidi-dev libmp3lame-dev \
       libopenjp2-7-dev libopus-dev libsoxr-dev libva-dev \
       libvpl-dev libvorbis-dev libvpx-dev libwebp-dev libx264-dev \
       libx265-dev libzimg-dev ocl-icd-opencl-dev \
    && curl -fsSL -o /tmp/svt-av1.tar.gz \
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
    && curl -fsSL -o /tmp/ffmpeg.tar.xz \
       "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
    && echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c - \
    && mkdir -p /src \
    && tar -xJf /tmp/ffmpeg.tar.xz -C /src --strip-components=1 \
    && cd /src \
    && PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig ./configure \
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
       --enable-vaapi \
       --enable-libdrm \
       --enable-opencl \
    && make -j"$(nproc)" \
    && make install \
    && LD_LIBRARY_PATH=/opt/ffmpeg/lib /opt/ffmpeg/bin/ffmpeg -hide_banner -buildconf 2>&1 | grep -F -- "--enable-libvpl" \
    && if LD_LIBRARY_PATH=/opt/ffmpeg/lib /opt/ffmpeg/bin/ffmpeg -hide_banner -buildconf 2>&1 | grep -Fq -- "--enable-libmfx"; then \
         echo "FFmpeg unexpectedly linked the legacy Media SDK runtime" >&2; exit 1; \
       fi \
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
       libopus0 libsoxr0 libsqlite3-0 libva2 libva-drm2 libvpl2 \
       libvorbis0a libvorbisenc2 libvpx9 libwebp7 libx264-164 libx265-215 \
       libzimg2 ocl-icd-libopencl1 sqlite3 tesseract-ocr \
       tesseract-ocr-eng tesseract-ocr-jpn tesseract-ocr-spa tzdata \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
         apt-get install -y --no-install-recommends \
           intel-media-va-driver i965-va-driver libmfx-gen1.2 libvpl-tools; \
       fi \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/mvforge-api /usr/local/bin/mvforge-api
COPY --from=ffmpeg /opt/ffmpeg/bin/ /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/lib/ /opt/ffmpeg/lib/
COPY --from=seconv /usr/local/bin/seconv /usr/local/bin/seconv
COPY --from=seconv /out/native/ /usr/local/lib/

RUN ldconfig

ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib:/usr/local/lib
ENV PATH=/opt/ffmpeg/bin:${PATH}

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
