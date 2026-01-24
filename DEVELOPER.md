# Developer Information

This file contains a collection of information relevant to developers of `swcat`.

## Building

See <https://dnswlt.github.io/swcat/getting-started/> 
(or, locally, [docs/getting-started.md](docs/getting-started.md)).

## Updating documentation

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

## Creating tags and releases

Releases are only created from tags.

### 1. Create and push the tag

```bash
TAG="v0.4.0"
git tag -a "$TAG" -m "Release version $TAG: 

- Add page to generate custom, ad hoc entity diagrams.
"
git push origin "$TAG"
```

### 2. Create the Windows release bundle

To create a release bundle (`.zip`) for Windows:

```bash
make release-windows
```

### 3. Create the release

Use the GitHub CLI (`gh`) to create the release from the tag and upload
the generated `.zip` archive:

```bash
gh release create "$TAG" --notes-from-tag "swcat-$TAG-windows-amd64.zip"
```

(You might have to run `gh auth login` beforehand.)

Check that the release look as expected on
<https://github.com/dnswlt/swcat/releases>.

Done!
