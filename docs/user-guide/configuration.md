# Configuration

You can configure swcat by providing a YAML configuration file
(typically `swcat.yml`) via the `--config` command line flag.
The following sections explain the available configuration options.

See [internal/config/config.go](https://github.com/dnswlt/swcat/blob/main/internal/config/config.go)
for the Go struct that holds all available configuration options.

## SVG Configuration

The `svg` section allows you to customize the appearance of the generated SVG diagrams.

* `stereotypeLabels`: A list of labels whose values should be displayed as &lt;&lt;stereotypes&gt;&gt; in node labels.
* `nodeColors`: Allows you to override the default node colors based on labels or types.
  * `labels`: Maps label keys and values to specific colors.
  * `types`: Maps entity types to specific colors.
* `showAPIProvider`: If true, includes the API provider (component) in the labels of API entities.
* `showVersionAsLabel`: If true, shows the API version in consumed/provided API references if no explicit label is present.

## Catalog Configuration

The `catalog` section allows you to configure repository-specific settings.

* `repositoryURLPrefix`: A URL prefix for constructing links to the source repositories of entities.
* `validation`: Defines validation rules for entity specifications.
  You can define rules for domains, systems, components, resources, and APIs.
  * `values`: A list of allowed values for a field.
  * `matches`: A list of regular expressions that the value must match.

## Example Configuration

```yaml
# Example configuration file.
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
  # Show the API version in consumedApis/providedApis references, if specified
  # and unless an explicit label is present.
  showVersionAsLabel: false
catalog:
  # Entities with a `swcat/repo: default` annotation will have a generated
  # link in the Links section that points to <repositoryURLPrefix>/<entity-name>.
  # This is useful in "multi-repo" setups where each component lives in its own
  # git repository.
  repositoryURLPrefix: https://github.com/dnswlt/swcat
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
