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

### Deleting observations from all entities

Send a `DELETE` request to `/catalog/observations` to remove observations from
**every** entity that carries them, including entities no longer in the catalog,
selected by exactly one of these query parameters:

- `key`: An exact observation key, e.g. `swcat.io/last-build`.
- `prefix`: A key prefix, e.g. `swcat-deps/` for all keys under it. A prefix
  matches as a plain string: `swcat-deps` would also match `swcat-deps-other`.

Neither may be blank, so a single request cannot delete all observations. Add
`dryRun=true` to see what would be deleted without deleting anything.

```bash
# Preview, then delete, the observations of all tools under "swcat-deps/".
curl -X DELETE 'http://localhost:9191/catalog/observations?prefix=swcat-deps/&dryRun=true'
curl -X DELETE 'http://localhost:9191/catalog/observations?prefix=swcat-deps/'
```

The response lists the affected entities and the keys deleted from each:

```json
{
  "dryRun": false,
  "deleted": [
    {"entity": "component:my-component", "keys": ["swcat-deps/kafka-scanner"]}
  ]
}
```

Nothing matching is not an error: `deleted` is then empty.

## Reporting observed endpoints

!!! tip
    Observed endpoints are shown on the Component's detail page and feed
    swcat's optional dependency lint check. See
    [Observed dependency linting](linting/observed-dependencies.md) for its
    configuration, matching behavior, and guidance on resolving findings.

External tools (for example a message-bus or runtime-traffic scanner) can report
how an entity communicates, as they *observe* it: its **endpoints** (e.g. the
topics it publishes to or subscribes to) and, per endpoint, the
**counterparts** it was found to connect to. These are tentative,
machine-detected facts — distinct from the relationships declared in an entity's
spec — and swcat stores them as a [status observation](#status-observations) on
the source entity under the well-known key `swcat-endpoints/<detectedBy>`.

Send a `POST` request to `/catalog/observed-endpoints` with an
[`ObservedEndpoints`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto)
JSON body:

| Field | Required | Description |
| --- | --- | --- |
| `source` | yes | Reference to the entity whose endpoints are reported. |
| `detectedBy` | yes | Label of the reporting tool. Must be a hyphen-separated identifier of lowercase alphanumeric segments (e.g. `kafka-scanner`), since it forms the observation key `swcat-endpoints/<detectedBy>`. |
| `observedAt` | no | RFC 3339 timestamp of when the endpoints were observed; defaults to "now" when omitted. |
| `version` | no | The version of the source entity the endpoints were observed for. |
| `meta` | no | Additional string key/value metadata. |
| `endpoints` | no | The source's endpoints (see below), including those without any observed counterpart. An empty list clears the endpoints previously reported by this tool. |

Each entry in `endpoints` is an `Endpoint`:

| Field | Required | Description |
| --- | --- | --- |
| `address` | yes | The address as configured, e.g. a topic `orders/${STAGE}/created`. Placeholders and wildcards are kept as they are. |
| `direction` | yes | `ENDPOINT_DIRECTION_INBOUND` (the source receives: subscribes to events, or serves requests) or `ENDPOINT_DIRECTION_OUTBOUND` (the source sends: publishes events, or sends requests). |
| `kind` | no | `ENDPOINT_KIND_EVENT` (the default) or `ENDPOINT_KIND_REQUEST` for requests that expect a reply. |
| `technology` | no | Free text, e.g. `solace` or `kafka`. |
| `names` | no | Tool-specific identifiers, e.g. the names of the bindings that use the address. |
| `counterparts` | no | The other ends this endpoint connects to (see below). |

Each entry in `counterparts` is a `Counterpart`, with exactly one of `entity`
and `external` set:

| Field | Required | Description |
| --- | --- | --- |
| `entity` | * | Reference to the catalog entity at the other end. |
| `external` | * | Display name of an other end that is not in the catalog, e.g. its Maven coordinates. |
| `address` | no | The counterpart's own address that matched, if it differs, e.g. a wildcard subscription `orders/*/created`. |

Each side of a connection is reported by its own source: when A publishes to a
topic that B subscribes to, A's report lists B as a counterpart of its outbound
endpoint, and B's report lists A as a counterpart of its inbound endpoint.
Who depends on whom follows from the endpoint: the source depends on the
counterparts of its inbound event endpoints (it consumes their events) and of
its outbound request endpoints (it calls them).

The `source` and every counterpart `entity` must reference entities that exist
in the catalog; otherwise the request fails with `404 Not Found`. Report other
ends that are not in the catalog as `external`. Malformed input (missing
required fields, an invalid `detectedBy`, or an unknown reference kind) fails
with `400 Bad Request`.

Re-posting with the same `detectedBy` overwrites that tool's previously reported
endpoints, so each tool owns a single `swcat-endpoints/<detectedBy>`
observation. The resulting observation records `<detectedBy>` as its
`producer` and `observedAt` (or the receive time) as its `updatedAt`.

**Note:** Like status observations, this operation is not available when the
server is running in read-only mode, or when the server has no database
configured.

### Reporting example

Report that `flights-search-backend` calls `cache-server`, and consumes the
`flights.updates` topic, which an importer outside the catalog publishes to:

```bash
curl -X POST 'http://localhost:9191/catalog/observed-endpoints' \
  -H 'Content-Type: application/json' \
  -d '{
        "source": {"kind": "Component", "name": "flights-search-backend"},
        "detectedBy": "kafka-scanner",
        "endpoints": [
          {
            "address": "GET /cache/{id}",
            "direction": "ENDPOINT_DIRECTION_OUTBOUND",
            "kind": "ENDPOINT_KIND_REQUEST",
            "counterparts": [{"entity": {"kind": "Component", "name": "cache-server"}}]
          },
          {
            "address": "flights.updates",
            "direction": "ENDPOINT_DIRECTION_INBOUND",
            "technology": "kafka",
            "names": ["flightUpdates-in-0"],
            "counterparts": [{"external": "com.example:flights-importer"}]
          }
        ]
      }'
```

The reported endpoints then appear in the source entity's `status` under the
`swcat-endpoints/kafka-scanner` observation when you query it:

```json
{
  "status": {
    "observations": {
      "swcat-endpoints/kafka-scanner": {
        "value": {
          "endpoints": [
            {
              "address": "GET /cache/{id}",
              "direction": "outbound",
              "kind": "request",
              "counterparts": [
                {"entity": {"kind": "Component", "namespace": "default", "name": "cache-server"}}
              ]
            },
            {
              "address": "flights.updates",
              "direction": "inbound",
              "kind": "event",
              "technology": "kafka",
              "names": ["flightUpdates-in-0"],
              "counterparts": [{"external": "com.example:flights-importer"}]
            }
          ]
        },
        "producer": "kafka-scanner",
        "updatedAt": "2026-06-22T21:00:05Z"
      }
    }
  }
}
```

What is stored is swcat's **internal representation** of the endpoints, not a
verbatim copy of the request payload. The endpoint normalizes the input as it
ingests it:

- `direction` and `kind` are stored as short, stable labels (`inbound`,
  `outbound`; `event`, `request`) rather than the enum names from the request.
  An omitted `kind` is stored as `event`.
- `detectedBy`, `observedAt`, `version` and `meta` are folded into the
  observation envelope (`producer`, `updatedAt`, `version`, `meta`).
- Each counterpart `entity` is resolved to a fully-qualified reference, so its
  `namespace` is filled in (e.g. `default`) even when the request left it out.

### Deleting all endpoints from a tool

When a reporting tool misbehaves and leaves stale endpoints behind, you can
administratively wipe everything it reported with a single `DELETE` request to
`/catalog/observed-endpoints/{detectedBy}`. This removes the
`swcat-endpoints/<detectedBy>` observation from **every** entity that carries
it.

- `detectedBy`: The reporting tool's label (e.g. `kafka-scanner`), same value
  used when the endpoints were reported.

```bash
curl -X DELETE 'http://localhost:9191/catalog/observed-endpoints/solace-graph'
```

Deleting a `detectedBy` that reported nothing is a no-op and still succeeds. The
response reports how many entities were affected.

**Note:** Like the other observation operations, this is not available when the
server is running in read-only mode, or when the server has no database
configured.
