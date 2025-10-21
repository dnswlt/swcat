# Component

> A deployable and runnable software artifact such as an API gateway or
> a backend service.

The `spec` of a `Component` entity has the following fields:

* `type` - *required* - The type of component (e.g., "service", "website", "library").
* `lifecycle` - *required* - The lifecycle state of the component (e.g., "production", "experimental").
* `owner` - *required* - An entity reference to the owner of the component (e.g., `group:my-team`).
* `system` - *required* - An entity reference to the system that the component belongs to.
* `subcomponentOf` - *optional* - An entity reference to another component of which this component is a part.
* `providesApis` - *optional* - A list of APIs that are provided by this component.
* `consumesApis` - *optional* - A list of APIs that are consumed by this component.
* `dependsOn` - *optional* - A list of other entities that this component depends on.

Example:

```yaml
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
    - resource:default/my-database
```
