# Prometheus Scan

The Prometheus scanner identifies running workloads in your infrastructure (e.g., in Kubernetes) that are not yet registered in your catalog. By comparing the results of a PromQL query with the entities in `swcat`, it can help you ensure that everything running in production has a corresponding owner.

## How it works

The scanner runs a configured PromQL query against a Prometheus or Thanos endpoint on-demand when you trigger it from the Lint Findings page. Each result from the query is expected to represent a running workload. `swcat` then tries to match each workload to an existing entity in the catalog.

If a workload cannot be matched to any entity, it is reported as a linting finding.

## Setup

### 1. Configure the Prometheus Client

The Prometheus client is configured via command-line flags or environment variables when starting `swcat`.

| Flag | Environment Variable | Description |
| :--- | :--- | :--- |
| `-prometheus-url` | `SWCAT_PROMETHEUS_URL` | Base URL of the Prometheus or Thanos REST API. |
| `-prometheus-timeout` | `SWCAT_PROMETHEUS_TIMEOUT` | Maximum time to wait for Prometheus queries (default: `30s`). |
| | `SWCAT_PROMETHEUS_USER` | Username for Basic Auth. |
| | `SWCAT_PROMETHEUS_PASSWORD` | Password for Basic Auth. |
| | `SWCAT_PROMETHEUS_TOKEN` | Bearer token for authentication. |

### 2. Configure the Scan in `lint.yml`

The scanner itself is configured in the `prometheus` section of your `lint.yml` file.

| Field | Type | Description |
| :--- | :--- | :--- |
| `enabled` | `boolean` | Whether the scan is active. **Default: `false`**. |
| `workloadsQuery` | `string` | The PromQL instant query to run to find workloads. |
| `workloadNameLabel` | `string` | The label in the query result that contains the workload name (e.g., `app` or `container`). |
| `displayLabels` | `list` | Labels from the query result to display in the UI (e.g., `namespace`, `cluster`). Each item should have `key` and `label`. |
| `showMetrics` | `boolean` | Whether to show the numeric value of the metric in the findings UI. |
| `excludedWorkloads` | `list` | List of workload names to ignore. |
| `workloadNameAnnotation`| `string` | The entity annotation used to match against the workload name. Defaults to `app.kubernetes.io/name`. |

### Example `lint.yml`

```yaml
prometheus:
  enabled: true
  # Find all containers that have been running in the last hour
  workloadsQuery: 'sum by (app, namespace, cluster) (up{job="kubernetes-pods"})'
  workloadNameLabel: "app"
  displayLabels:
    - key: "namespace"
      label: "Namespace"
    - key: "cluster"
      label: "Cluster"
  showMetrics: false
  excludedWorkloads:
    - "prometheus-server"
    - "kube-state-metrics"
```

## Matching Entities

For each workload found in Prometheus, `swcat` tries to find a matching entity in the catalog:

1.  **By Name:** First, it checks if an entity's name (or `metadata.name`) matches the workload name exactly.
2.  **By Annotation:** If no match is found by name, it checks if any entity has an annotation that matches the workload name. By default, it looks for the `app.kubernetes.io/name` annotation (this can be changed via `workloadNameAnnotation` in `lint.yml`).

### Example with Annotation

If your Prometheus query returns a workload named `my-service-deployment`, but your entity is named `my-service`, you can use an annotation to link them:

```yaml
apiVersion: swcat.dnswlt.io/v1/alpha1
kind: Component
metadata:
  name: my-service
  annotations:
    app.kubernetes.io/name: "my-service-deployment" # Matches the workload name
spec:
  type: service
  owner: team-alpha
```
