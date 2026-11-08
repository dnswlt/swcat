#!/usr/bin/env bash
#
# Populates "observed endpoints" for a few flights components via the swcat
# REST API, so their detail pages show an "Observed endpoints" table and the
# dependency-candidate lint check produces warnings.
#
# Observed endpoints are tentative, runtime-detected facts that an external tool
# (e.g. a traffic or message-bus scanner) reports: the endpoints a component
# uses and the counterparts each connects to. swcat stores them as
# "swcat-endpoints/*" status observations; they cannot be authored in the
# catalog YAML, which is why this script exists. With checkDependencyCandidates
# enabled in lint.yml, the linter warns about any observed dependency that is
# neither a declared dependency nor reachable through a shared API.
#
# Prerequisites: a swcat server
#   - serving this flights catalog,
#   - started with a database configured (observations are stored in the DB),
#   - not running in read-only mode.
#
# Usage:
#   ./add-observed-endpoints.sh [BASE_URL]
#
# BASE_URL defaults to $SWCAT_URL, then to http://localhost:9191.

set -euo pipefail

BASE_URL="${1:-${SWCAT_URL:-http://localhost:9191}}"
ENDPOINT="${BASE_URL%/}/catalog/observed-endpoints"

post() {
  local description="$1"
  local body="$2"
  echo "POST ${ENDPOINT} — ${description}"
  curl -sS -X POST "${ENDPOINT}" \
    -H 'Content-Type: application/json' \
    -d "${body}" \
    -w '  -> HTTP %{http_code}\n'
}

# flights-search-backend already reaches cache-server through the
# flights-cache-api it consumes, so that observed dependency is reflected and
# must NOT produce a warning. internal-payment-handler and availability-
# aggregator are neither declared dependencies nor reachable via a shared API,
# so both produce a warning. The last endpoint has no observed counterpart.
post "flights-search-backend" '{
  "source": {"kind": "Component", "name": "flights-search-backend"},
  "detectedBy": "traffic-scanner",
  "endpoints": [
    {"address": "GET /cache/{id}",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_REQUEST",
     "counterparts": [{"entity": {"kind": "Component", "name": "cache-server"}}]},
    {"address": "POST /internal/payments",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_REQUEST",
     "counterparts": [{"entity": {"kind": "Component", "name": "internal-payment-handler"}}]},
    {"address": "flights.availability", "technology": "kafka",
     "direction": "ENDPOINT_DIRECTION_INBOUND", "kind": "ENDPOINT_KIND_EVENT",
     "counterparts": [{"entity": {"kind": "Component", "name": "availability-aggregator"}}]},
    {"address": "flights.search.audit", "technology": "kafka",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_EVENT"}
  ]
}'

# flights-frontend-service reaches purchase-forwarder through the
# flights-purchase-api it consumes (reflected, no warning). It has no edge to
# cache-server, so that observed dependency produces a warning. The tracking
# collector is outside the catalog.
post "flights-frontend-service" '{
  "source": {"kind": "Component", "name": "flights-frontend-service"},
  "detectedBy": "traffic-scanner",
  "endpoints": [
    {"address": "POST /purchases",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_REQUEST",
     "counterparts": [{"entity": {"kind": "Component", "name": "purchase-forwarder"}}]},
    {"address": "GET /cache/{id}",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_REQUEST",
     "counterparts": [{"entity": {"kind": "Component", "name": "cache-server"}}]},
    {"address": "clicks.tracking", "technology": "kafka",
     "direction": "ENDPOINT_DIRECTION_OUTBOUND", "kind": "ENDPOINT_KIND_EVENT",
     "counterparts": [{"external": "com.example:tracking-collector"}]}
  ]
}'

echo
echo "Done. Expect dependency-candidate warnings for:"
echo "  - flights-search-backend  -> internal-payment-handler, availability-aggregator"
echo "  - flights-frontend-service -> cache-server"
echo "Reflected (no warning): cache-server for flights-search-backend, purchase-forwarder for flights-frontend-service."
echo "View them on the entity pages or the lint report at ${BASE_URL%/}/ui/lint."
