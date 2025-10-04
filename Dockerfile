# --- Stage 1: build web assets ---
FROM node:20-alpine AS webbuilder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
COPY templates /app/templates
# outputs to /app/static/dist (per vite.config.js outDir: ../static/dist)
RUN npm run build

# --- Stage 2: build Go binary ---
FROM golang:1.24.5-alpine AS gobuilder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /out/swcat ./cmd/swcat

# --- Stage 3: runtime image ---
FROM alpine:3.21
WORKDIR /app

# Install graphviz (swcat needs the dot tool)
RUN apk add --no-cache graphviz fontconfig ttf-dejavu

# binary
COPY --from=gobuilder /out/swcat /app/swcat

# static base (icons, third-party JS, etc.) and templates from repo
COPY static /app/static
COPY templates /app/templates

# overlay built frontend (dist) produced by Vite
COPY --from=webbuilder /app/static/dist /app/static/dist

# runtime mount for catalog input
VOLUME ["/catalog"]

EXPOSE 8080
CMD ["/app/swcat", "-addr", "0.0.0.0:8080", "/catalog"]
