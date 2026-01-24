SHELL := /bin/sh

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

.PHONY: test test-integration test-race build build-web build-windows release-windows run-examples run-examples-git

#
# Building
#

build:
	$(GO) build $(LDFLAGS) -o swcat ./cmd/swcat

build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o swcat.exe ./cmd/swcat
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o swcat-launcher.exe ./cmd/launcher

build-web:
	npm run build --prefix web

release-windows:
	./scripts/build-release-windows.sh

#
# Running
#

# Run swcat using the ./examples/flights catalog.
run-examples:
	$(GO) run $(LDFLAGS) ./cmd/swcat \
		-addr localhost:9191 \
		-root-dir ./examples/flights

# Run swcat using the remote git repo's ./examples/flights catalog.
run-examples-git:
	$(GO) run $(LDFLAGS) ./cmd/swcat \
		-addr localhost:9191 \
		-git-url https://github.com/dnswlt/swcat.git \
		-git-ref main \
		-config examples/flights/swcat.yml \
		-catalog-dir examples/flights/catalog

#
# Testing
#

test:
	$(GO) test ./...

# Build and run integration tests, no caching.
test-integration:
	$(GO) test $(GOTESTFLAGS) -tags=integration -count=1 -race ./...

#
# Run with Docker compose
#

UNAME_S := $(shell uname -s)
# macOS (Homebrew/Colima): Uses the dashed binary (even in newer versions like 5.0.x)
ifeq ($(UNAME_S),Darwin)
    DC_CMD := docker-compose
# Linux: Use compose via Docker Plugin (the "modern" way)
else
    DC_CMD := docker compose
endif

DC := $(DC_CMD) -f compose.yml

.PHONY: docker-build docker-up docker-stop

docker-build:
	VERSION=$(VERSION) $(DC) build swcat

docker-up:
	VERSION=$(VERSION) $(DC) up swcat

docker-stop:
	$(DC) stop swcat
