# Bitbucket Scan

The Bitbucket scanner identifies source code repositories (or, for monorepos, directories in those repositories) in your Bitbucket Data Center instance that should have a corresponding entity in the catalog but are currently missing.

## How it works

The scanner iterates through configured Bitbucket projects and searches for specific files (e.g., `pom.xml`, `package.json`, or API definitions). If a file is found, `swcat` tries to match it against the existing entities in your catalog.

If a file is found in Bitbucket but no matching entity exists in the catalog, `swcat` reports it as a linting finding.

When configured and enabled, the scanner and its results are shown on the Lint Findings page.

## Setup

### 1. Configure the Bitbucket Client

The Bitbucket client is configured via command-line flags or environment variables when starting `swcat`.

| Flag | Environment Variable | Description |
| :--- | :--- | :--- |
| `-bitbucket-url` | `SWCAT_BITBUCKET_URL` | Base URL of your Bitbucket Data Center instance (e.g., `https://bitbucket.example.com`). |
| | `SWCAT_BITBUCKET_USER` | Username for authentication. |
| | `SWCAT_BITBUCKET_PASSWORD` | Password for authentication. |
| | `SWCAT_BITBUCKET_TOKEN_<PROJECT>` | Personal Access Token for a specific project (e.g., `SWCAT_BITBUCKET_TOKEN_MYPROJ`). |

### 2. Configure the Scan in `lint.yml`

The scanner is configured in the `bitbucket` section of your `lint.yml` file.

| Field | Type | Description |
| :--- | :--- | :--- |
| `enabled` | `boolean` | Whether the scan is active. **Default: `false`**. |
| `projects` | `list` | List of Bitbucket project keys to scan (e.g., `["MYPROJ", "CORE"]`). |
| `excludedRepos` | `list` | List of regex patterns of repository names to exclude from the scan. |
| `queries` | `list` | List of search queries to run (see below). |

#### Bitbucket Queries

Each query in the `queries` list has the following fields:

| Field | Type | Description |
| :--- | :--- | :--- |
| `kind` | `string` | The kind of entity expected to find (`Component` or `API`). |
| `path` | `string` | The full path of the file to look for relative to the repository root. |
| `pathRegex` | `string` | A regular expression to match against all files in a repository (potentially expensive). |
| `repositories` | `list` | (Optional) Limit this query to a specific list of repository slugs. |

### Example `lint.yml`

```yaml
bitbucket:
  enabled: true
  projects:
    - "PROJECT_A"
    - "PLATFORM"
  excludedRepos:
    - ".*-test"
    - "sandbox-.*"
  queries:
    - kind: Component
      path: "pom.xml" # Match Java projects
    - kind: Component
      path: "package.json" # Match Node.js projects
    - kind: API
      pathRegex: ".*\.yaml" # Match potential OpenAPI/AsyncAPI files
      repositories:
        - "api-gateway"
        - "service-specs"
```

## Matching Entities

An entity is considered a match for a found file if it has a link that points to the repository or a **parent directory** of the found file.

`swcat` only considers links of type `code` or `bitbucket` for matching. Both explicit links defined in the entity's `metadata.links` and auto-generated links from annotations (e.g., using `annotationBasedLinks` in your `swcat.yml` configuration) are evaluated.

### Example

If the Bitbucket scanner finds `src/main/resources/api.yaml` in the repository `PROJECT_A/my-service`, the following entity would be a match because its link points to a parent directory (the root of the repo) in Bitbucket:

```yaml
apiVersion: swcat.dnswlt.io/v1/alpha1
kind: Component
metadata:
  name: my-service
  links:
    - type: code
      url: https://bitbucket.example.com/projects/PROJECT_A/repos/my-service/browse
spec:
  type: service
  owner: team-alpha
```
