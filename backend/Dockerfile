FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache ffmpeg gcc musl-dev sqlite-dev

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/mvforge-api ./cmd/api

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ffmpeg sqlite \
    && if [ "$(apk --print-arch)" = "x86_64" ]; then \
         apk add --no-cache intel-media-driver intel-media-sdk; \
       fi

COPY --from=build /out/mvforge-api /usr/local/bin/mvforge-api

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1

CMD ["mvforge-api"]
