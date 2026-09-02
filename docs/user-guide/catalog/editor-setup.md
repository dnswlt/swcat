# Editor setup

swcat publishes a
[JSON Schema](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.schema.json)
for catalog YAML files. Associate the schema with the entity files in a catalog
repository to enable validation, completion, and hover documentation.

## VS Code

Install the
[YAML extension by Red Hat](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml),
then add the following to `.vscode/settings.json` in the catalog repository:

```json
{
  "yaml.schemas": {
    "https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.schema.json": [
      "catalog/**/*.yml",
      "catalog/**/*.yaml"
    ]
  }
}
```

Adjust the two file patterns if the entity files are stored somewhere other
than `catalog/`. To recommend the YAML extension automatically to everyone who
opens the repository, also add `.vscode/extensions.json`:

```json
{
  "recommendations": ["redhat.vscode-yaml"]
}
```

## IntelliJ IDEA

1. Open **Settings** (or **Preferences** on macOS) and select
   **Languages & Frameworks → Schemas and DTDs → Remote JSON Schemas**. Enable
   **Allow downloading JSON schemas from remote sources**.
2. Select **Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings**
   and add a mapping named `swcat catalog`.
3. Set **Schema file or URL** to
   `https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.schema.json`
   and select JSON Schema version 7.
4. Add the repository's `catalog` directory to the mapping. If catalog entities
   are stored elsewhere, add those directories or `*.yml` and `*.yaml` file
   patterns instead.

Both setups apply the schema to every YAML document in the selected files,
including catalog files containing multiple documents separated by `---`.

## REST API schema

The catalog JSON Schema describes the YAML files used to author a catalog. The
JSON returned by the REST API instead follows the
[catalog Protobuf schema](https://dnswlt.github.io/swcat/schema/swcat/catalog/v1/catalog.proto).
See the [JSON API documentation](../json-api.md) for details.
