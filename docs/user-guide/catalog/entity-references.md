# Entity References

Many `spec` fields contain references to other entities. Entity references are strings that identify other entities, typically in the format:

`[<kind>:][<namespace>/]<name>`

* If `<kind>` is omitted, it is inferred from the context (e.g., for an `owner` field, the kind is `group`).
* If `<namespace>` is omitted, it defaults to the `default` namespace.

Examples:

```yaml
- my-api  # Only name
- my-namespace/my-resource  # Namespace and name
- api:ns/some-api  # Kind, namespace, and name
```

For relationships like `consumesApis`, `providesApis`, and `dependsOn`, you can use a more expressive "labelled" entity reference, which includes an optional version, and a label:

`[<kind>:][<namespace>/]<name> [@<version>] ["<label>"]`

* The `[@<version>]` part is a shorthand for the `version` attribute (see below). It can be used to refer
    to specific versions of an API.
* The `["<label>"]` part describes the relationship and is displayed in SVG diagrams.

Here are some examples of this shorthand notation:

```yaml
- component:my-component "is using"
- api:my-api @v2
- resource:my-db "stores data for"
```

If you want to specify more attributes than just the `version` attribute,
you must use a YAML object instead of the shorthand string:

```yaml
spec:
  dependsOn:
    - ref: component:other-component
      label: is using
      attrs:
        version: v1
        criticality: high
```

Apart from `version`, attributes are not interpreted by `swcat` in any way yet.
