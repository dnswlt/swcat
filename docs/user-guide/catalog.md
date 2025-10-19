# The software catalog

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

## YAML file structure

The fields of each entity kind are documented in
[internal/api/api.go](https://github.com/dnswlt/swcat/blob/main/internal/api/api.go)

See [examples/](https://github.com/dnswlt/swcat/tree/main/examples/twosys) for example definitions.
