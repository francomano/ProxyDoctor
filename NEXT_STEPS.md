# Next Steps

Follow-up work intentionally outside the current delivery scope.

## High priority

- Add integration tests with local HTTP, HTTPS, SOCKS4 and SOCKS5 proxy fixtures.
- Add timeout and cancellation tests across all built-in checks.
- Add CI jobs that run `gofmt`, `go test ./...` and `go vet ./...` on every pull request.
- Add end-to-end tests for the HTTP server API.

## Medium priority

- Add golden-file tests for text, JSON, Markdown and HTML output.
- Add structured logging around adapter creation and check execution.
- Add configuration examples for authenticated proxies.
- Improve documentation for common corporate proxy troubleshooting scenarios.

## Optional future checks

These require explicit design of data sources, privacy trade-offs and test fixtures before implementation:

- DNS leak detection.
- WebRTC leak detection.
- Geolocation comparison.
- IP reputation enrichment.

## Documentation rules

- `ARCHITECTURE.md` describes only implemented behavior.
- Planned or speculative work belongs in this file.
