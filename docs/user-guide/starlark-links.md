# Starlark links

swcat can automatically add links to catalog entities when it loads a catalog.
Configure Starlark-based link generation under `catalog.starlarkLinks` in
`swcat.yml`; each script receives a matching entity and returns its links.
This is useful when link targets depend on entity data or one entity needs links
for several environments, versions, or related resources.
[Starlark](https://github.com/google/starlark-go) is a deterministic Python-like
configuration language suited to iteration, branching, structured annotations,
and following catalog references.

## Configuration

Each entry under `catalog.starlarkLinks` selects entities with the normal swcat
[query syntax](query-syntax.md) and points to a Starlark file. The file path is
relative to `swcat.yml`.

```yaml
catalog:
  starlarkLinks:
    - filter: |
        kind=component AND domain=payments
      file: links/deployments.star
```

The script must define `links(entity)`. It is called once for every entity that
matches the filter and must return a list of values created by the host-provided
`link` constructor.

```python
def links(entity):
    metadata = entity["metadata"]
    return [
        link(
            url="https://deployments.example.com/{name}".format(
                name=metadata["name"],
            ),
            title="Deployment",
            type="deployment",
        ),
    ]
```

Starlark does not support Python f-strings; use `str.format()` or `%`
interpolation.

## Entity data

`entity` is an immutable dictionary containing the exact protobuf JSON
representation returned by the JSON API. See the [JSON API schema](json-api.md#schema)
for its field names and reference representation; the authoritative definition
is [`catalog.proto`](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto).
ProtoJSON omits unset and default-valued fields, so use dictionary `get` where a
field is optional.

## Host functions

### `link`

```python
link(
    url,
    title="",
    icon="",
    type="",
    group="",
    label="",
)
```

Constructs a typed link result. `url` and every optional argument must be a
string. `group` and `label` must either both be set or both be empty. Grouped
links with the same `group` are displayed together, using `label` for the
individual link.

URLs must be absolute. The repository validates generated links before it
accepts the catalog.

### `lookup_ref`

```python
related_entity = lookup_ref(ref)
```

Resolves any catalog reference and returns the referenced entity in the same
immutable dictionary representation. It returns `None` when the reference does
not resolve.

For example, a Component can obtain its System and Domain through the references
already present in its spec:

```python
system = lookup_ref(entity["componentSpec"]["system"])
domain = lookup_ref(entity["componentSpec"]["domain"])
```

The same function works with owner, API, dependency, and inverse-relationship
references.

### `iannotation`

```python
value = iannotation("swcat/data-environments")
```

Returns an inherited annotation for the current entity. swcat starts at the
entity and follows its parent chain, returning the first defined value. It
returns `None` when no entity in the chain defines the annotation.

Direct annotations need no host function:

```python
annotations = entity["metadata"].get("annotations", {})
app_name = annotations.get("app.kubernetes.io/name")
```

### `json`

The standard Starlark `json` module is available. In particular,
`json.decode(string)` is useful for structured annotation values:

```python
environments = json.decode(iannotation("swcat/data-environments") or "[]")
```

## Generating grouped links

The following script generates one deployment link for every environment held
in an inherited annotation:

```python
def links(entity):
    environments = json.decode(
        iannotation("swcat/data-environments") or "[]"
    )

    result = []
    for entry in environments:
        environment = entry["value"]
        result.append(link(
            url=(
                "https://deployments.{host}/{application}"
            ).format(
                host=environment["host"],
                application=entity["metadata"]["name"],
            ),
            title="Deployment ({label})".format(label=entry["label"]),
            type="deployment",
            group="Deployment",
            label=entry["label"],
        ))

    return result
```

## Failure behavior and restrictions

Starlark link generation is part of catalog loading and is intentionally strict.
The catalog fails to load if:

* the configured file cannot be read or compiled;
* its top-level code or `links(entity)` raises an error;
* it does not define a callable `links` function;
* `links` returns anything other than a list of `link(...)` values;
* a `link` argument has the wrong type or an invalid group/label combination;
* a generated URL is not absolute; or
* the script exceeds its execution-step limit.

Scripts cannot import other files and have no filesystem, network, clock, or
process access.
