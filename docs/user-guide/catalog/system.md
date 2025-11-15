# System

> A collection of Components, Resources, and APIs that together
> deliver a cohesive *application*.

The `spec` of a `System` entity has the following fields:

* `owner` - *required* - An [entity reference](./entity-references.md) to the owner of the system (e.g., `group:my-team`).
* `domain` - *optional* - An [entity reference](./entity-references.md) to the domain that the system belongs to.
* `type` - *optional* - The type of system.

Example:

```yaml
apiVersion: swcat/v1
kind: System
metadata:
    name: my-system
    title: My System
    description: |
        My System is the application to render and manage wonderful things.
    # See metadata.md for other fields like labels, annotations, etc.
spec:
  owner: teams/my-team
  domain: my-domain
```
