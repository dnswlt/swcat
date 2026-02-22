# Linting

As a software catalog grows, it becomes increasingly important to maintain a high level of data quality. Missing descriptions, missing owners, or incorrect relationships can make the catalog less useful and even misleading.

The `swcat` linter helps you maintain a clean and consistent catalog by automatically checking your entities against a set of rules.

## Concepts

The linter works by evaluating [CEL (Common Expression Language)](https://github.com/google/cel-spec/blob/master/doc/langdef.md) expressions against your entities. CEL is a powerful and efficient language designed for safety and performance.

### Rule definition

A linting rule consists of:

*   **Name:** A unique identifier for the rule (e.g., `has-description`).
*   **Severity:** How critical the violation is. Can be `error`, `warn`, or `info`.
*   **Condition:** (Optional) A CEL expression that determines if the rule should be evaluated for a given entity.
*   **Check:** The CEL expression that validates the entity. If it returns `false`, the rule is considered violated.
*   **Message:** The message shown to the user when the rule is violated.

!!! tip
    For a comprehensive list of CEL examples and how to use them with the `swcat` model, see the [CEL Demo test file](https://github.com/dnswlt/swcat/blob/main/internal/lint/lint_demo_test.go) in the repository.

## Configuration

Linter rules are configured in a `lint.yml` file located in your data root directory.

The file has the following sections:

*   `commonRules`: Rules that are applied to all entities, regardless of their `kind`.
*   `kindRules`: Rules that are applied only to entities of a specific `kind` (e.g., `Component`, `API`, `System`).
*   `reportedGroups`: (Optional) A list of group names (e.g. `team-alpha` or `my-namespace/team-beta`). If set, the global lint findings page will only show these groups as individual cards. All other groups will be grouped under an "Others" section. This is useful for focusing on your own teams in a large catalog with many external owners.

### Example `lint.yml`

```yaml
reportedGroups:
  - team-alpha
  - team-beta
  - external/partner-team

commonRules:
  - name: has-description
    severity: warn
    check: 'metadata.description != ""'
    message: "The entity should have a non-empty description for better documentation."

  - name: has-docs-link
    severity: info
    # CEL 'exists' macro to check for a specific link type
    check: 'metadata.links.exists(l, l.type == "docs")'
    message: "Consider adding a link of type 'docs' for detailed documentation."

kindRules:
  API:
    - name: api-has-provider
      severity: error
      # Inverse relationships are also available!
      check: 'size(spec.providers) > 0'
      message: "The API must have at least one provider component."

  Domain:
    - name: domain-has-org
      severity: error
      check: '"company/org" in metadata.labels'
      message: "The Domain must have a 'company/org' label to indicate ownership."
```

## Viewing Findings

When an entity violates a rule, `swcat` displays the findings in three ways:

1.  **Global Lint Findings Page:** Click the "Bug" icon in the top navigation bar to see a report of all findings in the catalog, grouped by Owner and System.
2.  **Entity Detail Page:** A small indicator icon (yellow exclamation mark) appears in the top toolbar next to the entity's kind. The detailed findings are shown in a foldable section at the bottom of the page.
3.  **Search:** You can find all entities with linting violations using the `lint` search property.

### Searching for violations

You can use the `lint` attribute in your search queries:

*   `lint:error`: Find all entities with at least one "error" severity violation.
*   `lint:warn`: Find all entities with at least one "warn" severity violation.
*   `lint:has-description`: Find all entities that violate the `has-description` rule.

Example: `kind:API AND lint:error`

## The Entity Model

When writing CEL expressions, you have access to the entity's fields as defined in the [catalog.proto](https://github.com/dnswlt/swcat/blob/main/proto/swcat/catalog/v1/catalog.proto) Protobuf definition.

The following variables are available in the CEL environment:

*   `kind`: The entity's kind (string).
*   `metadata`: The entity's metadata (e.g., `metadata.name`, `metadata.labels`).
*   `spec`: The entity's specification (e.g., `spec.type`, `spec.owner`). For kind-specific rules, `spec` is automatically cast to the correct type (e.g., `ApiSpec` for API entities).

### Inverse Relationships

Unlike the static YAML files, the model seen by the linter includes **inverse relationships** that are automatically populated by `swcat`:

*   `ComponentSpec`: `dependents` (entities that depend on this component), `subcomponents`.
*   `ApiSpec`: `providers` (components that provide this API), `consumers` (components that consume this API).
*   `ResourceSpec`: `dependents`.
*   `SystemSpec`: `components`, `apis`, `resources`.
*   `DomainSpec`: `systems`.
*   `GroupSpec`: `children`.

These fields allow you to write powerful rules that enforce architectural standards across your catalog.
