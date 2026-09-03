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

* `annotationBasedLinks`: An optional map from annotation keys to simple,
    one-to-one links. A link is added whenever an entity has the corresponding
    annotation. The `url` and `title` templates can use:
    * `{{ .Metadata.<Field> }}` for any `<Field>` in the entity's metadata
        (e.g., `Name`).
    * `{{ .Annotation.Key }}` and `{{ .Annotation.Value }}` for the key and value
        of the annotation being processed.
    * `{{ .GetAnnotation "key" }}` and `{{ .Label "key" }}` for annotations and
        labels directly on the entity.
    * `{{ .IAnnotation "key" }}` and `{{ .ILabel "key" }}` to search the entity
        and its parent hierarchy.
    * `{{ .System }}` and `{{ .Domain }}` for the related entities, where
        applicable (e.g., `{{ .System.Metadata.Name }}`).

* `starlarkLinks`: A list of Starlark programs that generate zero or more links
    for entities matching a filter. Each entry has a `filter` using the existing
    [query syntax](query-syntax.md) and a `file` path relative to `swcat.yml`.
    See [Starlark links](starlark-links.md) for the script API and examples.

* `validation`: Defines validation rules for entity specifications.
  You can define rules for domains, systems, components, resources, and APIs.
    * `values`: A list of allowed values for a field.
    * `matches`: A list of regular expressions that the value must match.

### Legacy generated-link configuration

`automaticLinks`, `multiLinks`, `multiLinkData`, the `swcat/data-*` annotation
convention and its `{label, value}` entries, and implicit per-version expansion
through `{{ .Version }}` are deprecated. They remain supported so historical
Git branches and tags containing those settings continue to load, but new
configurations should use `starlarkLinks` and application-specific annotations.

### Annotation link template functions

`annotationBasedLinks` supports these template functions:

* `{{ first <val1> <val2> ... }}` returns the first non-empty string. This is
    useful to provide fallback values, e.g. `{{ first (index .Metadata.Annotations "my/annot") .Metadata.Name }}`.
* `{{ pathEscape <string> }}` percent-encodes a string for use in URL path
    segments (e.g. `{{ pathEscape .Metadata.Name }}`).
* `{{ queryParams <key1> <val1> <key2> <val2> ... }}` builds a set of query
    parameters from an even-numbered list of key-value pairs.
* `{{ addQueryParams <baseUrl> <queryParams> }}` appends query parameters to a
    base URL, merging with any existing ones.
    Example: `{{ addQueryParams "https://example.com" (queryParams "id" .Metadata.Name) }}`.

## SVG Configuration

The `svg` section allows you to customize the appearance of the generated SVG diagrams.

* `stereotypeLabels`: A list of labels whose values should be displayed as
    &laquo;stereotypes&raquo; in node labels.
* `nodeColors`: Allows you to override the default node colors based on labels
     or types.
    * `labels`: Maps label keys and values to specific colors.
    * `types`: Maps entity types to specific colors.
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
  # Use Starlark when link generation involves conditions, related entities,
  # versions, or multiple links.
  starlarkLinks:
    - filter: kind=component
      file: links/components.star
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
