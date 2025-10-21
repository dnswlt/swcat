# API

> A communication interface provided by one or more components and consumed by
> others (e.g., gRPC, http/REST, Pub/Sub topics, web services, or FTP).

The `spec` of an `API` entity has the following fields:

* `type` - *required* - The type of the API (e.g., "openapi", "grpc", "asyncapi").
* `lifecycle` - *required* - The lifecycle state of the API (e.g., "production", "experimental").
* `owner` - *required* - An entity reference to the owner of the API (e.g., `group:my-team`).
* `system` - *required* - An entity reference to the system that the API belongs to.
* `versions` - *optional* - A list of versions in which this API currently exists.

Example:

```yaml
spec:
  type: openapi
  lifecycle: production
  owner: my-team
  system: my-system
```
