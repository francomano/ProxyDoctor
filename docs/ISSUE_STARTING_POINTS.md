# Issue Starting Points

This document maps the current open roadmap issues to concrete entry points in the codebase. It is meant to help a contributor start from existing code instead of guessing where a feature belongs.

## Core extension pattern

Most diagnosis features should start from these files:

- `core/check/types.go` — `Checker`, `CheckResult`, `ExecutionContext`, `NetworkAdapter` contracts.
- `core/checks/register.go` — built-in check registration.
- `core/plugin/plugin.go` — plugin interfaces (`CheckPlugin`, `ExportPlugin`, `MiddlewarePlugin`).
- `core/plugins/registry.go` — plugin catalog and `Available()` map.
- `core/engine/orchestrator.go` — direct/proxy execution and comparison flow.
- `cmd/cli/commands/diagnose.go` — CLI flags, filtering and text/JSON/Markdown output.
- `cmd/cli/commands/plugins.go` — `--plugins` flag, shared `loadPlugins()`, and standalone `runPlugins()`.
- `cmd/server/main.go` — web GUI and JSON API (diagnose + local forward proxy).
- `cmd/server/localproxy.go` — web-GUI lifecycle of the `local_proxy` plugin.
- `core/plugins/localproxy/plugin.go` — standalone `local_proxy` plugin (forward proxy over `internal/testproxy`).
- `core/adapters/dial.go` — `NewProxyDialContext` / `ForwardTransport` dial helpers used by the local proxy.
- `ARCHITECTURE.md` — implemented architecture overview.

Plugins have two modes:
- **Check plugins** (`route_trace`): register a `Check` via `RegisterChecks()`. Must be loaded with `diagnose --plugins <id>`.
- **Standalone plugins** (`mcp_server`, `local_proxy`): start a long-running process in `Init()`. Loaded with `proxyctl --plugins <id>` (blocks until SIGINT). The `local_proxy` plugin also backs the web GUI control in `cmd/server/`.

## Open issues

| Issue | Topic | Starting point |
| --- | --- | --- |
| #5 | DNS leak detection | Start from `core/checks/dns_resolve/check.go`, `core/adapters/*`, and `core/engine/orchestrator.go` comparison logic. New check should live under `core/checks/dns_leak/`. |
| #6 | WebRTC leak detection | Start from the check contract in `core/check/types.go` and the plugin extension points in `core/plugin/plugin.go`. Browser/STUN-specific logic should be isolated under `core/checks/webrtc_leak/`. |
| #7 | Geolocation check | Start from `core/checks/public_ip/check.go` and `core/checks/route_trace/check.go`, which already perform public IP detection and country enrichment. New check should live under `core/checks/geolocation/`. |
| #8 | IP reputation | Start from `core/checks/public_ip/check.go` for dependency on public IP and API-style response handling. New code should live under `core/checks/ip_reputation/`. |
| #13 | Error messages and logging | Start from `cmd/cli/commands/diagnose.go` for verbosity flags/output, and from `core/checks/*` for contextual errors. Shared logger belongs in `core/utils/`. |
| #22 | Shell completion | Start from Cobra setup in `cmd/cli/main.go` and command definitions under `cmd/cli/commands/`. |
| #26 | Prometheus exporter plugin | Implement as a standalone plugin (`--plugins prometheus`). Start from `core/plugin/plugin.go`. Serve `/metrics` on a configurable port via HTTP in `Init()`. |
| #27 | Watch mode plugin | Implement as a standalone plugin (`--plugins watch`). Start from `core/plugin/plugin.go` and `core/engine/orchestrator.go`. Scheduling/storage should live outside the core check packages. |
| #30 | Slack/Discord bot plugin | Implement as a standalone plugin (`--plugins slack` or `--plugins discord`). Start from `core/plugin/plugin.go`. Use `MiddlewarePlugin` to hook into diagnosis lifecycle if paired with `diagnose`. |
| #34 | VPN proxy/profile support | Start from `core/check/types.go`, `core/utils/proxyconfig.go`, `core/adapters/factory.go`, and `core/checks/route_trace/check.go`. |
| #36 | Authenticated proxy support | Start from `core/utils/proxyconfig.go`, `core/adapters/http_proxy.go`, and `core/adapters/socks.go`. Add redaction helpers in `core/utils/` if needed. |
| #38 | Multi-proxy route optimizer | Start from `core/plugins/route_trace/` and `core/engine/orchestrator.go`. Scoring logic should be a `CheckPlugin` under `core/plugins/route_optimizer/`. |
| #41 | Windows portable executable | Cross-compile with `GOOS=windows`. Build script or GoReleaser (`.goreleaser.yaml` already produces Windows archives). |
| #42 | Add --no-color flag | Start from `cmd/cli/commands/diagnose.go` — add flag, modify `formatText()` to skip emoji when set. |
| #43 | Per-check execution time in text output | Start from `cmd/cli/commands/diagnose.go` `formatText()` — `CheckResult.ExecutionTime` is already populated, just render it. |
| #45 | PAC file for the tested proxy | Start from `core/plugins/localproxy/plugin.go` (add a `/proxy.pac` route to the HTTP handler), `cmd/server/localproxy.go` (surface the PAC URL in the GUI), and reuse `internal/testproxy` fixtures for tests. |

## Recently closed

| Issue | Topic | Where it landed |
| --- | --- | --- |
| #21 | Installation methods | `go.mod` (replace directive removed), `.goreleaser.yaml`, `.github/workflows/release.yml`, README installation section. `go install`, Homebrew cask, and release binaries. |
| #25 | MCP server plugin | `core/plugins/mcp/plugin.go`, registered in `core/plugins/registry.go`, `opencode.jsonc`. |
| #35 | Local proxy integration-test fixtures | `internal/testproxy/fixtures.go`, `core/adapters/adapters_integration_test.go`, `core/plugins/localproxy/plugin_test.go`. |
| #44 | Local forward proxy plugin | `core/plugins/localproxy/plugin.go`, `core/adapters/dial.go`, GUI in `cmd/server/`. Superseded by the PAC enhancement #45. |

## Rule of thumb

A new diagnostic capability should normally be implemented as a `core/checks/<feature>/` package (for built-in checks) or a `core/plugins/<feature>/` package (for plugin checks), registered appropriately, exposed through the existing CLI/server flow, and covered by package-level tests.
