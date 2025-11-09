# nomad-context

`nomad-context` is a helper CLI that stores multiple Nomad environments (address + ACL token) and transparently injects the matching credentials when you proxy commands to the real `nomad` binary.

## Toolchain

This repo uses [`mise`](https://mise.jdx.dev) to pin Go to 1.24.x. Install the toolchain once via:

```bash
mise install
```

Every time you work on the project, either launch your shell through `mise shell` or prefix commands with `mise exec -- ...` to ensure the configured version of Go is used.

## Usage

```bash
# Save a context (will prompt for the token if omitted)
nomad-context ctx set dev --addr https://nomad.dev.internal:4646 --prompt-token

# Switch between contexts
nomad-context ctx use dev

# List available contexts (current one is marked with *)
nomad-context ctx list

# Inspect the active context
nomad-context ctx show

# Proxy commands to the underlying nomad binary using the active context
nomad-context status jobs
nomad-context job run example.nomad
```

Tokens are stored securely via the platform keyring using `github.com/zalando/go-keyring`, while context metadata lives in `~/.config/nomad-context/config.json` (override with `NOMAD_CONTEXT_HOME`).

Set the `NOMAD_CONTEXT_NOMAD_PATH` environment variable if `nomad` is not on your `PATH`.

## Development

```bash
# Compile the CLI
mise run build

# Run the Go tests
mise run test

# Build release artifacts
mise run release

```

Set `GOCACHE=$(pwd)/.gocache` before running the commands above if your environment restricts writes to the default Go build cache directory.

## Releasing

Releases are driven by tags that match `v*`. To publish:

1. Update `CHANGELOG`/docs as needed and tag the commit: `git tag vX.Y.Z && git push origin vX.Y.Z`.
2. The `Release` GitHub Actions workflow runs [GoReleaser](https://goreleaser.com) to build darwin, linux, and windows artifacts (amd64 + arm64) and attaches them to the GitHub release alongside checksums.

To preview locally without publishing, install GoReleaser (`brew install goreleaser`, `scoop install goreleaser`, etc.) and run `mise run release`, which executes it in snapshot mode. On tag pushes, the `Release` GitHub Actions workflow builds all artifacts, creates the GitHub release, and uploads the tarballs/zips plus checksums automatically.
