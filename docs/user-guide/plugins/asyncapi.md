# AsyncAPI Importer Plugin

The `AsyncAPIImporterPlugin` imports channel and message definitions from [AsyncAPI](https://www.asyncapi.com/) specifications into entity annotations. 

This plugin acts as a **processor** in a chain: it requires a *provider plugin* (such as a [gRPC Plugin](grpc.md) or [External Plugin](external.md)) to fetch the specification file from any source (Artifactory, GitHub, container images, etc.).

## Configuration

The plugin is configured in `plugins.yml` with `kind: AsyncAPIImporterPlugin`.

### Specification Fields

| Field | Type | Description |
|---|---|---|
| `providerPlugin` | `string` | **Required**. The name of another defined plugin that will retrieve the AsyncAPI file. |
| `file` | `string` | **Required**. The path or identifier of the file to retrieve (passed to the provider plugin). |
| `targetAnnotation` | `string` | **Required**. The annotation key where the parsed channel definitions will be stored. |

### Example Configuration

In this example, the `maven-extractor` plugin (an external plugin) downloads a JAR and extracts a specific YAML file. The `asyncapi-importer` then processes that file.

```yaml
plugins:
  maven-extractor:
    kind: ExternalPlugin
    # ... (see External Plugins documentation)

  asyncapi-importer:
    kind: AsyncAPIImporterPlugin
    trigger: "kind:API AND type:kafka"
    spec:
      providerPlugin: "maven-extractor"
      file: "asyncapi.yaml"
      targetAnnotation: "asyncapi.com/channels"
```

## How it Works

1.  **Delegation:** When executed, the importer calls the `providerPlugin` specified in its config.
2.  **Retrieval:** It passes the `file` parameter to the provider. The provider is expected to "return" exactly one file (usually by writing it to the plugin's temporary directory).
3.  **Parsing:** The importer parses the retrieved file as an AsyncAPI specification.
4.  **Extraction:** It extracts a simplified representation of the channels and messages.
5.  **Enrichment:** The extracted data is saved to the entity's `targetAnnotation`.

## Visualization

The imported data can be visualized using [Custom Content](../custom-content.md). A common approach is to use the `table` style to list channels:

```yaml
ui:
  annotationBasedContent:
    asyncapi.com/channels:
      title: "Message Channels"
      type: table
      config:
        columns:
          - title: "Channel"
            value: "{{ .Address }}"
          - title: "Messages"
            value: "{{ range .Messages }}{{ . }} {{ end }}"
```

---

!!! tip
    The `AsyncAPIImporterPlugin` expects the provider plugin to implement the `Files()` interface in its return value. Both `ExternalPlugin` and `GRPCPlugin` support this automatically.
