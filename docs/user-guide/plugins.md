# Plugins

Plugins in `swcat` are responsible for enriching entity metadata by fetching or generating data from external sources.

## Overview

A plugin typically connects to an external system (such as an artifact repository, a build server, or a message broker) to retrieve technical details that are then attached to entities as annotations.

## Configuration

Plugins are configured in the `plugins.yml` file. Each plugin definition includes:

*   **Kind:** The type of plugin implementation (e.g., `AsyncAPIImporterPlugin`).
*   **Trigger:** A query predicate that determines which entities the plugin supports.
*   **Inhibit:** An optional predicate to prevent the plugin from running even if the trigger matches.
*   **Spec:** Plugin-specific configuration settings.

### Example `plugins.yml`

```yaml
plugins:
  asyncApiImporter:
    kind: AsyncAPIImporterPlugin
    trigger: "kind:API AND type~'^kafka/'"
    inhibit: "annotation='swcat/visibility=internal'"
    spec:
      targetAnnotation: asyncapi.com/channels
```

## Execution

When a plugin's trigger matches an entity, a "zap" (lightning bolt) icon appears on the entity's detail page in the `swcat` UI. Clicking this icon:

1.  Executes the applicable plugins.
2.  Collects the generated annotations.
3.  Saves the results into a [Sidecar Extension](sidecar-extensions.md) file.

## Available Plugins

`swcat` currently includes the following plugin types:

*   **AsyncAPIImporterPlugin:** Imports channel and message definitions from AsyncAPI specifications.
*   **ExternalPlugin:** Integrates external tools or scripts.

## External Plugins

The `ExternalPlugin` kind allows you to integrate any external tool or script into `swcat`. The tool runs as a subprocess, communicating with `swcat` via standard input (stdin) and standard output (stdout) using JSON.

### Configuration

```yaml
plugins:
  my-plugin:
    kind: ExternalPlugin
    spec:
      command: python3
      args: ["scripts/my_plugin.py"]
      verbose: true
      config:
        key: value
```

| Field | Type | Description |
|---|---|---|
| `command` | `string` | **Required**. The executable to run (e.g., `java`, `python`, `node`, or a script path). |
| `args` | `[]string` | Command-line arguments passed to the executable. |
| `verbose` | `bool` | If `true`, logs input and output JSON payloads to the console for debugging. |
| `config` | `map` | Arbitrary key-value pairs passed to the plugin in the input JSON. |

### Protocol

#### Input JSON (stdin)

The plugin receives a JSON object with the following structure:

```json
{
  "entity": {
    "metadata": {
      "name": "component-name",
      "namespace": "default",
      "annotations": { ... },
      "labels": { ... }
    },
    "spec": { ... }
  },
  "config": {
    "key": "value"  // From plugins.yml spec.config
  },
  "tempDir": "/path/to/temp/dir",
  "args": { ... } // Runtime arguments
}
```

#### Output JSON (stdout)

The plugin must print a single JSON object to stdout. Any other output (logs, debug info) should be printed to **stderr**.

```json
{
  "success": true, // or false
  "error": "Error message if success is false",
  "generatedFiles": [
    "/absolute/path/to/file1",
    "/absolute/path/to/file2"
  ],
  "annotations": {
    "new/annotation": "value" // Annotations to add back to the entity
  }
}
```

### Example: Maven Artifact Extractor

A reference implementation is available in `extensions/maven`. This Java application acts as an External Plugin that downloads artifacts from a Maven repository and extracts files from them.

See [extensions/maven/README.md](../../extensions/maven/README.md) for details.

---

*For details on how to visualize plugin output, see [Custom Content](custom-content.md).*
