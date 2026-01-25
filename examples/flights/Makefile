
# Default version to deploy
VERSION ?= 0.1

.PHONY: docs-serve docs-build docs-publish

# Serve the documentation locally.
# By default, mkdocs looks for a 'mkdocs.yml' in the current directory
# and expects documentation source files in the 'docs/' directory.
docs-serve:
	mkdocs serve --livereload

# Build the documentation site (default output is 'site/').
docs-build:
	mkdocs build

# Publish a new version using mike.
# This updates the 'gh-pages' branch (or configured remote).
docs-publish:
	mike deploy --push --update-aliases $(VERSION) latest
