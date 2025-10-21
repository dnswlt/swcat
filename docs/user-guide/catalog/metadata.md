# Metadata

Every entity definition follows the Kubernetes Resource Model (KRM) structure
and therefore consists of four fields:

```yaml
apiVersion: swcat/v1
kind: Component  # Or System, Resource, API, etc.
metadata: ... # See this section
spec: ...  # Specific to each entity kind
```

The `metadata` section contains the following fields.

The valid `metadata` fields are the following:

* `name` - *required* - The name of the entity. Must be unique within the catalog
    for any given namespace + kind pair.

* `namespace` - *optional* - The namespace that the entity belongs to.
    If empty, the entity is assume to live in the default namespace.

* `title` - *optional* - A display name of the entity, used in certain places in the UI.

* `description` - *optional* - A short description of the entity (one or a few lines max.)
    Do not use this field to document a component, API, etc. in detail, but use links to
    point to external documentation.

* `labels` - *optional* - User-specified key/value pairs
    that are displayed as small chips in the swcat UI entity detail view and can be used
    for filtering entities.
    See the k8s documentation for the intended semantics of
    [labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/).

* `annotations` - *optional* - User-specified key/value pairs that are not visible in the UI.
    Also see the k8s documentation for
    [annotations](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/).

!!! tip
    There are a few well-known annotations that can be used to control the display
    of entities in the UI:

    * `swcat/repo` - A URL prefix that is used to generate a Link entry for the entity
        pointing to a (Git or other) SCM repository for the node. The name of the
        entity will be appended to the prefix to form the link's href.
        Use `swcat/repo: default` to refer to the repository prefix defined in the `swcat.yml`
        configuration file as `repositoryURLPrefix`.
    * `swcat/stereotype` - A `<<stereotype>>` label that should be shown for the node
        in SVG diagrams.
    * `swcat/fillcolor` - An SVG color name or 6-digit hex color code (e.g., `#7f7f7f`) that
        should be used to color the entity node in SVG diagrams.

* `tags` - *optional* - A list of single-valued strings that can used to, well, tag entities.

* `links` - *optional* - A list of external hyperlinks related to the entity (e.g., documentation).

Example:

```yaml
metadata:
  name: my-component
  namespace: my-namespace
  title: My Component
  description: |
    Brief description of the entity. **Can** use _simple_ Markdown,
    including [links](https://example.com).
  labels:
    foobar.dev/language: java17
  annotations:
    swcat/stereotype: new
  tags:
    - needs-update
  links:
    - url: https://example.com/my-component
      title: Source code
```
