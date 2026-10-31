# syntax=docker/dockerfile:1

# ---------- web ----------
FROM node:24-alpine AS web
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@12.4.2 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web/package.json ./web/package.json
RUN pnpm install --frozen-lockfile
COPY web/ ./web/
RUN pnpm --filter myvideostation-web build

# ---------- go ----------
FROM golang:1.27-alpine AS build
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -tags embedui -trimpath \
      -ldflags "-s -w \
        -X github.com/liyu1981/myvideostation/internal/buildinfo.Version=${VERSION} \
        -X github.com/liyu1981/myvideostation/internal/buildinfo.Commit=${COMMIT} \
        -X github.com/liyu1981/myvideostation/internal/buildinfo.Date=${BUILD_DATE}" \
      -o /out/mvs ./cmd/mvs

# ---------- runtime ----------
FROM alpine:3.23
RUN apk add --no-cache ffmpeg tzdata ca-certificates tini wget \
 && addgroup -S mvs \
 && adduser -S -G mvs -h /data mvs \
 && mkdir -p /data/cache \
 && chown -R mvs:mvs /data

COPY --from=build /out/mvs /usr/local/bin/mvs

USER mvs
ENV MVS_LISTEN=0.0.0.0:6500 \
    MVS_DATA_DIR=/data \
    MVS_DB=/data/mvs.db \
    MVS_LOG_FORMAT=json

VOLUME ["/data"]
EXPOSE 6500

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s \
  CMD wget -qO- http://127.0.0.1:6500/api/v1/health >/dev/null || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/mvs"]
