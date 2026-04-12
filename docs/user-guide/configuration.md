# Configuration

`swcat` follows a convention-over-configuration approach for its data directory structure.
It expects the following files and directories to be present under the data root directory:

* `catalog/`: A directory containing your YAML entity definitions.
* `swcat.yml`: The main configuration file (optional).
* `plugins.yml`: The plugins configuration file (optional).
* `lint.yml`: The linting rules configuration file (optional).

You specify the data root directory via the `--root-dir` command line flag (for local storage)
or the `--git-root-dir` flag (when using a git repository as storage).

The following sections explain the available options within `swcat.yml`.

See [internal/config/config.go](https://github.com/dnswlt/swcat/blob/main/internal/config/config.go)
for the Go struct that holds all available configuration options.

## Catalog Configuration

The `catalog` section allows you to configure repository-specific settings.

* `annotationBasedLinks`: An optional map from annotation keys to links.
    The `url` and `title` fields support the following template placeholders:
    * `{{ .Metadata.<Field> }}` for any `<Field>` in the entity's metadata
        (e.g., `Name`).
    * `{{ .Annotation.Key }}` and `{{ .Annotation.Value }}` for the key and value
        of the annotation being processed.
    * *(only for versioned API entities)* `{{ .Version }}` and
        `{{ .Version.<Part> }}` for each API version or one of its parts
        (`Major`, `Minor`, `Patch`, `Suffix`).
        The version part fields are only populated if the version string matches a
        common pattern (e.g. *v1*, *1.2.3*, or *v1alpha*).

    Supports `multiLinks` and `multiLinkData` for generating per-environment link
    groups. See [Multi-environment Links](#multi-environment-links) below.

* `automaticLinks`: A list of link templates automatically added to entities
    matching a filter expression. Each entry has the following fields:
    * `filter`: A query expression (see [Query Syntax](query-syntax.md)) that
        determines which entities the link applies to.
    * `url`: The URL template for the link (supports `{{ .Metadata.<Field> }}`).
    * `title`: The title template for the link.

    Supports `multiLinks` and `multiLinkData` for generating per-environment link
    groups. See [Multi-environment Links](#multi-environment-links) below.

* `validation`: Defines validation rules for entity specifications.
  You can define rules for domains, systems, components, resources, and APIs.
    * `values`: A list of allowed values for a field.
    * `matches`: A list of regular expressions that the value must match.

### Custom template functions

Both `annotationBasedLinks` and `automaticLinks` support custom template functions:

* `{{ first <val1> <val2> ... }}` returns the first non-empty string. This is
    useful to provide fallback values, e.g. `{{ first (index .Metadata.Annotations "my/annot") .Metadata.Name }}`.
* `{{ pathEscape <string> }}` percent-encodes a string for use in URL path
    segments (e.g. `{{ pathEscape .Metadata.Name }}`).
* `{{ queryParams <key1> <val1> <key2> <val2> ... }}` builds a set of query
    parameters from an even-numbered list of key-value pairs.
* `{{ addQueryParams <baseUrl> <queryParams> }}` appends query parameters to a
    base URL, merging with any existing ones.
    Example: `{{ addQueryParams "https://example.com" (queryParams "id" .Metadata.Name "env" .MultiLink.Value) }}`.


### Multi-environment Links

Both `annotationBasedLinks` and `automaticLinks` support generating multiple
links from a single template. This is useful for linking to the same resource
across multiple environments or stages.

You can either provide a static list of links via `multiLinks`, or dynamic
entries via `multiLinkData`.

#### Static Multi-links

The `multiLinks` field takes a list of entries, each with two fields:

* `label`: The short display label shown as a pill in the UI (e.g. `dev`).
* `value`: Substituted into the `url` template via `{{ .MultiLink.Value }}`.

#### Dynamic Multi-links

The `multiLinkData` field allows you to fetch the link entries from an entity
annotation. If `multiLinkData: <name>` is set, `swcat` reads the entries from the
`swcat/data-<name>` annotation of the entity (or its parent system/domain).

The annotation value must be a valid JSON list of objects with `label` and
`value` fields, e.g.:
`[{"label": "dev", "value": "development"}, {"label": "prod", "value": "production"}]`.

The `title` template serves as the shared group title (e.g. `Monitoring`) and
is rendered without `{{ .MultiLink.* }}` data. Individual link titles are derived
automatically as `<group title> (<label>)` (e.g. `Monitoring (dev)`).

In the UI, grouped links are displayed as a labelled row of clickable pills:

```text
Monitoring  [dev]  [staging]  [prod]
```

See the [Example Configuration](#example-configuration) below for a complete
example.

## SVG Configuration

The `svg` section allows you to customize the appearance of the generated SVG diagrams.

* `stereotypeLabels`: A list of labels whose values should be displayed as
    &laquo;stereotypes&raquo; in node labels.
* `nodeColors`: Allows you to override the default node colors based on labels
     or types.
    * `labels`: Maps label keys and values to specific colors.
    * `types`: Maps entity types to specific colors.
* `showAPIProvider`: If true, includes the API provider (component) in the labels of API entities.
* `showParentSystem`: If true, includes the parent system in the labels of component, resource, and API entities.
* `showVersionAsLabel`: If true, shows the API version in consumed/provided API references if no explicit label is present.

## UI Configuration

The `ui` section allows for customizing the user interface.

* `annotationBasedContent`: Defines custom sections on entity detail pages based on annotations. See [Custom Content](custom-content.md) for details.
* `helpLinks`: An optional list of custom help links (each with `title` and `url`) displayed in the footer.

## Example Configuration

```yaml
# Example configuration file.
ui:
  # Define custom sections in entity detail pages based on annotations.
  annotationBasedContent:
    # Show solace topics, annotated as a JSON list in the solace.com/topics annotation,
    # as a card on the API detail page:
    solace.com/topics:
      heading: Solace Topics
      style: list  # Possible values: text|list|json|table
  # Add custom help links to the footer.
  helpLinks:
    - title: "Internal Documentation"
      url: "https://wiki.example.com/swcat"
    - title: "Support Channel"
      url: "https://slack.com/app_redirect?channel=swcat-support"
svg:
  # Show the (programming) language label as a <<stereotype>> on nodes.
  stereotypeLabels:
    - foobar.dev/language
  # Highlight nodes with a certain status in different fill colors.
  nodeColors:
    labels:
      foobar.dev/status:
        deprecated: '#f3c1de'
        critical: '#c7398a'
    types:
      # Color entities (of any kind) with spec.type "external" in a special color.
      external: '#ffbf79'
  # Include the API provider (component) in labels of API entities.
  showAPIProvider: true
  # Include the parent system in labels of component, resource, and API entities.
  showParentSystem: true
  # Show the API version in consumedApis/providedApis references, if specified
  # and unless an explicit label is present.
  showVersionAsLabel: false
catalog:
  annotationBasedLinks:
    # Auto-generates an entry in the "Links" section of every entity detail page
    # that has a hexz.me/repo annotation.
    hexz.me/repo:
      # The annotation value is the "project" name, the repo is named after the entity.
      url: https://example.com/projects/{{ .Annotation.Value }}/repos/{{ .Metadata.Name }}
      title: Source code
    # Auto-generates per-environment monitoring links for annotated entities.
    # Rendered as a grouped pill row: "Monitoring  [dev]  [staging]  [prod]"
    hexz.me/app-name:
      url: https://grafana.{{ .MultiLink.Value }}.example.com/d/{{ .Annotation.Value }}
      title: Monitoring
      icon: dashboard
      multiLinks:
        - label: dev
          value: dev.example.com
        - label: staging
          value: staging.example.com
        - label: prod
          value: prod.example.com
    # Auto-generates per-environment monitoring links for annotated entities.
    # The list of environments is fetched from the swcat/data-environments annotation.
    # Rendered as a grouped pill row: "Logs  [dev]  [staging]  [prod]"
    hexz.me/app-logs:
      url: https://logs.{{ .MultiLink.Value }}.example.com/s/{{ .Metadata.Name }}
      title: Logs
      icon: list
      multiLinkData: environments
  validation:
    api:
      type:
        matches: 
          - "http(s)?/.*"
          - "grpc(/.*)?"
          - "rest(/.*)?"
      lifecycle: 
        values: ["experimental", "production", "deprecated"]
    resource:
      type:
        values: ["database", "cache"]
    component:
      type:
        values: ["service", "batch", "support", "external"]
      lifecycle: 
        values: ["development", "production", "deprecated", "external"]
    system:
      type:
        matches:
          - ".*"  # just for fun
```
