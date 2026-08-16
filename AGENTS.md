# Working on swcat

swcat is a software catalog: a Go server reads YAML entities (domains, systems,
components, APIs, resources, groups) from a directory or git repo and serves a
browsable UI with relationship diagrams.

**Scope of this file:** the build, the conventions, and decisions whose rationale
is not obvious from the code. Not a changelog — git history says what changed.
Correct what goes stale rather than appending to it.

## Build and test

```
make build          # go build ./cmd/swcat
make build-web      # vite build -> static/dist (gitignored)
make test           # go test ./... plus the frontend tests
make test-web       # the frontend tests, on node's runner (no dependencies)
make test-integration  # -tags=integration; needs graphviz `dot` on PATH
```

`embed.go` embeds `static/` and `templates/`, so a release runs `make build-web`
first. `-base-dir .` serves both from disk during development.

## Layout

- `internal/catalog` — entity types, refs, validation
- `internal/repo` — the in-memory catalog and its relationship index
- `internal/store` — where entities are read from (disk, git)
- `internal/web` — HTTP handlers, templates, session/edit flows
- `internal/svg` — builds diagrams from the catalog
- `internal/dot` — writes graphviz source and runs `dot`
- `internal/sysview` — lays out and renders the external views in process
- `internal/plugins`, `internal/lint`, `internal/query` — extensions, checks, search

## Frontend

Go `html/template` + HTMX + vanilla JS. No component framework.

- Tailwind utilities go in the markup. `web/style.css` is only for markup we do
  not control: graphviz's SVG output and elements JS creates (`#rich-tooltip`).
  Repeat utilities rather than adding component classes; extract a template
  partial if the duplication becomes real.
- Tailwind scans `templates/`, so a new utility class needs `make build-web`.
- Assets are content-hashed and resolved through the `asset` template function.

**Behavior is keyed to markup, not to pages.** `web/features.js` lists, per
feature, the selector it needs; a page gets a diagram or an editor by containing
one. Adding a feature to a page is a template change. The scan runs again on
every `svgUpdated` event, because a swap can bring in markup the page did not
have — so everything it starts has to be safe to run twice, which is what
`claim()` in `web/dom.js` is for. Only genuinely page-specific behavior (see
`document.body.dataset.page` in `web/diagram.js`) asks what page it is on.

htmx requests that carry view state rebuild their query params from the URL, so
the URL stays the single source of truth: see `onConfigRequest` in
`web/url-params.js`, used by the graph page and the relationship views.

## Diagrams

Every view is an SVG plus a JSON sidecar (`#relationships-svg-meta`) holding
labels, tooltips and URLs. The frontend looks elements up by id, so renderers
must emit:

- node `<g>`: id is the entity ref (`component:ns/name`), class `clickable-node`
- edge `<g>`: id is `svg-edge-N`, class `edge`
- cluster `<g>`: id is `svg-cluster-N`, class `cluster`
- wrapping `<g>`: class `graphviz-svg`, which pins the font

Most views go through graphviz. The **external** views of a system and of a
domain do not: both are two columns of neighbors around a focal entity, so
`internal/sysview` lays them out directly — aligned columns, orthogonal routing,
no subprocess per request. Their **internal** views (what a system or domain is
made of, and how those parts relate) are general graphs, which is what graphviz
is good at, so they stay with `dot`.

Before changing `internal/sysview`:

- Text is measured with the Noto Sans subsets in `internal/sysview/fonts/`, copied
  from what `web/package.json` installs. Keep them in sync or labels outgrow their
  boxes. They are embedded so layout never depends on host fonts.
- Coordinates are points (`width="600pt"` with a matching viewBox), so the browser
  scales by 4/3 as it did for graphviz output.

An external view takes two params: `s=` selects neighbors (none selected = all),
and `detail=` picks a rung of one ladder — `domains`, `systems`, `apis`, `all` —
of which each view offers the part that makes sense for it: a domain view
`domains | systems` (default `systems`), a system view `systems | apis | all`
(default `apis`). Entities a level omits are represented by whatever contains
them, and their edges attach to that container's frame.

## Conventions

- Never commit unless asked to. Finished work stays in the working tree until
  then, so it can still be reviewed and changed.
- Commit subjects are short, imperative and end with a period; the body says why.
- Tests needing `dot`, docker or the network go behind a build tag.
- Unknown YAML config keys are ignored on purpose, so old configs keep loading.
