# Sn
Tin - a static website application written in Go

---

## Project Goals

1. Accept the location of a configuration file from an environment variable.
2. Render and serve markdown files from a directory specified in the configuration file.
3. Provide TLS encryption support with automatic cert updates from LetsEncrypt.
4. Define URL navigation structure via config file, offering specific posts (filtered by characteristics) from different URL schemes.
5. Keep an internal cache of posts and post characteristics, and render from memory as possible.  No database requirement.
6. Provide for templatized rendering per URL scheme.
7. Monitor source directories for changes and update internal cache as appropriate.
8. Provide a mechanism for a web hook to execute pre-configured commands (to pull updates from an external repo, for example).
9. Offer a mountable, access-restricted editing interface that can be used to make updates to the local copy.
10. Performance per page load should be sub-50ms.
