# Sidecar Extensions

`swcat` supports an extensibility mechanism called "sidecar extensions". This feature allows you to augment your catalog entities with auto-generated annotations without modifying the source YAML files maintained by humans.

## Concept

The core idea is to separate manually maintained metadata from machine-generated metadata.

### Sidecar Files

When extensions are generated for an entity defined in `catalog-entity.yml`, `swcat` stores the generated data in a "sidecar" file named `catalog-entity.ext.json` in the same directory.

*   **Format:** The file contains a JSON object mapping entity references to their extension data. Sidecars carry **annotations** only.
*   **Read-only:** These files are intended to be managed by `swcat` plugins, not edited manually.
*   **Invisible in Editor:** To prevent clutter, these sidecar files are hidden when editing entities in the `swcat` web interface.

#### Example Sidecar File

A sidecar file named `payments.ext.json` might look like this:

```json
{
    "entities": {
        "component:payment-service": {
            "annotations": {
                "example.com/build-info": {
                    "commit": "a1b2c3d",
                    "buildTime": "2024-09-12T10:30:00Z"
                }
            }
        }
    }
}
```

### Plugins

Plugins can emit two kinds of output:

*   **Annotations:** Persisted in the sidecar `.ext.json` files described above and merged into the entity's `metadata.annotations` at load time.
*   **Status observations:** Persisted separately in `swcat`'s database (the `status_observations` table) and exposed on the entity's runtime `status` field. Use these for "live" data that changes frequently and should not pollute the source tree.

A plugin chooses which output type fits its data; it is not required to produce both. For more details, see the [Plugins](plugins/index.md) page.

## Visualization

You can render plugin output directly in the entity UI using the **Custom Content** feature:

*   `ui.annotationBasedContent` — for sidecar/annotation values.
*   `ui.statusBasedContent` — for status observations.

See [Custom Content](custom-content.md) to learn how to configure `swcat` to render JSON values as rich tables, lists, or properties cards on your entity pages.
