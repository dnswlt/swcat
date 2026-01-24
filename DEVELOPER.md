# Developer Information

This file contains a collection of information relevant to developers of `swcat`.

## Building

See <https://dnswlt.github.io/swcat/getting-started/> 
(or, locally, [docs/getting-started.md](docs/getting-started.md)).

## Documentation

Documentation is generated using [mkdocs](https://www.mkdocs.org/). 

To run the user guide locally with live reloading:

```bash
.venv/bin/mkdocs serve -w ./docs --livereload
```

If you don't have the virtual environment set up yet, you can create it and install the requirements:

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install mkdocs mkdocs-material
```

## Tags and releases

Releases are only created from tags.

To create and push a tag:

```bash
TAG="v0.4.0"
git tag -a "$TAG" -m "Release version 0.4.0: 

- Add page to generate custom, ad hoc entity diagrams.
"
git push origin "$TAG"
```

To create a release bundle (`.zip`) for Windows:

```bash
make release-windows
```

Then create a release for the new tag on GitHub and upload the generated `.zip`
archive.
