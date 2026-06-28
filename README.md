# Sn
Tin - a static website application written in Go

---

## Project Goals

1. ~Accept the location of a configuration file from an environment variable.~
2. Render and serve markdown files from a directory specified in the configuration file.
3. Provide TLS encryption support with automatic cert updates from LetsEncrypt.
4. Define URL navigation structure via config file, offering specific posts (filtered by characteristics) from different URL schemes.
5. Keep an internal cache of posts and post characteristics, and render from memory as possible.  No database requirement.
6. Provide for templatized rendering per URL scheme.
7. Monitor source directories for changes and update internal cache as appropriate.
8. Provide a mechanism for a web hook to execute pre-configured commands (to pull updates from an external repo, for example).
9. Offer a mountable, access-restricted editing interface that can be used to make updates to the local copy.
10. Performance per page load should be sub-50ms.

## Design Principles

**Self-Contained Application**: Sn is designed to be completely self-contained and must work with both local files and virtual filesystem (git mode). The application uses a virtual filesystem abstraction that allows it to operate seamlessly on local directories or remote git repositories cloned into memory. All functionality, including maintenance, recovery, and administrative operations, is implemented as built-in commands accessible via the main `sn` binary.

**No External Scripts or Utilities**: Do not create external scripts, shell utilities, or tools that require direct filesystem access. Such tools will not work in git mode where files are stored in a virtual filesystem only accessible to the running application. All operations must work through the application's virtual filesystem layer.

## Configuration

Sn is configured via `sn.yaml`. The location of the config file can be set with the `SN_CONFIG` environment variable; if unset, Sn looks for `sn.yaml` in the current working directory.

## Git Mode

Sn can serve content cloned from a git repository into an in-memory virtual filesystem. Set the `SN_GIT_REPO` environment variable to enable this mode.

**Remote repository (HTTPS):**
```sh
SN_GIT_REPO=https://github.com/example/my-site.git ./sn
```

For private repositories, supply credentials via environment variables:
```sh
SN_GIT_USERNAME=myuser SN_GIT_PASSWORD=mytoken \
SN_GIT_REPO=https://github.com/example/my-site.git ./sn
```

**Local repository:**

Use a `file://` URL to clone from a local git repository on disk:
```sh
SN_GIT_REPO=file:///path/to/my-local-repo ./sn
```

The repository must be a valid git repo (i.e. have a `.git` directory). The `sn.yaml` config file is read from the cloned content, so it must exist in the repository root. Authentication environment variables are not required for local repos.
