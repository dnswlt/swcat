# swcat

A simple tool to define and visualize systems, their components, resources, and interfaces.

Inspired by the [backstage.io](https://backstage.io/)
[Software Catalog](https://backstage.io/docs/features/software-catalog/).

## Getting started (Docker)

To run `swcat` locally in Docker and serve the example catalog folder (`./examples/twosys`):

```bash
# from the repo root
docker compose up --build
```

Then open: <http://localhost:9191>

* Docker Compose maps host 9191 to container 8080.
* The catalog is mounted read-only at `/catalog` inside the container.

If you want to work with your own catalog, pass its location (folder) in the
`CATALOG_DIR` environment variable:

```bash
CATALOG_DIR=/abs/path/to/your/catalog docker compose up --build
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
go run ./cmd/swcat -addr localhost:9191 examples/twosys
```

Point your browser at <http://localhost:9191> and explore the example catalog.

## The software catalog

The `swcat` software catalog consists of a set of YAML (`*.yml`) files,
each containing one or more entity definitions.
Its data format follows the Kubernetes Resource Model (KRM), using the familiar
`apiVersion`, `kind`, `metadata`, and `spec` fields.
Supported entity kinds are a subset of the
[backstage.io software catalog](https://backstage.io/docs/features/software-catalog/descriptor-format),
with minor adjustments to required and optional fields:

* **Domain**
  * A high-level grouping of related systems that share a bounded context
    (terminology, models, business purpose).
* **System**
  * A collection of Components, Resources, and APIs that together
    deliver a cohesive *application*.
* **Component**
  * A deployable and runnable software artifact such as an API gateway or
    a backend service.
* **Resource**
  * Represents infrastructure such as messaging brokers, caches, or databases.
* **API**
  * A communication interface provided by one or more components and consumed by
    others (e.g., gRPC, http/REST, Pub/Sub topics, web services, or FTP).
* **Group**
  * An organizational entity (team or business unit) used to model ownership and contact information.

The fields of each entity kind are documented in
[internal/backstage/api.go](./internal/backstage/api.go).

See [examples/twosys/](./examples/twosys/) for example definitions.
