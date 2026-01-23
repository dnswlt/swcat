# Custom Content

`swcat` allows you to display custom information on entity detail pages by leveraging
entity annotations. This is useful for integrating external data or displaying
specific metadata that doesn't fit into the standard catalog schema.

## How it works

You can configure `swcat` to look for specific annotations on your entities and
render their values as dedicated cards in the user interface.

### 1. Configure swcat.yml

In your `swcat.yml`, use the `ui.annotationBasedContent` section to define which
annotations should be rendered and how.

```yaml
ui:
  annotationBasedContent:
    # Key is the annotation name
    my-org.com/data:
      heading: Organization Data
      style: table  # text | list | json | table
```

### 2. Annotate your entities

Add the corresponding annotation to your entity metadata.

```yaml
apiVersion: swcat.dnswlt.me/v1alpha1
kind: Component
metadata:
  name: my-service
  annotations:
    my-org.com/data: |
      {
        "team": "Alpha",
        "cost-center": "12345",
        "criticality": "high"
      }
spec:
  type: service
```

!!! note
    From the YAML parser's perspective, the annotation value is just a string.
    If you use a structured style like `list`, `json`, or `table`, `swcat` will
    attempt to parse that string as JSON. This approach ensures compatibility
    with Kubernetes CRDs and the Backstage software catalog format, where
    annotations are strictly key-value pairs of strings.

## Supported Styles

The `style` property determines how the annotation value is parsed and displayed:

| Style | Description | Expected Value Format |
| :--- | :--- | :--- |
| `text` | Renders the value as plain text. | Any string. |
| `list` | Renders a bulleted list. | A JSON array of strings: `["item1", "item2"]` |
| `json` | Renders a formatted code block with syntax highlighting. | Any valid JSON object or array. |
| `table` | Renders a key-value table. | A simple JSON object: `{"key": "value"}` |
