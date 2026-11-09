# Observed dependency linting

Dependencies declared in catalog YAML describe the intended architecture, but
runtime traffic or messaging can reveal connections that are missing from that
model. External tools can report the endpoints they observe (e.g. the topics a
Component publishes to or subscribes to) and what each endpoint connects to.
swcat can then warn when an observed Component dependency is not reflected in
the catalog.

The workflow is:

1. An external detector reports observed endpoints through the REST API.
2. swcat stores each report as runtime status on the source entity, and shows
   the endpoints on the Component's detail page.
3. The linter compares the Components the source depends on, according to its
   observed endpoints, with the source Component's modeled neighbors.
4. An unmodeled dependency produces a `dependency-candidate` warning.

## Enable the check

Enable dependency reconciliation in `lint.yml`:

```yaml
checkDependencyCandidates: true
```

The check is disabled by default. Reporting observations also requires swcat to
have a database configured and to run without read-only mode enabled.

## How observations are matched

The source depends on the counterparts of its inbound event endpoints (it
consumes their events) and of its outbound request endpoints (it calls them).
Counterparts of its other endpoints depend on the source instead; that
dependency is checked, and reported, on the counterpart's side. Each missing
relationship is therefore reported once, on the dependent Component.

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

Only observations whose source and counterpart are both Components participate
in this check. Counterparts that are Resources, other entity kinds, or outside
the catalog (`external`) are shown on the detail page but ignored by the
dependency linter. When several tools report the same missing target, swcat
produces one finding and combines their tool names and evidence (the addresses
of the endpoints that link the two Components).

## Report observed endpoints

Send a report to `POST /catalog/observed-endpoints`. For example:

```bash
curl -X POST 'http://localhost:9191/catalog/observed-endpoints' \
  -H 'Content-Type: application/json' \
  -d '{
        "source": {"kind": "Component", "name": "flights-search-backend"},
        "detectedBy": "traffic-scanner",
        "endpoints": [
          {
            "address": "POST /internal/payments",
            "direction": "ENDPOINT_DIRECTION_OUTBOUND",
            "kind": "ENDPOINT_KIND_REQUEST",
            "counterparts": [
              {"entity": {"kind": "Component", "name": "internal-payment-handler"}}
            ]
          }
        ]
      }'
```

The source and counterpart entities must already exist in the catalog. A new
report replaces the previous report from the same detector for that source;
reporting an empty `endpoints` list clears it.

See [Reporting observed endpoints](../json-api.md#reporting-observed-endpoints)
for the complete request schema, normalization rules, and administrative delete
endpoint.

## Observed endpoints on the detail page

The Component's detail page shows one "Observed endpoints" table per reporting
tool, sorted by address: the direction of each endpoint, its address, and its
counterparts with links to their pages. A counterpart the catalog declares no
relationship with is marked *not declared* (by the same undirected rule as the
lint check, but on both sides of the connection). Counterparts outside the
catalog are shown by name, without a link. The table starts folded; its heading
shows the number of endpoints and of undeclared counterparts.

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
[`add-observed-endpoints.sh`](https://github.com/dnswlt/swcat/blob/main/examples/flights/scripts/add-observed-endpoints.sh)
script that reports endpoints with both reflected and missing dependencies.
