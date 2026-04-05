# Contributing to Sortie

Thank you for your interest in contributing to Sortie. This guide covers
how to set up a development environment, run tests, and submit changes.

## Prerequisites

- Go 1.24+
- Node.js 22+
- Make
- (Optional) Docker, Kind, kubectl, Helm for Kubernetes testing

## Development Setup

```bash
git clone https://github.com/rjsadow/sortie.git
cd sortie
cp .env.example .env   # edit to set SORTIE_JWT_SECRET (min 32 chars)
make dev               # starts frontend (:5173) + backend (:8080)
```

Open `http://localhost:5173` to access the UI. The frontend proxies API
calls to the backend automatically.

## Project Structure

```text
main.go              Server entry point
internal/            Go backend packages
web/                 React frontend (Vite + TypeScript)
charts/sortie/       Helm chart
docs-site/           VitePress documentation
tests/integration/   API integration tests
web/e2e/             Playwright browser tests
```

See [docs-site/developer/architecture.md](docs-site/developer/architecture.md)
for a detailed architecture overview.

## Running Tests

```bash
make test               # Go unit tests
make test-integration   # API integration tests (mock runner, no K8s needed)
make test-all           # Unit + integration combined
make test-playwright    # Playwright browser E2E tests
make test-helm          # Helm chart unit tests
make test-e2e           # Full E2E against live Kind cluster
```

Run a single test:

```bash
go test -v -run TestSessionFromDB ./internal/sessions/
```

## Linting

```bash
make lint                           # Frontend ESLint
npx --prefix web tsc -b --noEmit    # TypeScript type checking
golangci-lint run ./...             # Go linting
```

All lint checks run in CI. Please fix any issues before submitting a PR.

## Submitting Changes

1. Fork the repo and create a branch from `main`.
2. Make your changes. Add or update tests as appropriate.
3. Ensure `make test-all` and `make lint` pass locally.
4. Commit with a clear message describing **what** and **why**.
5. Open a pull request against `main`.

### PR Guidelines

- Keep PRs focused on a single change. Separate refactors from features.
- Reference related issues (e.g., `Fixes #42`).
- Include a short description of the changes and any testing you did.
- New features should include tests. Bug fixes should include a regression test
  where practical.

### Commit Messages

Use clear, imperative-mood commit messages:

- `Add session timeout configuration`
- `Fix VNC reconnection on network change`
- `Update Helm chart resource defaults`

## Reporting Issues

- **Bugs:** Use the bug report issue template. Include steps to reproduce,
  expected vs. actual behavior, and environment details.
- **Features:** Use the feature request template. Describe the use case and
  any alternatives you considered.
- **Security:** See [SECURITY.md](SECURITY.md) for responsible disclosure.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you agree to uphold this code.

## License

By contributing to Sortie, you agree that your contributions will be
licensed under the [Apache License 2.0](LICENSE).
