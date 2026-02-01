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
* `name`: The name of the entity.
* `namespace`: The namespace of the entity.
* `title`: The title of the entity.
* `description`: The description of the entity.
* `tag`: A tag associated with the entity.
* `label`: A label associated with the entity (searches in `key=value`).
* `annotation`: An annotation associated with the entity (searches in `key=value`).
* `owner`: The owner of the entity.
* `system`: The system that the entity is a part of (for components, apis, resources).
* `type`: The type of the entity (e.g., for components, apis, groups).
* `lifecycle`: The lifecycle state of the entity (e.g., for components and apis).
* `consumesApis`: An API listed in the `consumesApis` spec of a component.
* `providesApis`: An API listed in the `providesApis` spec of a component.
* `rel`: Entities directly related to the given entity reference (both incoming and outgoing). For example, `rel:component:my-service` will find the owner, the system it belongs to, and any APIs it provides or consumes.

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
