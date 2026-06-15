# Contributing to warmbly-go

Thanks for taking the time to contribute! `warmbly-go` is the official Go SDK
for [Warmbly](https://warmbly.com), and we welcome bug reports, feature ideas,
docs improvements, and pull requests from everyone.

This document explains how to get set up, the standards we hold code to, and how
changes make their way into a release.

## Code of Conduct

This project and everyone participating in it is governed by our
[Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you are expected to
uphold it. Please report unacceptable behavior to the maintainers.

## Reporting bugs and requesting features

Found a bug or have an idea? Please open an issue using one of our templates:

- [Report a bug](https://github.com/warmbly/warmbly-go/issues/new?template=bug_report.yml)
- [Request a feature](https://github.com/warmbly/warmbly-go/issues/new?template=feature_request.yml)

Before filing, please search [existing issues](https://github.com/warmbly/warmbly-go/issues)
to avoid duplicates. For bugs, a minimal reproducible example plus your Go
version (`go version`) and SDK version goes a long way.

> Issues are for SDK bugs and feature requests. For account, billing, or
> platform questions, see [SUPPORT.md](./SUPPORT.md).

## Development setup

You'll need **Go 1.23 or newer**.

```bash
# Clone the repository
git clone https://github.com/warmbly/warmbly-go.git
cd warmbly-go

# Build everything
go build ./...

# Run the tests
go test ./...

# Lint and format
make lint
make fmt
```

`warmbly-go` has **zero external runtime dependencies** — the entire module is
built on the Go standard library, including the real-time WebSocket protocol
(implemented in `internal/wsconn`). Please keep it that way (see below).

## Coding standards

- **Formatting.** All code must be `gofmt`-clean, with imports organized by
  `goimports`. Run `make fmt` before committing.
- **Linting.** Code must pass `golangci-lint` with no new warnings. Run
  `make lint` locally; CI enforces it.
- **Documentation.** Every exported symbol (types, functions, methods,
  constants) needs a doc comment that starts with the symbol's name and reads
  as a complete sentence.
- **Tests.** Prefer table-driven tests. Exercise HTTP behavior with
  `net/http/httptest` rather than hitting the live API. New features and bug
  fixes should ship with tests.
- **Zero new third-party dependencies.** This is a stdlib-only project, and
  that's a core selling point. Adding any third-party dependency requires
  discussion first — please open an issue and get maintainer sign-off before
  introducing one. `go.mod` should stay free of `require` entries for external
  modules.

## Commit and pull request conventions

- **Use a conventional commit style.** Prefix your commit messages with a type:
  `feat:`, `fix:`, `docs:`, `chore:`, `test:`, or `refactor:`. For example:
  `feat(campaigns): add pause/resume helpers`.
- **Keep PRs small and focused.** One logical change per pull request is much
  easier to review and merge.
- **Ship tests and docs with code.** Behavior changes should update the
  relevant tests and documentation in the same PR.
- **Green checks required.** Every PR runs CI (build, vet, lint, tests). PRs
  need passing checks before they can be merged.

A typical workflow:

```bash
git checkout -b feat/my-change
# ...make your changes...
make fmt
make lint
make test
git commit -m "feat: describe your change"
git push origin feat/my-change
# open a PR against main
```

## Make targets

| Target       | What it does                                             |
| ------------ | -------------------------------------------------------- |
| `make test`  | Run the full test suite (`go test ./...`)                |
| `make lint`  | Run `golangci-lint` across the module                    |
| `make fmt`   | Format code with `gofmt`/`goimports`                     |
| `make vet`   | Run `go vet ./...`                                        |
| `make cover` | Run tests with coverage and produce a coverage report    |
| `make tidy`  | Run `go mod tidy` to keep `go.mod`/`go.sum` clean         |

## Releases

Releases are cut by maintainers. We follow semantic versioning: a maintainer
tags a release as `vX.Y.Z`, which publishes the new version to the Go module
proxy and makes it available via `go get`.

You don't need to bump versions in your PR — just describe your change clearly,
and the maintainers will handle tagging and release notes.

Thanks again for contributing!
