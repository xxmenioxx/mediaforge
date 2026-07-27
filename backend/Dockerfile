FROM golang:1.25-bookworm AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/mvforge-api ./cmd/api

FROM debian:bookworm-slim AS seconv

ARG TARGETARCH
ARG SECONV_VERSION=5.1.0-rc15
ARG SECONV_SHA256_AMD64=e06126665545979edf2d4e859b135008c4e25458c629d29723d61d4ccca849e7
ARG SECONV_SHA256_ARM64=beeb323a53119d8f7719fd8abf43143287fbce202d4b58eeb1f7a7c42ed964d5

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && case "${TARGETARCH}" in \
         amd64) archive="SeConv-Linux-x64.tar.gz"; checksum="${SECONV_SHA256_AMD64}" ;; \
         arm64) archive="SeConv-Linux-ARM64.tar.gz"; checksum="${SECONV_SHA256_ARM64}" ;; \
         *) echo "Unsupported SeConv architecture: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && curl -fsSL -o /tmp/seconv.tar.gz "https://github.com/SubtitleEdit/subtitleedit/releases/download/v${SECONV_VERSION}/${archive}" \
    && echo "${checksum}  /tmp/seconv.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/seconv.tar.gz -C /usr/local/bin ./seconv \
    && chmod 0755 /usr/local/bin/seconv

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl ffmpeg libsqlite3-0 sqlite3 tesseract-ocr \
       tesseract-ocr-eng tesseract-ocr-jpn tesseract-ocr-spa tzdata \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
         apt-get install -y --no-install-recommends intel-media-va-driver i965-va-driver; \
       fi \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/mvforge-api /usr/local/bin/mvforge-api
COPY --from=seconv /usr/local/bin/seconv /usr/local/bin/seconv

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health >/dev/null || exit 1

CMD ["mvforge-api"]
