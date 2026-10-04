# Changelog

All notable changes to swcat are documented in this file. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and swcat follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Releases before
0.17.0 are described on the [GitHub releases page](https://github.com/dnswlt/swcat/releases).

## [Unreleased]

## [0.17.0] - 2026-10-04

### Upgrade notes

- **Square brackets are now query syntax.** Quote attribute values that contain
  `[` or `]`, including regular-expression character classes: write
  `name~'^[a-z]+$'` instead of `name~^[a-z]+$`. This applies to saved searches
  and to every query in configuration: plugin `trigger` and `inhibit`
  predicates, `catalog.starlarkLinks` filters, and legacy
  `catalog.automaticLinks` filters. An unquoted bracket in configuration
  prevents it from loading. Earlier versions accept quoted values too, so you
  can update your configuration before upgrading.
- **Queries in configuration are validated when it loads.** A plugin predicate
  or link filter with an unknown attribute or a malformed regular expression
  now prevents the configuration from loading; previously it silently never
  matched. This includes the `lint` attribute in plugin predicates, which was
  never supported there.
- **The HTTP API rejects invalid queries.** `GET /catalog/entities` returns
  400 for an invalid `q` parameter instead of an empty list. `POST
  /catalog/findings` returns 400 for an invalid `lint.query` that parses but
  cannot be evaluated, such as one with a malformed regular expression,
  instead of a lint section without findings.

### Added

- Relationship predicates in queries: `relationship[query]` matches entities
  with at least one related entity that matches the inner query. For example,
  `!domain=payments consumesApis[domain=payments]` finds entities that consume
  APIs from the payments domain without belonging to it, and
  `systems[components[consumesApis=checkout-api]]` finds domains with a
  system whose components consume `checkout-api`. Supported relationships are
  `owner`, `system`, `domain`, `consumesApis`, `providesApis`, `providedBy`,
  `consumedBy`, `dependsOn`, `dependents`, `subcomponentOf`, `subcomponents`,
  `components`, `apis`, `resources`, `systems`, and `rel`. They work in
  searches, link filters, and plugin predicates. See
  [Related-entity predicates](https://dnswlt.github.io/swcat/user-guide/query-syntax/#related-entity-predicates).
- `subcomponentOf`, `subcomponents`, `components`, `apis`, `resources`, and
  `systems` are also available as plain attributes, as in
  `subcomponentOf=my-service`.

### Changed

- Search pages and the graph builder show an error for an invalid query instead
  of an empty result list. Whether a query is valid no longer depends on the
  catalog's contents.
- The AsyncAPI importer distinguishes request/reply operations from one-way
  ones, and shows AsyncAPI 2.x specs by channel and 3.x specs by operation.
  AsyncAPI observations stored by earlier versions are discarded and fetched
  again on the next plugin run.

### Fixed

- AsyncAPI 3.x references such as `#/channels/...` resolve; previously only
  `#/components/...` references were followed. The `reply` object of 3.x
  operations is parsed.
- AsyncAPI specs with null channels, messages, or references no longer abort
  the importer.
- Diagrams with system boundaries lay out the same way on every render.

[Unreleased]: https://github.com/dnswlt/swcat/compare/v0.17.0...HEAD
[0.17.0]: https://github.com/dnswlt/swcat/compare/v0.16.0...v0.17.0
