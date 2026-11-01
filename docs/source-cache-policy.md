# Source build cache policy

Build and test wrappers using the source-cache resolver select caches in this order: a nonempty process GOCACHE, GOMODCACHE, or npm_config_cache override; the repository-local .cache/source-build-cache.json setting; then the nearest existing SharedTemp directory directly under the repository or one of its ancestors. If no such directory exists, the resolver falls back to Repository/SharedTemp. A configured root must use schemaVersion 1 and an absolute root path. The resolver is pure: it does not create directories or change process environment.

The default cache leaves are go-build for Go compiler artifacts, go-mod for Go module downloads, and npm for npm downloads beneath the selected cache root. The local JSON setting can point multiple repositories at one shared root without machine-specific paths in checked-in scripts. Each wrapper creates directories only when its own workflow requires them. Runtime data, temporary files, and test evidence keep their existing paths.

Offline gates that set GOPROXY=off force their seeded repository-local .go-build/mod module cache, even when an incoming GOMODCACHE is present. Those gates use the shared Go compiler cache, and keep GOWORK, GOPROXY, GOSUMDB, GOTOOLCHAIN, CGO_ENABLED, GOFLAGS, GOTMPDIR, TEMP, TMP, lifecycle guards, and -count=1 behavior unchanged. This preserves offline module reuse without fetching or migrating private dependencies.
Preserve .cache/source-build-cache.json as local configuration when cleaning disposable cache subtrees.
