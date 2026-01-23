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
