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
| #2 | HTML export formatter | Start in `cmd/cli/commands/diagnose.go`, especially the existing JSON/Markdown/text rendering helpers. Add tests in `cmd/cli/commands/diagnose_test.go`. |
| #5 | DNS leak detection | Start from `core/checks/dns_resolve/check.go`, `core/adapters/*`, and `core/engine/orchestrator.go` comparison logic. New check should live under `core/checks/dns_leak/`. |
| #6 | WebRTC leak detection | Start from the check contract in `core/check/types.go` and the plugin extension points in `core/plugin/plugin.go`. Browser/STUN-specific logic should be isolated under `core/checks/webrtc_leak/`. |
| #7 | Geolocation check | Start from `core/checks/public_ip/check.go` and `core/checks/route_trace/check.go`, which already perform public IP detection and country enrichment. New check should live under `core/checks/geolocation/`. |
| #8 | IP reputation | Start from `core/checks/public_ip/check.go` for dependency on public IP and API-style response handling. New code should live under `core/checks/ip_reputation/`. |
| #10 | IPv6 leak detection | Start from `core/checks/public_ip/check.go`, `core/checks/dns_resolve/check.go`, and adapter DNS/HTTP methods. New check should live under `core/checks/ipv6_leak/`. |
| #13 | Error messages and logging | Start from `cmd/cli/commands/diagnose.go` for verbosity flags/output, and from `core/checks/*` for contextual errors. Shared logger belongs in `core/utils/`. |
| #21 | Installation methods | Start from `go.mod`, `run.sh`, `setup.sh`, `README.md`, and the CLI entry point `cmd/cli/main.go`. Release automation can be added later outside `.github` if the repo intentionally keeps GitHub metadata out. |
| #22 | Shell completion | Start from Cobra setup in `cmd/cli/main.go` and command definitions under `cmd/cli/commands/`. |
| #25 | MCP server plugin | Start from `core/plugin/plugin.go` and `cmd/server/main.go`. Keep protocol-specific server code isolated under a plugin/example package. |
| #26 | Prometheus exporter plugin | Start from `core/plugin/plugin.go`, especially `ExportPlugin` and `MiddlewarePlugin`. Metrics collection should hook after diagnosis reports are produced. |
| #27 | Watch mode plugin | Start from `core/plugin/plugin.go` and `core/engine/orchestrator.go`. Scheduling/storage should live outside the core check packages. |
| #29 | Interactive HTML report plugin | Start from `core/plugin/plugin.go` (`ExportPlugin`) and the existing GUI rendering in `cmd/server/main.go`. |
| #30 | Slack/Discord bot plugin | Start from `core/plugin/plugin.go` (`MiddlewarePlugin`) and `cmd/server/main.go` request/response models. |
| #34 | VPN proxy/profile support | Start from `core/check/types.go`, `core/utils/proxyconfig.go`, `core/adapters/factory.go`, and `core/checks/route_trace/check.go`. |
| #35 | Local proxy integration-test fixtures | Start from `core/adapters/*` and add fixture-based tests under `core/adapters/` or `core/checks/`. |
| #36 | Authenticated proxy support | Start from `core/utils/proxyconfig.go`, `core/adapters/http_proxy.go`, and `core/adapters/socks.go`. Add redaction helpers in `core/utils/` if needed. |
| #37 | Stable plugin SDK/examples | Start from `core/plugin/plugin.go` and `core/plugin/plugin_test.go`. Examples should live under `examples/plugins/`. |
| #38 | Multi-proxy route optimizer | Start from `core/checks/route_trace/check.go`, `core/engine/orchestrator.go`, and `cmd/cli/commands/diagnose.go`. Scoring logic should be isolated from check execution. |

## Rule of thumb

A new diagnostic capability should normally be implemented as a `core/checks/<feature>/` package (for built-in checks) or a `core/plugins/<feature>/` package (for plugin checks), registered appropriately, exposed through the existing CLI/server flow, and covered by package-level tests.
