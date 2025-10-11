SHELL := /bin/sh

DOCKER := $(shell command -v docker)
GO ?= go

.PHONY: test test-integration test-race

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
	$(DC) build

docker-up:
	$(DC) up -d

docker-stop:
	$(DC) stop
