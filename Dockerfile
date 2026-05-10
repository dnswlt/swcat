# --- Base Image Definitions ---
ARG NODE_BASE=node:20-alpine
ARG GO_BASE=golang:1.25.7-alpine
ARG RUNTIME_BASE=alpine:3.21

# --- Stage 1: build web assets ---
FROM ${NODE_BASE} AS webbuilder
WORKDIR /app/web
# Cache npm dependencies
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY web/ .
COPY templates /app/templates
# outputs to /app/static/dist (per vite.config.js outDir: ../static/dist)
RUN npm run build

# --- Stage 2: Build Go binary ---
FROM ${GO_BASE} AS gobuilder
WORKDIR /app

# 1. Cache modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 2. Copy source (for building) and web assets (for embedding)
COPY cmd ./cmd
COPY templates ./templates
COPY internal ./internal
COPY *.go ./
COPY --from=webbuilder /app/static/dist ./static/dist

# 3. Build with persistent cache mounts
# This keeps the Go build cache (GOCACHE) across Docker builds
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o /out/swcat ./cmd/swcat

# --- Stage 3: Runtime image ---
FROM ${RUNTIME_BASE}
WORKDIR /app

# Install graphviz (swcat needs the dot tool) plus Liberation Sans, which is
# the font graphviz uses for SVG layout. The browser falls back to Arial (which
# is metric-compatible with Liberation Sans) when Liberation Sans isn't available.
RUN apk add --no-cache graphviz fontconfig ttf-liberation

# binary
COPY --from=gobuilder /out/swcat /app/swcat

# runtime mount for catalog input
VOLUME ["/data"]
EXPOSE 8080
CMD ["/app/swcat"]
