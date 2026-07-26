# Changelog

## Unreleased

### Changed
- Removed repository GitHub metadata files from `.github/`.
- Added contributor-oriented codebase and issue starting-point documentation.
- Removed stray macOS `.DS_Store` files from the repository.


All notable changes to this project will be documented in this file.

## v0.2.1 - 2026-07-25

### Added
- Fork/PR timeline chart (`images/forks-prs-timeline.svg`) and repo stats badges to README.

### Changed
- Centralized default checks registration into a single `core/checks.RegisterDefaults()` function (`core/checks/register.go`). CLI and server now import one function instead of 4 individual check constructors.
- Removed duplicate registration boilerplate from `cmd/cli/commands/diagnose.go`, `cmd/cli/commands/diagnose_test.go`, and `cmd/server/main.go`.

## v0.2.0 - 2025-07-25

### Added
- **SOCKS4/SOCKS5 proxy adapters** (`core/adapters/socks.go`):
  - `SOCKS4Adapter`: full SOCKS4/4a protocol implementation (custom dialer, CONNECT handshake, domain name support via SOCKS4a `0.0.0.1` extension).
  - `SOCKS5Adapter`: full SOCKS5 protocol implementation using `golang.org/x/net/proxy` with manual fallback. Supports IPv4, IPv6, domain targets, and username/password authentication (RFC 1929).
  - Both adapters implement all `NetworkAdapter` interface methods: HTTP requests, redirect following, DNS resolution, port testing, TLS certificate/cipher suite/version detection, and public IP detection.
- **3 new diagnostic checks** (registered in both CLI and server):
  - `dns_resolve` — resolves hostname to IP addresses through the current connection.
  - `tls_certificate` — validates TLS certificate (issuer, expiry, cipher suite, TLS version).
  - `port_connectivity` — tests TCP connectivity to ports 80, 443, 8080, 8443.
- `golang.org/x/net` dependency added for SOCKS5 proxy support.
- `core/utils/proxyconfig.go`: `ParseProxyConfig` now supports bare `host:port` format (without scheme) when an explicit proxy type is selected. Also handles bare `host` with default port fallback.
- Web GUI at `http://localhost:8080/`: single-page form (URL, proxy, proxy type) that runs a real diagnosis and renders results in the browser.
- `POST /api/diagnose`: runs a diagnosis through the real core engine (`core/engine.DiagnosisOrchestrator`), same code path as `cli diagnose`.
- `GET /api/checks`: lists all registered checks as JSON (web equivalent of `cli list-checks`).
- `core/utils.ParseProxyConfig`: proxy URL parsing extracted into a shared helper used by both the CLI and the server.
- JSON tags on `engine.DiagnosisReport` / `engine.RequestMetadata` (snake_case, consistent with `check.CheckResult`).

### Changed
- `cmd/server`: rewritten from a standalone placeholder into a thin HTTP layer over the core engine. The old `GET /api/check/public-ip` endpoint was removed in favor of `POST /api/diagnose`.
- GUI proxy input: improved with hint text, examples, and "auto (from scheme)" default in dropdown.
- All 4 checks registered in `cmd/cli/commands/diagnose.go`, `cmd/cli/commands/list.go`, and `cmd/server/main.go`.

### Fixed
- `cli diagnose --proxy`: the flag value was parsed and then discarded. Now the proxy URL is properly parsed (scheme, host, port, credentials) and `--proxy-type` is respected.
- Proxy input: `host:port` format now works when an explicit proxy type is selected (previously failed with URL parse error).

## v0.1.0 - 2025-07-20

- Initial project snapshot: CLI and core components.
