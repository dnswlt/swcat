# Plugins

Plugins in `swcat` are responsible for enriching entity metadata by fetching or generating data from external sources.

## Overview

A plugin typically connects to an external system (such as an artifact repository, a build server, or a message broker) to retrieve technical details that are then attached to entities as annotations.

## Configuration

Plugins are configured in a `plugins.yml` file. Each plugin definition includes:

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
*   **MavenArtifactExtractorPlugin:** Extracts files or metadata from Maven artifacts.

---

*For details on how to visualize plugin output, see [Custom Content](custom-content.md).*
