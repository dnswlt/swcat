# JFrog Xray BOM Plugin

The `JFrogXrayPlugin` automatically retrieves Software Bill of Materials (SBOM) data from JFrog Xray for Docker images and Artifactory artifacts. It extracts a simplified list of dependencies ("MiniBOM") and can optionally detect mismatches between the SBOM and the entity's declared dependencies in `swcat`.

## Configuration

The plugin is configured in `plugins.yml` with `kind: JFrogXrayPlugin`.

### Specification Fields

| Field | Type | Description |
|---|---|---|
| `jfrogUrl` | `string` | **Required**. The base URL of your JFrog instance (e.g., `https://my-company.jfrog.io`). |
| `defaultRepository` | `string` | The default Artifactory repository to search if not specified on the entity. |
| `imageAnnotation` | `string` | Annotation key on the entity containing the Docker image name. Defaults to the entity name. |
| `repositoryAnnotation` | `string` | Annotation key on the entity containing the Artifactory repository name. |
| `auth` | `object` | Authentication settings (see below). |
| `componentsFilter` | `object` | Filters which components from the SBOM to include in the result. |
| `coordsAnnotation` | `string` | Annotation key used to find entity coordinates (GAV) for dependency matching. |
| `targetAnnotation` | `string` | **Required**. The annotation key where the generated `MiniBOM` JSON will be stored. |
| `lintFindingAnnotation` | `string` | Annotation key where dependency mismatch findings will be stored as a `LintFinding` JSON object. |
| `lintIgnoreAnnotation` | `string` | Annotation key containing a JSON list of `groupId:artifactId` strings to ignore during mismatch detection. |

### Authentication (`auth`)

| Field | Type | Description |
|---|---|---|
| `username` | `string` | Basic authentication username. |
| `password` | `string` | Basic authentication password or API key. |
| `mavenServerId` | `string` | If set, the plugin attempts to read credentials from your Maven `settings.xml` for this server ID. Supports environment variable expansion. |
| `mavenSettingsPath` | `string` | Optional path to `settings.xml`. Defaults to `~/.m2/settings.xml`. |

### Components Filter (`componentsFilter`)

| Field | Type | Description |
|---|---|---|
| `types` | `[]string` | CycloneDX component types to include (e.g., `library`, `framework`). |
| `namePattern` | `string` | A regular expression to filter components by name. |

## Example Configuration

```yaml
plugins:
  jfrog-sbom:
    kind: JFrogXrayPlugin
    trigger: "kind:component AND type:service"
    # ...
    spec:
      jfrogUrl: "https://artifactory.example.com"
      defaultRepository: "docker-local"
      imageAnnotation: "example.com/docker-image"
      targetAnnotation: "example.com/sbom"
      lintFindingAnnotation: "swcat/lint-finding"
      auth:
        mavenServerId: ${JFROG_SERVER_ID:-jfrog-instance}
      componentsFilter:
        types: ["library"]
        namePattern: "^(com\.my-company|org\.apache)"
```

## How it Works

1.  **Tag Discovery:** The plugin queries Artifactory for the list of tags for the Docker image.
2.  **Version Selection:** It identifies the latest 3 versions matching semantic versioning (semver).
3.  **SBOM Export:** It requests a CycloneDX SBOM export from JFrog Xray for the latest version.
4.  **Processing:** It filters the SBOM components based on `componentsFilter` and creates a `MiniBOM`.
5.  **Dependency Matching:** If `lintFindingAnnotation` is set, it compares the SBOM components against the entity's `dependsOn`, `consumesApis`, and `providesApis`. Any component found in the SBOM that is also a valid entity in the `swcat` catalog but missing from the entity's declarations will trigger a lint finding.
6.  **Persistence:** The resulting `MiniBOM` and optional findings are saved as annotations.

### MiniBOM Structure

The generated `MiniBOM` stored in the `targetAnnotation` has the following JSON structure:

```json
{
  "name": "image-name:version",
  "components": [
    "groupId:artifactId:version",
    "another-groupId:another-artifactId:version"
  ]
}
```

## Visualization

You can use [Custom Content](../custom-content.md) to display the extracted SBOM data. A simple way to render the entire JSON object is using the `json` style:

```yaml
ui:
  annotationBasedContent:
    example.com/sbom:
      title: "Software Bill of Materials"
      style: json
```
