# JSON API for Entities

The `/catalog/entities` endpoint provides a JSON API to query entities
stored in the catalog. This endpoint allows you to retrieve entities
programmatically, making it suitable for integrations and custom tooling.

## Usage

To query entities, send a `GET` request to `/catalog/entities`.
You can filter the results using the `q` query parameter, which accepts the same
[query syntax](../user-guide/query-syntax.md) as the UI search.

### Example

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
