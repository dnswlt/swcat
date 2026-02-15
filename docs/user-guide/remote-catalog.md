# Remote catalogs

`swcat` can connect to a remote catalog (e.g. hosted on GitHub).

This allows you to host `swcat` centrally (on the web, in your company's network, etc.)
and use a remote Git repository as the catalog data source.

When using a remote catalog, `swcat` clones the repository into memory and allows you to switch between branches and tags using the dropdown in the footer.

This is very useful for things like:

* Making quick edits to entity metadata directly in the browser (see [Editing Remote Catalogs](#editing-remote-catalogs) below).
* Reviewing pull requests for catalog changes.
* viewing the software catalog at different release versions of your software 
    (identified, e.g., by Git tags).

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
* `-git-root-dir` (`SWCAT_GIT_ROOT_DIR`): path to the directory in the remote repo
  containing the fixed `swcat` structure (`catalog/`, `swcat.yml`, `plugins.yml`).
  Defaults to `.`.

Note that revision-dependent locations of the catalog and config are not supported:
all git branches and tags must have them at the same location. The catalog files
in the catalog directory may change freely, however.

## Editing Remote Catalogs

If you provide Git author information, `swcat` enables remote edit sessions. This allows team members to make small updates directly in the browser without having to clone the repository locally.

### Enabling Edits

To enable editing, provide the following configuration:

* `-git-user-name` (`SWCAT_GIT_USER_NAME`): The name used for Git commits.
* `-git-user-email` (`SWCAT_GIT_USER_EMAIL`): The email address used for Git commits.

When these are set, a "pencil" icon appears in the footer.

### Edit Workflow

1.  **Select a Base Branch:** Use the dropdown in the footer to select the branch you want to update (e.g., `main` or `master`).
2.  **Start a Session:** Click the pencil icon. `swcat` creates a unique local branch (prefixed with `edit/`) in memory.
3.  **Make Changes:** Use the "Edit" buttons on entities to update their metadata. These changes are stored in memory and will be lost if the `swcat` process restarts before they are pushed.
4.  **Push to Remote:** Click the "upload" icon in the footer to push your `edit/` branch to the remote repository. You can do this multiple times as you continue editing.
5.  **Merge Changes:** Once you are done, go to your Git hosting provider (e.g., GitHub or GitLab), create a Pull Request from your `edit/` branch to the base branch, and perform the merge there.
6.  **Close the Session:** Click the "trash" icon in the footer to discard the local edit session and return to the base branch.

**Note:** Discarding a session only removes the branch from `swcat`'s memory. If you have already pushed the branch to the remote, it will remain there and must be deleted manually through your Git provider if no longer needed.

### Limitations

The remote editing feature is designed for low-friction metadata updates (e.g., fixing a description or updating an owner). It has the following limitations:

*   **No Conflict Resolution:** `swcat` does not support complex Git workflows like resolving merge conflicts. If the base branch moves ahead significantly, you may need to handle merges manually outside of `swcat`.
*   **No File Creation:** You cannot currently create new files or domains through the UI. Larger structural changes still require a local development environment.
*   **In-Memory Only:** Local edits are volatile. Always push your changes if you want to preserve them across server restarts.

## Performance Considerations

Because `swcat` clones the repository history into memory (to allow switching between any tag or branch),
very large repositories may consume a significant amount of RAM.
