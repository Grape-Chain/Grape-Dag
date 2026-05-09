# Contributing to Luna

Thanks for your interest in improving Luna. This document covers the workflow,
coding standards, and review expectations for contributions.

## Code of Conduct

This project adopts the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you agree to uphold its terms.

## Reporting Bugs and Requesting Features

- **Security vulnerabilities:** see [SECURITY.md](SECURITY.md). Do not open a
  public issue.
- **Bugs:** open an issue with a minimal reproduction, the version/commit you
  observed, and the actual vs. expected behaviour.
- **Features and design proposals:** open an issue first to discuss the design
  before sending a pull request. For non-trivial work this saves everyone time.

## Development Setup

Requirements:

- Go 1.24 or newer
- Docker (for the integration tests and the `make docker` target)
- A POSIX shell (the example scripts under `examples/` assume `bash`)

Build and test:

```sh
make build      # produces binaries in ./bin/
make test       # runs unit tests with the race detector
make lint       # go vet + gofmt check
```

## Pull Request Workflow

1. Fork the repository and create a topic branch from `main`.
2. Keep changes focused. One logical change per PR.
3. Add or update tests that cover the change.
4. Run `make lint test` locally before pushing.
5. Open a PR with a clear title and a description that explains the *why*, not
   only the *what*. Reference any related issue.
6. Mark the PR as a draft if it is not ready for review yet.

A maintainer will review and may request changes. Squash-merge is the default;
keep the final commit message tidy.

## Sign Your Work — Developer Certificate of Origin

We use the [Developer Certificate of Origin](https://developercertificate.org/)
(DCO). Every commit must be signed off:

```sh
git commit -s -m "your message"
```

This appends a `Signed-off-by` trailer to the commit. CI rejects unsigned
commits.

## Coding Standards

- Run `gofmt` (the `make fmt` target enforces this).
- Use `go vet` clean output as a baseline.
- Keep functions small. Exported names need doc comments.
- Avoid adding dependencies casually. Prefer the standard library.
- New configuration knobs go in the YAML config files under `config/`, not as
  hard-coded constants.

## Tests

- Unit tests live next to the code they cover (`foo.go` → `foo_test.go`).
- Integration tests that need Docker live under `tests/integration/`.
- Tests must pass under `go test -race`.

## Commit Messages

- Imperative mood ("add tip-selection cache", not "added" or "adds").
- First line under 72 characters.
- Body explains the motivation. Reference issues with `#123`.

## License

By contributing you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
