# Observed dependency linting

Dependencies declared in catalog YAML describe the intended architecture, but
runtime traffic or messaging can reveal connections that are missing from that
model. External tools can report these observations to swcat, which can then
warn when an observed Component dependency is not reflected in the catalog.

The workflow is:

1. An external detector reports dependencies through the REST API.
2. swcat stores each report as runtime status on the source entity.
3. The linter compares observed Component targets with the source Component's
   modeled neighbors.
4. An unmodeled target produces a `dependency-candidate` warning.

## Enable the check

Enable dependency reconciliation in `lint.yml`:

```yaml
checkDependencyCandidates: true
```

The check is disabled by default. Reporting observations also requires swcat to
have a database configured and to run without read-only mode enabled.

## How observations are matched

For an observed dependency from Component A to Component B, the relationship is
considered reflected when any of the following is present in the catalog:

* A declares B in `dependsOn`.
* B declares A in `dependsOn`.
* A and B are connected as provider and consumer of the same API.

The comparison is deliberately undirected. Its purpose is to identify a
possibly missing catalog relationship, not to validate its direction or exact
type.

| Observation | Catalog model | Result |
| --- | --- | --- |
| A calls B | A depends on B | Reflected |
| A calls B | B depends on A | Reflected |
| A calls B | A and B share an API provider/consumer relationship | Reflected |
| A calls B | No relationship connects A and B | `dependency-candidate` warning |

Only observations whose source and target are both Components participate in
this check. Observations involving Resources or other entity kinds are stored
but currently ignored by the dependency linter. The reported `relation`
(`calls`, `produces`, or `consumes`) is also preserved but does not affect the
comparison. When several tools report the same missing target, swcat produces
one finding and combines their tool names and evidence.

## Report observed dependencies

Send a report to `POST /catalog/observed-dependencies`. For example:

```bash
curl -X POST 'http://localhost:9191/catalog/observed-dependencies' \
  -H 'Content-Type: application/json' \
  -d '{
        "source": {"kind": "Component", "name": "flights-search-backend"},
        "detectedBy": "traffic-scanner",
        "dependencies": [
          {
            "target": {"kind": "Component", "name": "internal-payment-handler"},
            "relation": "DEPENDENCY_RELATION_CALLS",
            "evidence": ["POST /internal/payments"]
          }
        ]
      }'
```

The source and targets must already exist in the catalog. A new report replaces
the previous report from the same detector for that source; reporting an empty
`dependencies` list clears it.

See [Reporting observed dependencies](../json-api.md#reporting-observed-dependencies)
for the complete request schema, normalization rules, and administrative delete
endpoint.

## Resolve a finding

A `dependency-candidate` warning means that an external tool observed a target
Component that swcat could not find among the source's modeled neighbors. Check
the supplied evidence, then do whichever reflects reality:

* add the dependency to the appropriate Component's `dependsOn` list;
* model the interaction through the relevant API provider and consumer;
* correct the detector and submit a replacement report if the observation was
  wrong or stale.

Findings appear on the source Component's detail page and in the global Lint
Findings page. They can also be found with the search query
`lint:dependency-candidate`.

## Runnable example

The flights example enables this check in
[`examples/flights/lint.yml`](https://github.com/dnswlt/swcat/blob/main/examples/flights/lint.yml)
and includes an
[`add-observed-deps.sh`](https://github.com/dnswlt/swcat/blob/main/examples/flights/scripts/add-observed-deps.sh)
script that reports both reflected and missing dependencies.
