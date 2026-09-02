# JSON API for Entities

The `/catalog/entities` endpoint provides a JSON API to query and
manipulate entities stored in the catalog.

## Schema

The JSON exchanged by this API is the **Protobuf JSON mapping** of the catalog
schema defined in
[`catalog.proto`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto).
That `.proto` file is the authoritative schema for entities, their specs, and
status observations.

This is **not** the same as the YAML you write to author the catalog. In
particular, the JSON representation follows protobuf's
[JSON conventions](https://protobuf.dev/programming-guides/json/):

- `kind` uses the canonical proper-case form (`Component`, `System`, `Domain`,
  `API`, `Resource`, `Group`) — the same casing as the YAML `kind:` field. The
  lowercase form (`component`, `api`, …) appears only inside entity *reference
  strings* such as `component:default/my-component` (e.g. in URL paths).
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

The response body is a
[`ListEntitiesResponse`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto):
a JSON object containing an `entities` array, where each element is an entity
encoded according to the schema described above:

```json
{
  "entities": [
    {
      "kind": "Component",
      "metadata": {
        "name": "my-component",
        "namespace": "default"
      },
      "componentSpec": {
        "type": "service",
        "lifecycle": "production",
        "owner": {
          "kind": "Group",
          "name": "my-team"
        },
        "system": {
          "kind": "System",
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
[`Observation`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto)
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

## Reporting observed dependencies

!!! tip
    This endpoint supplies observations to swcat's optional dependency lint
    check. See [Observed dependency linting](linting/observed-dependencies.md)
    for configuration, matching behavior, and guidance on resolving findings.

External tools (for example a runtime-traffic or message-bus scanner) can report
the dependencies they *observe* between entities. These are tentative,
machine-detected dependencies — distinct from the dependencies declared in an
entity's spec — and swcat stores them as a
[status observation](#status-observations) on the source entity under the
well-known key `swcat-deps/<detectedBy>`.

Send a `POST` request to `/catalog/observed-dependencies` with an
[`ObservedDependencies`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto)
JSON body:

| Field | Required | Description |
| --- | --- | --- |
| `source` | yes | Reference to the entity the dependencies originate from. |
| `detectedBy` | yes | Label of the reporting tool. Must be a hyphen-separated identifier of lowercase alphanumeric segments (e.g. `kafka-scanner`), since it forms the observation key `swcat-deps/<detectedBy>`. |
| `observedAt` | no | RFC 3339 timestamp of when the dependencies were observed; defaults to "now" when omitted. |
| `dependencies` | no | The list of observed dependencies (see below). An empty list clears the dependencies previously reported by this tool. |

Each entry in `dependencies` is an `ObservedDependency`:

| Field | Required | Description |
| --- | --- | --- |
| `target` | yes | Reference to the entity that `source` depends on. |
| `relation` | no | The nature of the dependency: `DEPENDENCY_RELATION_CALLS` (synchronous calls, e.g. REST/gRPC/GraphQL), `DEPENDENCY_RELATION_PRODUCES` (sends messages/events to the target), or `DEPENDENCY_RELATION_CONSUMES` (receives messages/events from the target). Defaults to a generic dependency when omitted. |
| `evidence` | no | Strings describing what the detection was based on, e.g. topic names or an RPC `Service.Method`. |

Both `source` and every `target` must reference entities that exist in the
catalog; otherwise the request fails with `404 Not Found`. Malformed input
(missing required fields, an invalid `detectedBy`, or an unknown reference kind)
fails with `400 Bad Request`.

Re-posting with the same `detectedBy` overwrites that tool's previously reported
dependencies, so each tool owns a single `swcat-deps/<detectedBy>` observation.
The resulting observation records `external/<detectedBy>` as its `producer` and
the receive time as its `updatedAt`.

**Note:** Like status observations, this operation is not available when the
server is running in read-only mode, or when the server has no database
configured.

### Reporting example

Report that `flights-search-backend` calls `cache-server` and consumes events
from `flights-inmem-cache`:

```bash
curl -X POST 'http://localhost:9191/catalog/observed-dependencies' \
  -H 'Content-Type: application/json' \
  -d '{
        "source": {"kind": "Component", "name": "flights-search-backend"},
        "detectedBy": "kafka-scanner",
        "dependencies": [
          {
            "target": {"kind": "Component", "name": "cache-server"},
            "relation": "DEPENDENCY_RELATION_CALLS",
            "evidence": ["GET /cache/{id}"]
          },
          {
            "target": {"kind": "Resource", "name": "flights-inmem-cache"},
            "relation": "DEPENDENCY_RELATION_CONSUMES",
            "evidence": ["topic:flights.updates"]
          }
        ]
      }'
```

The reported dependencies then appear in the source entity's `status` under the
`swcat-deps/kafka-scanner` observation when you query it:

```json
{
  "status": {
    "observations": {
      "swcat-deps/kafka-scanner": {
        "value": {
          "detectedBy": "kafka-scanner",
          "observedAt": "2026-06-22T21:00:00Z",
          "dependencies": [
            {
              "target": {"kind": "Component", "namespace": "default", "name": "cache-server"},
              "relation": "calls",
              "evidence": ["GET /cache/{id}"]
            },
            {
              "target": {"kind": "Resource", "namespace": "default", "name": "flights-inmem-cache"},
              "relation": "consumes",
              "evidence": ["topic:flights.updates"]
            }
          ]
        },
        "producer": "external/kafka-scanner",
        "updatedAt": "2026-06-22T21:00:05Z"
      }
    }
  }
}
```

What is stored is swcat's **internal representation** of the dependency
candidates, not a verbatim copy of the request payload. The endpoint normalizes
the input as it ingests it, so the stored observation differs from what you sent
in a few expected ways:

- `relation` is stored as a short, stable label (`dependency`, `calls`,
  `produces`, `consumes`) rather than the `DEPENDENCY_RELATION_*` enum name from
  the request. An omitted or unspecified relation is stored as `dependency`.
- `detectedBy` and `observedAt` are folded into the observation envelope as the
  server-assigned `producer` (`external/<detectedBy>`) and `updatedAt` (the
  receive time), while `observedAt` is also retained in the value (defaulted to
  "now" when the request omitted it).
- Each `target` is resolved to a fully-qualified reference, so its `namespace`
  is filled in (e.g. `default`) even when the request left it out.

### Deleting all dependencies from a tool

When a reporting tool misbehaves and leaves stale dependencies behind, you can
administratively wipe everything it reported with a single `DELETE` request to
`/catalog/observed-dependencies/{detectedBy}`. This removes the
`swcat-deps/<detectedBy>` observation from **every** entity that carries it.

- `detectedBy`: The reporting tool's label (e.g. `kafka-scanner`), same value
  used when the dependencies were reported.

```bash
curl -X DELETE 'http://localhost:9191/catalog/observed-dependencies/solace-graph'
```

Deleting a `detectedBy` that reported nothing is a no-op and still succeeds. The
response reports how many entities were affected.

**Note:** Like the other observation operations, this is not available when the
server is running in read-only mode, or when the server has no database
configured.
