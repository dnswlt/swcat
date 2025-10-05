GO ?= go

.PHONY: test test-integration test-race

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

# Build and run integration tests, no caching.
test-integration:
	$(GO) test $(GOTESTFLAGS) -tags=integration -count=1 -race ./...
