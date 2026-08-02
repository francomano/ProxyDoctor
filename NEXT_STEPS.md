# Next Steps

Follow-up work intentionally outside the current delivery scope.

## High priority

- Add CI jobs that run `gofmt`, `go test ./...` and `go vet ./...` on every pull request.
- Add timeout and cancellation tests across all built-in checks.
- Add end-to-end tests for the HTTP server API (`/api/diagnose`, `/api/local-proxy/*`).
- Add PAC file support (#45): serve `http://127.0.0.1:8081/proxy.pac` from the local proxy so users can configure their whole system in one step.
- Add DNS leak detection (#5).

## Medium priority

- Add golden-file tests for text, JSON, Markdown and HTML output.
- Add structured logging around adapter creation and check execution.
- Add configuration examples for authenticated proxies.
- Improve documentation for common corporate proxy troubleshooting scenarios.

## Optional future checks

These require explicit design of data sources, privacy trade-offs and test fixtures before implementation:

- WebRTC leak detection (#6).
- Geolocation comparison (#7).
- IP reputation enrichment (#8).

## Recently completed

- **Hermetic proxy integration tests** (`internal/testproxy` + `core/adapters/adapters_integration_test.go` + `core/plugins/localproxy/plugin_test.go`): every adapter (direct, HTTP, HTTPS, SOCKS4, SOCKS5, auth, TLS-through-proxy) now runs against local fixtures, offline. Closes #35.
- **Local forward proxy plugin** (`core/plugins/localproxy/`): expose the tested proxy on `127.0.0.1:8081` from the CLI (`--plugins local_proxy`) or the web GUI (start/stop + copy-ready commands).
- **Installation methods**: `go install`, Homebrew cask, and cross-compiled release binaries via GoReleaser. Closes #21.
- **WebRTC / DNS leak**: moved to the open-issues roadmap above.

## Documentation rules

- `ARCHITECTURE.md` describes only implemented behavior.
- Planned or speculative work belongs in this file.

## Issue hygiene

Every open issue should point to at least one concrete starting point in the codebase. Keep `docs/ISSUE_STARTING_POINTS.md` updated whenever roadmap issues are added, closed, or substantially rewritten.

