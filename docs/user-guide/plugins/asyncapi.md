# AsyncAPI Importer Plugin

The `AsyncAPIImporterPlugin` retrieves [AsyncAPI](https://www.asyncapi.com/) specifications from a JFrog Artifactory repository, parses the contained channels, and stores a simplified representation as a status observation on the entity.

The plugin uses a built-in JFrog/Maven fetcher: it looks up the entity's Maven coordinates and Artifactory repository via inherited annotations, downloads the matching artifact (a `.zip` or `.jar`), extracts the AsyncAPI YAML file from inside, and parses it.

For [API entities](../catalog/api.md) with declared `spec.versions`, the plugin fetches one matching artifact per declared major version. For other entities (or APIs without versions), it fetches the latest release.

## Configuration

The plugin is configured in `plugins.yml` with `kind: AsyncAPIImporterPlugin`.

### Specification Fields

| Field | Type | Description |
|---|---|---|
| `file` | `string` | **Required**. Path of the AsyncAPI spec file inside the fetched archive. |
| `fetcher` | `object` | **Required**. JFrog/Maven fetcher configuration (see below). |

### Fetcher (`fetcher`)

| Field | Type | Description |
|---|---|---|
| `packaging` | `string` | **Required**. Artifact packaging / extension, e.g. `jar` or `zip`. |
| `classifier` | `string` | Optional Maven classifier (dash-appended to the artifact file name). |
| `replaceProperties` | `bool` | If `true`, all `.properties` files in the artifact are read and used to substitute `@@placeholder@@` tokens in the AsyncAPI spec before parsing. |

### Required Entity Annotations

The plugin resolves the artifact to download from annotations on the entity. Annotations are inherited from parent entities (e.g. a `groupId` set on a System applies to all its Components and APIs).

| Annotation | Description |
|---|---|
| `maven.apache.org/groupId` | **Required**. Maven `groupId` of the artifact. Typically defined once on a System and inherited. |
| `jfrog.com/repository` | **Required**. Name of the Artifactory repository to search. Also typically inherited. |
| `maven.apache.org/coords` | Optional. Full or partial Maven GAV (`groupId:artifactId:version`). Any field set here overrides the values resolved from `groupId` / the entity name. Setting `version` here pins the entity to a specific version (only used for non-API entities or APIs without declared versions). |

If `maven.apache.org/coords` is not set, the entity's name is used as the `artifactId`.

### Example Configuration

```yaml
plugins:
  asyncapi-importer:
    kind: AsyncAPIImporterPlugin
    trigger: "kind:API AND type:kafka"
    spec:
      file: "META-INF/asyncapi.yaml"
      fetcher:
        packaging: jar
        classifier: asyncapi
```

A matching API entity might look like:

```yaml
apiVersion: backstage.io/v1alpha1
kind: API
metadata:
  name: order-events
  annotations:
    # groupId and repository are typically inherited from the parent System
    maven.apache.org/groupId: com.example.orders
    jfrog.com/repository: maven-releases
spec:
  type: kafka
  versions:
    - name: v1
      lifecycle: production
    - name: v2
      lifecycle: experimental
```

## How it Works

1.  **Resolve coordinates:** Reads `maven.apache.org/groupId`, `jfrog.com/repository`, and (optionally) `maven.apache.org/coords` from the entity, walking up the parent chain. The `artifactId` defaults to the entity name.
2.  **List versions:** Queries Artifactory for all release versions of the artifact.
3.  **Pick targets:**
    *   For API entities with `spec.versions`: for each declared version, picks the latest available artifact matching the same major version.
    *   Otherwise: uses the version pinned via `maven.apache.org/coords`, or falls back to the latest semver release.
4.  **Reuse cache:** Versions whose channels were fetched in a previous run are reused from the existing status observation; only new versions are downloaded.
5.  **Download & parse:** Retrieves each missing artifact, opens it as a zip, reads the file at `spec.file`, optionally substitutes `@@placeholders@@` from bundled `.properties` files, and parses the AsyncAPI spec (v2 and v3 supported).
6.  **Lint:** If a newer major version exists in the repository that is not declared on the entity, emits a lint finding.
7.  **Persist:** Writes the channels and the optional finding as status observations.

## Output

The plugin writes two status observations:

| Observation | Content |
|---|---|
| `swcat-plugins/asyncapi-channels` | Array of `VersionedChannels`, one per resolved version. |
| `swcat-lint/finding-newer-version` | A `LintFinding` describing a newer major version available in the repository but not listed on the entity. Only present when such a version exists. |

`VersionedChannels` JSON structure:

```json
[
  {
    "version": "1.4.2",
    "channels": [
      {
        "name": "orderCreated",
        "address": "orders.created.v1",
        "messages": ["OrderCreated"]
      }
    ]
  }
]
```

For API entities with declared versions, the observation's `meta` map records the resolution from each declared `RawVersion` to the concrete repository version (keys are prefixed with `version-`, e.g. `version-v1` → `1.4.2`). For other entities, the single resolved version is reported in the observation's `version` field instead.

## Visualization

The output is stored as a status observation, so it is rendered via `ui.statusBasedContent` (see [Custom Content](../custom-content.md)):

```yaml
ui:
  statusBasedContent:
    swcat-plugins/asyncapi-channels:
      heading: "Message Channels"
      template: |
        {{ range . }}
          <h4>Version {{ .version }}</h4>
          <table>
            <tr><th>Channel</th><th>Address</th><th>Messages</th></tr>
            {{ range .channels }}
              <tr>
                <td>{{ .name }}</td>
                <td>{{ .address }}</td>
                <td>{{ range .messages }}{{ . }} {{ end }}</td>
              </tr>
            {{ end }}
          </table>
        {{ end }}
```
