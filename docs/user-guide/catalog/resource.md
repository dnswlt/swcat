# Resource

> Represents infrastructure such as messaging brokers, caches, or databases.

The `spec` of a `Resource` entity has the following fields:

* `type` - *required* - The type of resource (e.g., "database", "messaging-broker").
* `owner` - *required* - An entity reference to the owner of the resource (e.g., `group:my-team`).
* `system` - *required* - An entity reference to the system that the resource belongs to.
* `dependsOn` - *optional* - A list of other entities that this resource depends on.

Example:

```yaml
spec:
  type: database
  owner: my-team
  system: my-system
```
