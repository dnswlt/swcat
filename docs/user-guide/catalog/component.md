# Component

> A deployable and runnable software artifact such as an API gateway or
> a backend service.

The `spec` of a `Component` entity has the following fields:

* `type` - *required* - The type of component (e.g., "service", "website", "library").
* `lifecycle` - *required* - The lifecycle state of the component (e.g., "production", "experimental").
* `owner` - *required* - An [entity reference](./entity-references.md) to the owner of the component (e.g., `group:my-team`).
* `system` - *required* - An [entity reference](./entity-references.md) to the system that the component belongs to.
* `providesApis` - *optional* - A list of [entity references](./entity-references.md)
    to APIs that are provided by this component.
* `consumesApis` - *optional* - A list of [entity references](./entity-references.md)
    to APIs that are consumed by this component.
    May use version references and labels, e.g. `my-api @v2 "oauth"`.
* `dependsOn` - *optional* - A list of [entity references](./entity-references.md)
    to other components or resources that this component depends on.
    MUST include the kind specifier and may use labels, e.g. `resource:my-database "read"`.

Example:

```yaml
apiVersion: swcat/v1
kind: Component
metadata:
    name: my-component
spec:
  type: service
  lifecycle: production
  owner: my-team
  system: my-system
  providesApis:
    - my-api
  consumesApis:
    - other-api @v2 "some usage"
  dependsOn:
    - resource:my-database
```
