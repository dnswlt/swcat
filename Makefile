SHELL := /bin/sh

DOCKER := $(shell command -v docker)
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

.PHONY: test test-integration test-race build build-web

#
# Building
#

build:
	$(GO) build $(LDFLAGS) -o swcat ./cmd/swcat

build-web:
	npm run build --prefix web

run-examples:
	$(GO) run $(LDFLAGS) ./cmd/swcat -addr localhost:9191 -root-dir . -config examples/flights/swcat.yml -base-dir . -catalog-dir examples/flights/catalog

run-examples-git:
	$(GO) run $(LDFLAGS) ./cmd/swcat -addr localhost:9191 -git-url . -git-ref feature/gitclient -config examples/flights/swcat.yml -catalog-dir examples/flights/catalog

#
# Testing
#

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

# Build and run integration tests, no caching.
test-integration:
	$(GO) test $(GOTESTFLAGS) -tags=integration -count=1 -race ./...

#
# Run with Docker compose
#

DC := $(DOCKER) compose -f compose.yml

.PHONY: build start stop

docker-build:
	VERSION=$(VERSION) $(DC) build swcat

docker-up:
	VERSION=$(VERSION) $(DC) up swcat

docker-stop:
	$(DC) stop swcat
