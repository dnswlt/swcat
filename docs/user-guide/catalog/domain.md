# Domain

> A high-level grouping of related systems that share a bounded context
> (terminology, models, business purpose).

The `spec` of a `Domain` entity has the following fields:

* `owner` - *required* - An entity reference to the owner of the domain (e.g., `group:my-team`).
* `subdomainOf` - *optional* - An entity reference to another domain of which this domain is a part.
* `type` - *optional* - The type of domain.

Example:

```yaml
spec:
  owner: default/my-team
  subdomainOf: domain:default/parent-domain
```
