# Search query syntax

In the list view of each entity kind (components, systems, etc.),
you can search for entities using a simple query language.

## Default search

If you search for a term without specifying an attribute, it will search
within the qualified name of an entity (a combination of its namespace
and name). For example:

```
my-component
```

This will find all entities that contain "my-component" in their qualified name.

## Attribute search

You can also search for entities with specific attributes. The format is `attribute:value`. For example:

```
title:gateway
```

This will find all entities with "gateway" in their title.

The following attributes are available for filtering:

* `*`: Full-text search across all fields (`*:'some thing'`, `*:foo`).
* `meta`: Search in all metadata fields (name, namespace, title, description, labels, annotations, tags, links).
* `kind`: The entity kind (e.g., `component`, `api`, `system`).
* `name`: The name of the entity.
* `namespace`: The namespace of the entity.
* `title`: The title of the entity.
* `description`: The description of the entity.
* `tag`: A tag associated with the entity.
* `label`: A label associated with the entity (searches in `key=value`).
* `annotation`: An annotation associated with the entity (searches in `key=value`).
* `owner`: The owner of the entity.
* `system`: The system that the entity is a part of (for components, apis, resources).
* `domain`: The domain that the entity is a part of (for systems, components, apis, resources).
* `type`: The type of the entity (e.g., for components, apis, groups).
* `lifecycle`: The lifecycle state of the entity (e.g., for components and apis).
* `consumesApis`: An API listed in the `consumesApis` spec of a component.
* `providesApis`: An API listed in the `providesApis` spec of a component.
* `providedBy`: Entities that provide a given API.
* `consumedBy`: Entities that consume a given API.
* `dependsOn`: Entities that a component or resource depends on.
* `dependents`: Entities that depend on a given component or resource.
* `rel`: Entities directly related to the given entity reference (both incoming and outgoing).
    For example, `rel:'component:my-service'` will find the owner,
    the system it belongs to, and any APIs it provides or consumes.
* `lint`: A linting violation severity (`error`, `warn`, `info`) or rule name.

## Operators

The following operators are supported for attribute searches:

* `:` (contains): Checks if the attribute value contains the given search term (case-insensitive).
* `=` (equals): Checks if the attribute value exactly matches the given search term (case-insensitive).
* `~` (regex): Matches the attribute value against a regular expression.

Example with equals:

```
name=my-component
```

Example with regex:

```
name~^my-.*-prod$
```

Square brackets delimit relationship predicates (see below). Quote attribute
values that contain literal brackets, including regular-expression character
classes:

```
name~'^[a-z]+-prod$'
```

## Combining expressions

You can combine multiple expressions using `AND` and `OR`. Parentheses can be used for grouping. If no operator is specified, `AND` is used by default.

Examples:

```
owner:my-team AND tag:production
```

```
owner:team-a OR owner:team-b
```

```
(owner:team-a OR owner:team-b) AND tag:production
```

## Negation

You can negate an expression using `!`.

Example:

```
!owner:my-team
```

## Related-entity predicates

Use `relationship[query]` to find entities with **at least one related entity**
that matches the query inside the brackets. Inside the brackets, attributes refer
to the related entity; outside, they refer to the entity being searched.

For example, on the components page, find components that consume APIs from
domain `payments` but belong to a different domain:

```
!domain=payments AND consumesApis[domain=payments]
```

For a search across all entity kinds, add `kind=component`. Components, APIs, and
resources inherit their domain from their system. This uses their assigned
domain, not its ancestor domains. Use `=` to select an exact qualified domain name, such as
`domain=finance/payments`; `:` would also match names containing that text.
On a domain entity, `domain[...]` tests the domain itself, not its parent
(`subdomainOf`), just as `domain=` matches its own qualified name.

Brackets are supported on `owner`, `system`, `domain`, `consumesApis`,
`providesApis`, `providedBy`, `consumedBy`, `dependsOn`, `dependents`, and `rel`.
Relationship names are case-insensitive. Scalar attributes such as `tag` and
`type` do not support brackets. The existing `consumesApis:payments` form still
matches API reference names; `consumesApis[name:payments]` instead follows those
references and searches the target API's name.

The nested query supports the same terms, operators, grouping, and implicit
`AND` as the outer query. Predicates can themselves contain relationship
predicates:

```
consumesApis[domain=payments AND lifecycle=production]
consumesApis[providedBy[owner=platform-team]]
consumedBy[domain=checkout]
```

These find, respectively, entities consuming a production API in payments,
entities consuming an API provided by a component owned by platform-team, and
APIs consumed by an entity in checkout. Each result entity appears only once,
even if several related entities match.

All conditions in one pair of brackets must match **the same related entity**:

```
consumesApis[domain=payments AND tag=production]
```

Separate predicates may match different APIs:

```
consumesApis[domain=payments] AND consumesApis[tag=production]
```

Negation outside brackets means that no related entity matches. Negation inside
brackets still requires at least one related entity:

```
!consumesApis[domain=payments]
consumesApis[!domain=payments]
```

The first matches entities consuming no APIs in payments, including entities
consuming no APIs at all. The second requires a consumed API outside payments.
A relationship that does not apply to an entity kind has no matches. References
are followed by their full identity, including kind and namespace; unresolved
references do not count as matches.

Brackets must contain a query (`consumesApis[]` is invalid). Each nesting level
follows one relationship; it does not search recursively through the graph.
Comparing attributes of the related entity with attributes of the outer entity
is not supported.
