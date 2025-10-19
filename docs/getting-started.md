# Getting started

Whether you run swcat in Docker or natively, check out the source code repo first:

```bash
git clone https://github.com/dnswlt/swcat.git
```

## Getting started (Docker)

To run `swcat` locally in Docker and serve the [example catalog folder](https://github.com/dnswlt/swcat/tree/main/examples/twosys):

Before the first execution, set up the `.env` file, so files modified inside the container 
have proper user and group IDs  on the host file system:

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
`CATALOG_DIR` environment variable. Your (optional) configuration file must be located
at `$CATALOG_DIR/swcat.yml`.

```bash
CATALOG_DIR=/abs/path/to/your/catalog make docker-up
```

## Getting started (w/out Docker)

### Prequisites

* Install a recent version of [Go](https://go.dev/) (>= 1.24.5).
* Install `npm` (e.g. via [nvm](https://github.com/nvm-sh/nvm)).
* Install [Graphviz](https://graphviz.org/download/).

### Build and run

Build the frontend artifacts:

```bash
cd web
npm install
npm run build
cd ..
```

Now run the server, using the example catalog files:

```bash
go run ./cmd/swcat -addr localhost:9191 -config examples/config/swcat.yml examples/twosys
```

Point your browser at <http://localhost:9191> and explore the example catalog.