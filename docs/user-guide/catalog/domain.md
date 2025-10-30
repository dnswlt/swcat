# Domain

> A high-level grouping of related systems that share a bounded context
> (terminology, models, business purpose).

The `spec` of a `Domain` entity has the following fields:

* `owner` - *required* - An [entity reference](./entity-references.md) to the owner of the domain (e.g., `group:my-team`).
* `type` - *optional* - The type of domain.

Example:

```yaml
apiVersion: swcat/v1
kind: Domain
metadata:
    name: my-domain
spec:
  owner: default/my-team
  subdomainOf: domain:default/parent-domain
```
