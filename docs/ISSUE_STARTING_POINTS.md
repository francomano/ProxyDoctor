# Issue Starting Points

This document maps the current open roadmap issues to concrete entry points in the codebase. It is meant to help a contributor start from existing code instead of guessing where a feature belongs.

## Core extension pattern

Most diagnosis features should start from these files:

- `core/check/types.go` — `Checker`, `CheckResult`, `ExecutionContext`, `NetworkAdapter` contracts.
- `core/checks/register.go` — built-in check registration.
- `core/plugin/plugin.go` — plugin interfaces (`CheckPlugin`, `ExportPlugin`, `MiddlewarePlugin`).
- `core/plugins/registry.go` — plugin catalog and loading.
- `core/engine/orchestrator.go` — direct/proxy execution and comparison flow.
- `cmd/cli/commands/diagnose.go` — CLI flags, filtering and text/JSON/Markdown output.
- `cmd/server/main.go` — web GUI and JSON API.
- `ARCHITECTURE.md` — implemented architecture overview.

## Open issues

| Issue | Topic | Starting point |
| --- | --- | --- |
| #5 | DNS leak detection | Start from `core/checks/dns_resolve/check.go`, `core/adapters/*`, and `core/engine/orchestrator.go` comparison logic. New check should live under `core/checks/dns_leak/`. |
| #6 | WebRTC leak detection | Start from the check contract in `core/check/types.go` and the plugin extension points in `core/plugin/plugin.go`. Browser/STUN-specific logic should be isolated under `core/checks/webrtc_leak/`. |
| #7 | Geolocation check | Start from `core/checks/public_ip/check.go` and `core/checks/route_trace/check.go`, which already perform public IP detection and country enrichment. New check should live under `core/checks/geolocation/`. |
| #8 | IP reputation | Start from `core/checks/public_ip/check.go` for dependency on public IP and API-style response handling. New code should live under `core/checks/ip_reputation/`. |
| #13 | Error messages and logging | Start from `cmd/cli/commands/diagnose.go` for verbosity flags/output, and from `core/checks/*` for contextual errors. Shared logger belongs in `core/utils/`. |
| #21 | Installation methods | Start from `go.mod`, `run.sh`, `setup.sh`, `README.md`, and the CLI entry point `cmd/cli/main.go`. Release automation can be added later outside `.github` if the repo intentionally keeps GitHub metadata out. |
| #22 | Shell completion | Start from Cobra setup in `cmd/cli/main.go` and command definitions under `cmd/cli/commands/`. |
| #25 | MCP server plugin | ✅ Implemented in `core/plugins/mcp/plugin.go`. Exposes `diagnose`, `list_checks`, and `compare` as MCP tools on an HTTP server (default :9090). Protocol-specific server code lives under `core/plugins/mcp/`. |
| #26 | Prometheus exporter plugin | Start from `core/plugin/plugin.go`, especially `ExportPlugin` and `MiddlewarePlugin`. Metrics collection should hook after diagnosis reports are produced. |
| #27 | Watch mode plugin | Start from `core/plugin/plugin.go` and `core/engine/orchestrator.go`. Scheduling/storage should live outside the core check packages. |
| #30 | Slack/Discord bot plugin | Start from `core/plugin/plugin.go` (`MiddlewarePlugin`) and `cmd/server/main.go` request/response models. |
| #34 | VPN proxy/profile support | Start from `core/check/types.go`, `core/utils/proxyconfig.go`, `core/adapters/factory.go`, and `core/checks/route_trace/check.go`. |
| #35 | Local proxy integration-test fixtures | Start from `core/adapters/*` and add fixture-based tests under `core/adapters/` or `core/checks/`. |
| #36 | Authenticated proxy support | Start from `core/utils/proxyconfig.go`, `core/adapters/http_proxy.go`, and `core/adapters/socks.go`. Add redaction helpers in `core/utils/` if needed. |
| #38 | Multi-proxy route optimizer | Start from `core/plugins/route_trace/` and `core/engine/orchestrator.go`. Scoring logic should be a `CheckPlugin` under `core/plugins/route_optimizer/`. |
| #41 | Windows portable executable | Cross-compile with `GOOS=windows`. Build script or GoReleaser. |
| #42 | Add --no-color flag | Start from `cmd/cli/commands/diagnose.go` — add flag, modify `formatText()` to skip emoji when set. |
| #43 | Per-check execution time in text output | Start from `cmd/cli/commands/diagnose.go` `formatText()` — `CheckResult.ExecutionTime` is already populated, just render it. |

## Rule of thumb

A new diagnostic capability should normally be implemented as a `core/checks/<feature>/` package (for built-in checks) or a `core/plugins/<feature>/` package (for plugin checks), registered appropriately, exposed through the existing CLI/server flow, and covered by package-level tests.
