# Linting Overview

As a software catalog grows, it becomes increasingly important to maintain a high level of data quality. Missing descriptions, missing owners, or incorrect relationships can make the catalog less useful and even misleading.

The `swcat` linter helps you maintain a clean and consistent catalog by automatically checking your entities and identifying gaps between your catalog and reality.

## Available Linting Features

`swcat` provides three distinct ways to lint your catalog:

### 1. CEL Rules (Entity Validation)
The core linter uses [Common Expression Language (CEL)](cel.md) to validate the metadata and specifications of your entities against a set of predefined rules. This is ideal for enforcing naming conventions, mandatory fields, and architectural standards.

### 2. Prometheus Scan (Workload Matching)
The [Prometheus scanner](prometheus.md) identifies running workloads (e.g., in Kubernetes) that are not yet registered in your catalog. This helps you ensure that everything running in production has a corresponding entity and owner in the catalog.

### 3. Bitbucket Scan (Repository Matching)
The [Bitbucket scanner](bitbucket.md) looks for specific files (like `pom.xml`, `package.json`, or API definitions) in your source code repositories that should have a corresponding entity in the catalog. This ensures that your catalog remains in sync with your codebase.

## Viewing Findings

When an entity violates a rule or a missing entity is identified, `swcat` displays the findings in several ways:

1.  **Global Lint Findings Page:** Click the "Bug" icon in the top navigation bar to see a report of all findings in the catalog, grouped by Owner and System. Note that external scans (Prometheus, Bitbucket) are run on-demand when you click the "Run Scan" button on this page.
2.  **Entity Detail Page:** For CEL rules, a small indicator icon (yellow exclamation mark) appears in the top toolbar next to the entity's kind. Detailed findings are shown at the bottom of the page.
3.  **Search:** You can find entities with CEL violations using the `lint` search property (e.g., `lint:error`).
