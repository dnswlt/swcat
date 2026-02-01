# Configuration

You can configure swcat by providing a YAML configuration file
(typically `swcat.yml`) via the `--config` command line flag.
The following sections explain the available configuration options.

See [internal/config/config.go](https://github.com/dnswlt/swcat/blob/main/internal/config/config.go)
for the Go struct that holds all available configuration options.

## Catalog Configuration

The `catalog` section allows you to configure repository-specific settings.

* `annotationBasedLinks`: An optional map from annotation keys to links.
    The `url` and `title` fields of these links support the following
    template annotations:
    * `{{ .Metadata.<Field> }}` for any `<Field>` in the entity's metadata
        (e.g., `Name`).
    * `{{ .Annotation.Key }}` and `{{ .Annotation.Value }}` for the key and value
        of the annotation being processed.
    * *(only for versioned API entities)* `{{ .Version }}` and
        `{{ .Version.<Part> }}` for each API version or one of its parts
        (`Major`, `Minor`, `Patch`, `Suffix`).
        The version part fields are only populated if the version string matches a
        common pattern (e.g. *v1*, *1.2.3*, or *v1alpha*).

* `validation`: Defines validation rules for entity specifications.
  You can define rules for domains, systems, components, resources, and APIs.
    * `values`: A list of allowed values for a field.
    * `matches`: A list of regular expressions that the value must match.

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
* `helpLink`: An optional custom link (with `title` and `url`) displayed in the footer.

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
