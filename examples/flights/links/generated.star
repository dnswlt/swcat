def grouped_link(url, group, label, icon=""):
    return link(
        url=url,
        title="{group} ({label})".format(group=group, label=label),
        icon=icon,
        group=group,
        label=label,
    )


def logging_links(entity, annotations):
    if "hexz.me/logging" not in annotations:
        return []

    environment_data = iannotation("hexz.me/log-environments")
    environments = json.decode(environment_data) if environment_data else [
        {"name": "main", "logName": "main"},
    ]
    result = []
    for environment in environments:
        result.append(grouped_link(
            url="https://example.com/logs/{log_name}/{entity_name}".format(
                log_name=environment["logName"],
                entity_name=entity["metadata"]["name"],
            ),
            group="Logs",
            label=environment["name"],
            icon="logs",
        ))
    return result


def api_schema_links(entity, annotations):
    schema_name = annotations.get("hexz.me/asyncapi")
    if schema_name == None:
        return []

    result = []
    for entry in entity["apiSpec"].get("versions", []):
        version = entry["version"]
        group = "API Schema ({version})".format(version=version["rawVersion"])
        for label in ["dev", "prod"]:
            result.append(grouped_link(
                url="https://example.com/repos/apis/{environment}/{schema}_{major}".format(
                    environment=label,
                    schema=schema_name,
                    major=version["major"],
                ),
                group=group,
                label=label,
            ))
    return result


def monitoring_link(entity, system_name):
    if system_name not in ["flights-search", "flights-tickets"]:
        return []

    dashboard = iannotation("hexz.me/monitoring") or entity["metadata"]["name"]
    return [link(
        url="https://grafana.example.com/dashboards/prod/{dashboard}".format(
            dashboard=dashboard,
        ),
        title="Monitoring",
    )]


def kubernetes_link(entity, system_name):
    if system_name != "flights-search":
        return []

    kind = iannotation("apps.kubernetes.io/kind") or "Deployment"
    gvks = {
        "Deployment": "apps~v1~Deployment",
        "StatefulSet": "apps~v1~StatefulSet",
        "DaemonSet": "apps~v1~DaemonSet",
        "ReplicaSet": "apps~v1~ReplicaSet",
        "CronJob": "batch~v1~CronJob",
        "Job": "batch~v1~Job",
        "DeploymentConfig": "apps.openshift.io~v1~DeploymentConfig",
    }
    resource = gvks.get(kind, "core~v1~{kind}".format(kind=kind))
    return [link(
        url="https://k8s.example.com/k8s/ns/{system}-prod/{resource}/pods".format(
            system=system_name,
            resource=resource,
        ),
        title="k8s pods",
    )]


def links(entity):
    annotations = entity["metadata"].get("annotations", {})
    result = []

    if entity["kind"] == "API":
        result.extend(api_schema_links(entity, annotations))

    if entity["kind"] == "Component":
        system_name = entity["componentSpec"]["system"]["name"]
        result.extend(logging_links(entity, annotations))
        result.extend(monitoring_link(entity, system_name))
        result.extend(kubernetes_link(entity, system_name))

    return result
