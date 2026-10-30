SHELL := /usr/bin/env bash

BIN      := bin/mvs
PKG      := github.com/liyu1981/myvideostation
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w \
  -X $(PKG)/internal/buildinfo.Version=$(VERSION) \
  -X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
  -X $(PKG)/internal/buildinfo.Date=$(DATE)

TOOLS    := .tools
FFMPEG_URL ?= https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz

.PHONY: all build web dev test lint fmt deps clean docker docker-run tidy

all: build

## deps: download a static ffmpeg/ffprobe into .tools (dev machines only)
deps: $(TOOLS)/ffprobe

$(TOOLS):
	mkdir -p $(TOOLS)

$(TOOLS)/ffprobe: | $(TOOLS)
	@echo ">> downloading static ffmpeg (dev only)"
	curl -fsSL $(FFMPEG_URL) -o $(TOOLS)/ffmpeg.tar.xz
	tar -xJf $(TOOLS)/ffmpeg.tar.xz -C $(TOOLS) --strip-components=1 --wildcards '*/ffmpeg' '*/ffprobe'
	rm -f $(TOOLS)/ffmpeg.tar.xz
	@$(TOOLS)/ffprobe -version | head -1

## web: install and build the SPA into internal/web/dist
web:
	cd web && pnpm install
	cd web && pnpm build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -R web/dist/. internal/web/dist/

## build: single static binary with the SPA embedded
build: web
	CGO_ENABLED=0 go build -tags embedui -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/mvs
	@echo ">> built $(BIN) ($(VERSION))"

## dev: Go API on :8080 + Vite on :5173
dev:
	./scripts/dev.sh

## test: Go tests (naming parser corpus, API, store)
test:
	go test ./...

## lint: formatting + vet
lint:
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt needed:"; gofmt -l ./cmd ./internal; exit 1)
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

clean:
	rm -rf bin internal/web/dist
	rm -f data/mvs.db data/mvs.db-wal data/mvs.db-shm

docker:
	docker build -f deployments/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t myvideostation:$(VERSION) .

docker-run:
	docker run --rm -p 8080:8080 -v mvs-data:/data \
		-e MVS_LISTEN=0.0.0.0:8080 \
		myvideostation:$(VERSION)
