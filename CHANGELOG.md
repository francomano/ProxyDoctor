# Changelog

## v0.3.0 - 2026-07-29

### Added
- **MCP server plugin** (`core/plugins/mcp/plugin.go`): exposes `diagnose`, `list_checks`, and `compare` as MCP (Model Context Protocol) tools via HTTP JSON-RPC on `:9090`. Registered in `core/plugins/registry.go`. Closes #25.
- **SSE transport** (`core/plugins/mcp/plugin.go`): MCP server now supports Server-Sent Events (GET /mcp) for streaming transport in addition to direct JSON-RPC POST.
- **MCP initialize handshake**: MCP server implements the `initialize` method per the MCP spec (protocol version `2024-11-05`), enabling compatibility with MCP clients like OpenCode.
- **OpenCode integration** (`opencode.jsonc`): configuration file for OpenCode AI assistant, registering ProxyDoctor as a remote MCP server at `http://localhost:9090/mcp` with tools `diagnose`, `compare`, and `list_checks`.
- `ipv6_leak` check (`core/checks/ipv6_leak/check.go`): detects whether the system and target support IPv6, discovers the system's real public IPv6 address by probing directly (bypassing any configured proxy/tunnel), tests whether the configured proxy forwards IPv6 destinations, and reports whether IPv6 traffic would bypass the proxy/tunnel and leak the system's real address. Registered in `core/checks/register.go`.
- Plugin system with `--plugins` CLI flag (`cmd/cli/commands/diagnose.go`). Plugins are loaded at runtime via `core/plugins.Load()`. The server auto-loads all available plugins.
- `route_trace` plugin (`core/plugins/routetrace/plugin.go`): wraps the route trace check as a `CheckPlugin`. Loaded via `--plugins route_trace` or `--plugins all`.

### Changed
- `route_trace` moved from built-in checks to a plugin. It is no longer registered by `core/checks.RegisterDefaults()`. Use `--plugins route_trace` to enable it.
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
