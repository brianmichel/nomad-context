# nomad-context

`nomad-context` is a helper CLI that stores multiple Nomad environments and transparently injects the matching credentials when you proxy commands to the real `nomad` binary. Contexts can use a manually supplied ACL token or the Nomad CLI's SSO login flow.

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

# Configure an SSO context and log in through the configured Nomad auth method
nomad-context ctx set corp --addr https://nomad.corp.internal:4646 --auth-method corp-oidc
nomad-context ctx login corp

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

The `--auth-method` value is the ACL auth-method name configured by the Nomad cluster administrator. `ctx login` runs `nomad login -json` against that context, and opens the provider's browser flow when required. The optional `--oidc-callback-addr` flag can be passed to `ctx login` if the default callback address is unavailable; the address must also be allowed in the Nomad auth-method and identity-provider configuration. Nomad's CLI login flow requires a compatible Nomad CLI (the login command is documented for Nomad 1.8 and later).

Manually supplied and SSO-issued Nomad ACL tokens are stored securely via the platform keyring using `github.com/zalando/go-keyring`. Context metadata—including address, auth method, and known token expiry—lives in `~/.config/nomad-context/config.json` (override with `NOMAD_CONTEXT_HOME`). Tokens are never written to that config file. Nomad connection settings such as `NOMAD_REGION` continue to be read from the environment. When an SSO token is known to have expired, proxied commands stop and ask you to run `nomad-context ctx login <name>` again; login is not started unexpectedly during a regular Nomad command.

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

## Installation

I currently use `mise` with the experimental github backend to install and use this tool with:

```bash
mise use -g github:brianmichel/nomad-context@0.0.1
```

In the near future I'm going to try to publish to:

- Homebrew
- Scoop

## Releasing

Releases are driven by publishing a new tag. To publish:

1. Update `CHANGELOG`/docs as needed and tag the commit: `git tag vX.Y.Z && git push origin vX.Y.Z`.
2. The `Release` GitHub Actions workflow runs [GoReleaser](https://goreleaser.com) to build darwin, linux, and windows artifacts (amd64 + arm64) and attaches them to the GitHub release alongside checksums.

To preview locally without publishing, install GoReleaser (`brew install goreleaser`, `scoop install goreleaser`, etc.) and run `mise run release`, which executes it in snapshot mode. On tag pushes, the `Release` GitHub Actions workflow builds all artifacts, creates the GitHub release, and uploads the tarballs/zips plus checksums automatically.
