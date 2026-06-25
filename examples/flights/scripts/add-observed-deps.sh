#!/usr/bin/env bash
#
# Populates "observed dependencies" for a few flights components via the swcat
# REST API, so the dependency-candidate lint check produces warnings.
#
# Observed dependencies are tentative, runtime-detected dependencies that an
# external tool (e.g. a traffic or message-bus scanner) reports. swcat stores
# them as "swcat-deps/*" status observations; they cannot be authored in the
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
#   ./add-observed-deps.sh [BASE_URL]
#
# BASE_URL defaults to $SWCAT_URL, then to http://localhost:9191.

set -euo pipefail

BASE_URL="${1:-${SWCAT_URL:-http://localhost:9191}}"
ENDPOINT="${BASE_URL%/}/catalog/observed-dependencies"

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
# so both produce a warning.
post "flights-search-backend" '{
  "source": {"kind": "Component", "name": "flights-search-backend"},
  "detectedBy": "traffic-scanner",
  "dependencies": [
    {"target": {"kind": "Component", "name": "cache-server"},
     "relation": "DEPENDENCY_RELATION_CALLS", "evidence": ["GET /cache/{id}"]},
    {"target": {"kind": "Component", "name": "internal-payment-handler"},
     "relation": "DEPENDENCY_RELATION_CALLS", "evidence": ["POST /internal/payments"]},
    {"target": {"kind": "Component", "name": "availability-aggregator"},
     "relation": "DEPENDENCY_RELATION_CONSUMES", "evidence": ["topic:flights.availability"]}
  ]
}'

# flights-frontend-service reaches purchase-forwarder through the
# flights-purchase-api it consumes (reflected, no warning). It has no edge to
# cache-server, so that observed dependency produces a warning.
post "flights-frontend-service" '{
  "source": {"kind": "Component", "name": "flights-frontend-service"},
  "detectedBy": "traffic-scanner",
  "dependencies": [
    {"target": {"kind": "Component", "name": "purchase-forwarder"},
     "relation": "DEPENDENCY_RELATION_CALLS", "evidence": ["POST /purchases"]},
    {"target": {"kind": "Component", "name": "cache-server"},
     "relation": "DEPENDENCY_RELATION_CALLS", "evidence": ["GET /cache/{id}"]}
  ]
}'

echo
echo "Done. Expect dependency-candidate warnings for:"
echo "  - flights-search-backend  -> internal-payment-handler, availability-aggregator"
echo "  - flights-frontend-service -> cache-server"
echo "Reflected (no warning): cache-server for flights-search-backend, purchase-forwarder for flights-frontend-service."
echo "View them on the entity pages or the lint report at ${BASE_URL%/}/ui/lint."
