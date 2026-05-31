# Developer Information

This file contains a collection of information relevant to developers of `swcat`.

## Building and running locally

This section walks you from a fresh checkout to a running dev server.
For a more user-focused guide (including Docker), see
<https://dnswlt.github.io/swcat/getting-started/>
(or, locally, [docs/getting-started.md](docs/getting-started.md)).

### Prerequisites

* [Go](https://go.dev/) (>= 1.24.5)
* `npm` (e.g. via [nvm](https://github.com/nvm-sh/nvm)) — needed to build the frontend assets
* [Graphviz](https://graphviz.org/download/) — used to render catalog graphs

### Build the frontend assets

The Go server embeds the compiled frontend (Tailwind CSS, JS bundles), so you
must build the web assets before running or building the server:

```bash
make build-web
```

This runs `npm run build` in `web/`. On the very first run, install the npm
dependencies first:

```bash
npm install --prefix web
```

### Run the dev server

Once the web assets are built, start the server against the bundled
`./examples/flights` catalog:

```bash
make run-examples
```

Then point your browser at <http://localhost:9191> and explore the example
catalog.

### Build a binary

To compile a standalone `swcat` binary (with the version stamped in):

```bash
make build
```

This produces a `swcat` executable in the repo root. Remember to run
`make build-web` first whenever the frontend assets have changed.

## Updating documentation

Documentation is generated using [mkdocs](https://www.mkdocs.org/). 

To run the user guide locally with live reloading:

```bash
.venv/bin/mkdocs serve -w ./docs --livereload
```

If you don't have the virtual environment set up yet, you can create it and install the requirements:

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install mkdocs mkdocs-material
```

## Protocol Buffers

The catalog model and plugin protocols are defined using Protocol Buffers in `proto/`. 
To regenerate the Go code, run:

```bash
make proto
```

This requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` to be installed.

To install the `protoc` compiler:

```bash
# macOS (Homebrew)
brew install protobuf

# Debian/Ubuntu
sudo apt install -y protobuf-compiler
```

See <https://protobuf.dev/installation/> for more options and details.

To install the Go plugins:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

`go install` places the binaries in `$(go env GOPATH)/bin`, so make sure that
directory is on your `PATH`.

## Testing

Tests are run via the `Makefile` targets below.

### Unit tests

```bash
make test
```

Runs the full Go unit test suite (`go test ./...`).

### Integration tests

```bash
make test-integration
```

Builds and runs the integration tests with the `integration` build tag,
the race detector enabled, and caching disabled (`-count=1`).
Pass extra flags via `GOTESTFLAGS`, e.g. `make test-integration GOTESTFLAGS=-v`.

### Docker integration tests

```bash
make test-docker
```

Builds the Docker image and runs the Docker-based integration tests
(`docker` build tag) using [testcontainers](https://testcontainers.com/).
This requires a running Docker daemon. The target auto-detects the
`DOCKER_HOST` endpoint (including Colima setups) so it usually works without
extra configuration.

## Creating tags and releases

Releases are only created from tags.

### 1. Create and push the tag

```bash
TAG="v0.12.3"
git tag "$TAG"
git push origin "$TAG"
```

### 2. Create the release

Use the GitHub CLI (`gh`) to create the release from the tag. 
GitHub will automatically generate release notes based on the commit history.

```bash
gh release create "$TAG" --generate-notes
```

(You might have to run `gh auth login` beforehand.)

#### Optional: Attach a Windows release bundle

If you want to include the legacy Windows release bundle (`.zip`), build it first:

```bash
make release-windows
```

And append the generated archive to the `gh release create` command:

```bash
gh release create "$TAG" --generate-notes "swcat-$TAG-windows-amd64.zip"
```

### 3. Verification

Check that the release and its auto-generated notes look as expected on
<https://github.com/dnswlt/swcat/releases>.

Done!
