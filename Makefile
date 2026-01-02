SHELL := /bin/sh

DOCKER := $(shell command -v docker)
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

.PHONY: test test-integration test-race build

#
# Building
#

build:
	$(GO) build $(LDFLAGS) -o swcat ./cmd/swcat

run-examples:
	$(GO) run $(LDFLAGS) ./cmd/swcat -addr localhost:9191 -config examples/config/swcat.yml -base-dir . examples/flights

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
	VERSION=$(VERSION) $(DC) build

docker-up:
	VERSION=$(VERSION) $(DC) up

docker-stop:
	$(DC) stop
