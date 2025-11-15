# Resource

> Represents infrastructure such as messaging brokers, caches, or databases.

The `spec` of a `Resource` entity has the following fields:

* `type` - *required* - The type of resource (e.g., "database", "messaging-broker").
* `owner` - *required* - An [entity reference](./entity-references.md) to the owner of the resource (e.g., `group:my-team`).
* `system` - *required* - An [entity reference](./entity-references.md) to the system that the resource belongs to.
* `dependsOn` - *optional* - A list of [entity references](./entity-references.md)
    to other components or resources that this component depends on.
    MUST include the kind specifier, e.g. `resource:my-database`.

Example:

```yaml
apiVersion: swcat/v1
kind: Resource
metadata:
    name: my-resource
    title: My Resource
    description: |
        My Resource is a relational database (postgres) used to store items.
    # See metadata.md for other fields like labels, annotations, etc.
spec:
  type: database
  owner: teams/my-team
  system: my-system
  dependsOn:
    - resource:aws/my-s3-bucket
```
