# Getting started

This page describes how to get started on a local machine using a local
software catalog. See the [user manual](./user-guide/remote-catalog.md) for
how to connect to a remote catalog.


## Getting started (Docker)

To run `swcat` locally in Docker and serve the [example catalog folder](https://github.com/dnswlt/swcat/tree/main/examples/flights):

Check out the source code repo:

```bash
git clone https://github.com/dnswlt/swcat.git
```

Before the first execution, set up the `.env` file, so files modified inside the
container have proper user and group IDs  on the host file system:

```bash
# in the repo root
echo "UID=$(id -u)" > .env
echo "GID=$(id -g)" >> .env
```

Then, run docker via `make`:

```bash
make docker-build
make docker-up
```

Then open: <http://localhost:9191>

To stop the process:

```bash
make docker-stop
```

* Docker Compose maps host 9191 to container 8080.
* The catalog is mounted in read-write (rw) mode at `/catalog` inside the container.

If you want to work with your own catalog, pass its location (folder) in the
`CATALOG_DIR` environment variable. Your configuration file and catalog files
must be located in this directory:

```bash
# For convenience, add CATALOG_DIR to your .env file, so you don't have to specify
# it on the CLI.
echo CATALOG_DIR=/abs/path/to/your/catalog >> .env
# Run the server
make docker-up
```

If the defaults (config file is `swcat.yml`,
catalog directory is `catalog/`) do not apply, see [compose.yml](https://github.com/dnswlt/swcat/blob/main/compose.yml)
for the environment variables to set.

!!! tip
    `swcat` refuses to start if there are catalog validation errors.
    Check the stderr logs in such cases to understand the problem.

## Getting started (w/out Docker)

### Prerequisites

* Install a recent version of [Go](https://go.dev/) (>= 1.24.5).
* Install `npm` (e.g. via [nvm](https://github.com/nvm-sh/nvm)).
* Install [Graphviz](https://graphviz.org/download/).

### Build and run

Check out the source code repo:

```bash
git clone https://github.com/dnswlt/swcat.git
```

Build the frontend artifacts:

```bash
cd web
npm install
npm run build
cd ..
```

Now run the server, using the example catalog files:

```bash
go run ./cmd/swcat -addr localhost:9191 -root-dir ./examples/flights
```

Point your browser at <http://localhost:9191> and explore the example catalog.

## Getting started (Windows)

### Prerequisites (Windows)

* Install [Graphviz](https://graphviz.org/download/).
* Download the latest binary release version `swcat-<version>.zip` from the
   [GitHub releases page](https://github.com/dnswlt/swcat/releases).

### Run swcat.exe

Unpack `swcat-<version>.zip` to any folder you like and run:

```bash
swcat.exe -addr localhost:9191 -root-dir examples/flights
```

Point your browser at <http://localhost:9191> and explore the example catalog.

Adjust the `-root-dir` to any software catalog you want to view or edit.
`swcat` expects a fixed structure under the root directory: `swcat.yml`, `plugins.yml`, and a `catalog/` directory.
