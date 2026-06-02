# JSON API for Entities

The `/catalog/entities` endpoint provides a JSON API to query and
manipulate entities stored in the catalog.

## Schema

The JSON exchanged by this API is the **Protobuf JSON mapping** of the catalog
schema defined in
[`proto/swcat/catalog/v1/catalog.proto`](https://github.com/dnswlt/swcat/blob/main/proto/swcat/catalog/v1/catalog.proto).
That `.proto` file is the authoritative schema for entities, their specs, and
status observations.

This is **not** the same as the YAML you write to author the catalog. In
particular, the JSON representation follows protobuf's
[JSON conventions](https://protobuf.dev/programming-guides/json/):

- `kind` is the canonical lowercase form (`component`, `system`, `domain`,
  `api`, `resource`, `group`) — not the YAML-cased `Component`/`API`.
- There is no `apiVersion` field.
- The spec is carried in a kind-specific field (`componentSpec`, `systemSpec`,
  `domainSpec`, `apiSpec`, `resourceSpec`, `groupSpec`) rather than a generic
  `spec`.
- References to other entities are objects (`{"kind": ..., "name": ...}`), not
  the shorthand strings used in YAML.
- Field names are `lowerCamelCase` (e.g. `subcomponentOf`, `providesApis`,
  `updatedAt`).

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
  --data-urlencode 'q=kind:component OR kind:api'
```

The response is a JSON object containing an `entities` array, where each
element is an entity encoded according to the schema described above:

```json
{
  "entities": [
    {
      "kind": "component",
      "metadata": {
        "name": "my-component",
        "namespace": "default"
      },
      "componentSpec": {
        "type": "service",
        "lifecycle": "production",
        "owner": {
          "kind": "group",
          "name": "my-team"
        },
        "system": {
          "kind": "system",
          "name": "my-system"
        }
      },
      "status": {
        "observations": {
          "swcat.io/last-build": {
            "value": {
              "status": "green",
              "artifact": "v1.2.3"
            },
            "producer": "ci-pipeline",
            "updatedAt": "2026-06-02T10:00:00Z",
            "version": "1.2.3",
            "meta": {
              "buildUrl": "https://ci.example.com/builds/42"
            }
          }
        }
      }
    }
  ]
}
```

The `status` field holds runtime [status observations](#status-observations)
and is only present when the entity has any. It is never present on `group`
entities.

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

Annotations are persisted alongside the human-authored catalog (in the
extensions sidecar of the YAML files). For short-lived, machine-generated
status, prefer [status observations](#status-observations) instead.

### Update example

To update the `swcat.io/status` annotation for `component:my-component` to `deployed`:

```bash
curl -X POST 'http://localhost:9191/catalog/entities/component%3Amy-component/annotations/swcat.io%2Fstatus' \
  --data 'deployed'
```

## Status observations

Status observations are short-lived, machine-generated status updates attached
to an entity at runtime — for example, information extracted from an entity's
most recent build artifact. Unlike annotations, they are stored in the server's
database rather than the catalog YAML, and they are returned in the `status`
field of an entity (see the query example above).

Each observation is identified by a key (following the annotation naming
conventions, e.g. `swcat.io/last-build`) and carries an
[`Observation`](https://github.com/dnswlt/swcat/blob/main/proto/swcat/catalog/v1/catalog.proto)
payload:

| Field | Required | Description |
| --- | --- | --- |
| `value` | yes | The observation payload, as any JSON value (object, array, ...). |
| `producer` | yes | Identifier of the system/plugin that produced it. |
| `updatedAt` | no | RFC 3339 timestamp; defaults to "now" when omitted. |
| `version` | no | The entity version at which the observation was made. |
| `meta` | no | Additional string key/value metadata for display. |

**Note:** These operations are not available when the server is running in
read-only mode, or when the server has no database configured.

### Writing an observation

Send a `POST` request to
`/catalog/entities/{entityRef}/observations/{key}` with an `Observation` as the
JSON body. An existing observation under the same key is overwritten.

- `entityRef`: The full entity reference (e.g., `component:default/my-component`).
- `key`: The observation key (e.g., `swcat.io/last-build`).

```bash
curl -X POST \
  'http://localhost:9191/catalog/entities/component%3Amy-component/observations/swcat.io%2Flast-build' \
  -H 'Content-Type: application/json' \
  -d '{
        "value": {"status": "green", "artifact": "v1.2.3"},
        "producer": "ci-pipeline",
        "version": "1.2.3",
        "meta": {"buildUrl": "https://ci.example.com/builds/42"}
      }'
```

### Deleting an observation

Send a `DELETE` request to
`/catalog/entities/{entityRef}/observations/{key}`. Deleting a key that does
not exist is a no-op and still succeeds, so the operation is idempotent.

```bash
curl -X DELETE \
  'http://localhost:9191/catalog/entities/component%3Amy-component/observations/swcat.io%2Flast-build'
```
