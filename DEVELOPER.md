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

## Protocol Buffers

The catalog model and plugin protocols are defined using Protocol Buffers in `proto/`. 
To regenerate the Go code, run:

```bash
make proto
```

This requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` to be installed.

## Creating tags and releases

Releases are only created from tags.

### 1. Create and push the tag

```bash
TAG="v0.4.0"
git tag "$TAG"
git push origin "$TAG"
```

### 2. Create the release

Use the GitHub CLI (`gh`) to create the release from the tag. 
GitHub will automatically generate release notes based on the commit history.

```bash
gh release create "$TAG" --generate-notes
```

(You might have to run `gh auth login` beforehand.)

#### Optional: Attach a Windows release bundle

If you want to include the legacy Windows release bundle (`.zip`), build it first:

```bash
make release-windows
```

And append the generated archive to the `gh release create` command:

```bash
gh release create "$TAG" --generate-notes "swcat-$TAG-windows-amd64.zip"
```

### 3. Verification

Check that the release and its auto-generated notes look as expected on
<https://github.com/dnswlt/swcat/releases>.

Done!
