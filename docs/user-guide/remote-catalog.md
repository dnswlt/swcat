# Remote catalogs

`swcat` can connect to a remote catalog (e.g. hosted on GitHub).

This allows you to host `swcat` centrally (on the web, in your company's network, etc.)
and use a remote Git repository as the catalog data source.

When using a remote catalog, `swcat` runs in read-only mode. It clones the repository
into memory and allows you to switch between branches and tags.
(Note the small dropdown that appears at the bottom of the UI.)

This is very useful for things like:

* reviewing pull requests for catalog changes
* viewing the software catalog at different release versions of your software 
    (identified, e.g., by Git tags)

## Configuration

The following flags and corresponding environment variables let you connect
to a remote Git repository:

* `-git-url` (environment variable: `SWCAT_GIT_URL`): The URL of the remote git repository.
* `-git-ref` (`SWCAT_GIT_REF`): Default git ref (branch or tag) to use initially.
  If unset, the repo's default branch (e.g., `main` or `master`) is used.
* `SWCAT_GIT_USER` (only supported via environment variable): an optional username to authenticate with
    when cloning the repository.
* `SWCAT_GIT_PASSWORD` (only supported via environment variable): the optional
  password to authenticate with.
* `-catalog-dir` (`SWCAT_CATALOG_DIR`): path to the directory in the remote repo
  containing the catalog files.
* `-config` (`SWCAT_CONFIG`): path to the `swcat.yml` configuration file in the remote repo.

Note that revision-dependent locations of the catalog and config are not supported:
all git branches and tags must have them at the same location. The catalog files
in the catalog directory may change freely, however.

## Performance Considerations

Because `swcat` clones the repository history into memory (to allow switching between any tag or branch),
very large repositories may consume a significant amount of RAM.
