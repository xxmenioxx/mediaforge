FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache ffmpeg gcc musl-dev sqlite-dev

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /out/mediaforge-api ./cmd/api

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ffmpeg sqlite-libs

COPY --from=build /out/mediaforge-api /usr/local/bin/mediaforge-api

EXPOSE 8080

CMD ["mediaforge-api"]
