# Sidecar Extensions

`swcat` supports an extensibility mechanism called "sidecar extensions". This feature allows you to augment your catalog entities with auto-generated metadata without modifying the source YAML files maintained by humans.

## Concept

The core idea is to separate manually maintained metadata from machine-generated metadata.

### Sidecar Files

When extensions are generated for an entity defined in `catalog-entity.yml`, `swcat` stores the generated data in a "sidecar" file named `catalog-entity.ext.json` in the same directory.

*   **Format:** The file contains a JSON object mapping entity references to their extension data (mainly annotations).
*   **Read-only:** These files are intended to be managed by `swcat` plugins, not edited manually.
*   **Invisible in Editor:** To prevent clutter, these sidecar files are hidden when editing entities in the `swcat` web interface.

#### Example Sidecar File

A sidecar file named `payments.ext.json` might look like this:

```json
{
    "entities": {
        "api:credit-card-check": {
            "annotations": {
                "asyncapi.com/channels": [
                    {
                        "address": "/payments/verify",
                        "messages": [ "VerifyRequest", "VerifyResponse" ]
                    }
                ]
            }
        }
    }
}
```

### Plugins

Plugins are responsible for populating these sidecar files. A plugin typically fetches data from an external system (like a build server, artifact repository, or cloud provider) and attaches it to an entity as an annotation.

For more details, see the [Plugins](plugins.md) page.

## Visualization

Since plugins store their output as standard entity annotations (in the sidecar file), you can visualize this data using the **Custom Content** feature.

See [Custom Content](custom-content.md) to learn how to configure `swcat` to render these JSON annotations as rich tables, lists, or properties cards on your entity pages.
