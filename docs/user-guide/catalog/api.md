# API

> A communication interface provided by one or more components and consumed by
> others (e.g., gRPC, http/REST, Pub/Sub topics, web services, or FTP).

The `spec` of an `API` entity has the following fields:

* `type` - *required* - The type of the API (e.g., "openapi", "grpc", "asyncapi").
* `lifecycle` - *required* - The lifecycle state of the API (e.g., "production", "experimental").
* `owner` - *required* - An [entity reference](./entity-references.md) to the owner of the API (e.g., `group:my-team`).
* `system` - *required* - An [entity reference](./entity-references.md) to the system that the API belongs to.
* `versions` - *optional* - A list of versions in which this API currently exists.
    * `version` - *required* - The version name, e.g. `v1` or  `1.0.0`.
    * `lifecycle` - *required* - The lifecycle state of the API in this particular version.
        The lifecycle of at least one version must match the lifecycle of the API. 

Example:

```yaml
apiVersion: swcat/v1
kind: API
metadata:
    name: my-api
spec:
  type: openapi
  lifecycle: production
  owner: my-team
  system: my-system
  versions:
    - version: v1
      lifecycle: deprecated
    - version: v2.1
      lifecycle: production
```
