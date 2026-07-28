# `oc-ols` CLI — distribution (how)

How the CLI binary is built, versioned, and published for end-user download.

**Ticket:** [OLS-1062](https://redhat.atlassian.net/browse/OLS-1062)

---

## Overview

Pre-built `oc-ols` binaries are published to a rolling `latest` GitHub Release via GoReleaser and GitHub Actions. The binary is an `oc` CLI plugin: the `oc-` name prefix lets `oc` auto-discover it as `oc ols <subcommand>` when placed in `$PATH`. It also works standalone (`./oc-ols ask "question"`) without `oc` installed.

---

## Build targets

| OS | Architecture |
|---|---|
| linux | amd64 |
| linux | arm64 |
| darwin | amd64 |
| darwin | arm64 |

---

## Release model

- **Trigger:** push to `main`, path-filtered (see below).
- **Versioning:** no semver tags. Version derived from `git describe --tags --always` (e.g. `v0.0.0-12-gabc1234`), injected via ldflags into `cli.Version`.
- **Build step:** GoReleaser runs in `--snapshot` mode to cross-compile binaries and produce archives. Snapshot mode does not publish to GitHub Releases.
- **Upload step:** A separate GitHub Actions step creates or updates a rolling `latest` GitHub Release and uploads the snapshot artifacts. This is the same two-step pattern used by the `oc-agentic` CLI in `lightspeed-agentic-operator`.
- **GitHub Release:** single release named `latest`, overwritten on each build. Stable download URLs.
- **Artifacts:** 4 tarballs (`oc-ols_{os}_{arch}.tar.gz`) + `checksums.txt` (SHA256).

---

## Path filter

The workflow triggers only when changes touch CLI-relevant paths:

```text
cmd/oc-ols/**
cli/**
go.mod
go.sum
.goreleaser.yaml
.github/workflows/release-cli.yml
```

---

## Key files

| File | Role |
|---|---|
| `.goreleaser.yaml` | Build config: targets `cmd/oc-ols`, cross-compiles 4 platforms, ldflags version injection, tarball archives, SHA256 checksums |
| `.github/workflows/release-cli.yml` | GitHub Actions workflow: path-filtered push to main, runs GoReleaser snapshot, uploads artifacts to rolling `latest` release |

---

## Version injection

`cli/version.go` declares `var Version = "dev"`. GoReleaser overrides at build time:

```yaml
builds:
  - main: ./cmd/oc-ols
    ldflags:
      - -X github.com/openshift/lightspeed-operator/cli.Version={{ .Summary }}
```

---

## Installation

```bash
# Linux amd64
curl -L https://github.com/openshift/lightspeed-operator/releases/latest/download/oc-ols_linux_amd64.tar.gz | tar xz
sudo mv oc-ols /usr/local/bin/

# macOS Apple Silicon
curl -L https://github.com/openshift/lightspeed-operator/releases/latest/download/oc-ols_darwin_arm64.tar.gz | tar xz
sudo mv oc-ols /usr/local/bin/
```

---

## Not covered

- Homebrew tap or formula
- RPM/DEB packaging
- `oc` plugin discovery integration
- Semver release tagging
- Windows binaries

---

## Cross-references

- CLI architecture and command tree: **how/cli.md**
- Version variable location: `cli/version.go`
