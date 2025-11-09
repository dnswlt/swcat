# JSON API for Entities

The `/catalog/entities` endpoint provides a JSON API to query and
manipulate entities stored in the catalog.

## Querying entities

To query entities, send a `GET` request to `/catalog/entities`.
You can filter the results using the `q` query parameter, which accepts the same
[query syntax](../user-guide/query-syntax.md) as the UI search.

### Query example

To retrieve all components, you can use the following request:

```
GET /catalog/entities?q=kind:component
```

Here is an example using `curl`:

```bash
curl -G 'http://localhost:9191/catalog/entities' \
  --data-urlencode 'q=kind:Component OR kind:API'
```

The response will be a JSON object containing an `entities` array,
where each element is an entity matching your query.

```json
{
  "entities": [
    {
      "apiVersion": "swcat.io/v1",
      "kind": "Component",
      "metadata": {
        "name": "my-component",
      },
      "spec": {
        "type": "service",
        "lifecycle": "production",
        "owner": "my-team",
        "system": "my-system"
      }
    }
  ]
}
```

For more details on the query syntax, refer to the
[Search query syntax](../user-guide/query-syntax.md) page.

## Updating annotations

You can update an entity's annotations using a `POST` request to
`/catalog/entities/{entityRef}/annotations/{annotationKey}`.
The new annotation value should be provided in the request body as plain text.

- `entityRef`: The full entity reference (e.g., `component:default/my-component`).
- `annotationKey`: The key of the annotation to update (e.g., `swcat.io/my-annotation`).

**Note:** This operation is not available when the server is running in read-only mode.
Only valid annotation keys and values are accepted.

### Update example

To update the `swcat.io/status` annotation for `component:my-component` to `deployed`:

```bash
curl -X POST 'http://localhost:9191/catalog/entities/component%3Amy-component/annotations/swcat.io%2Fstatus' \
  --data 'deployed'
```
