# Codebase Guide

## Where to add a new built-in check

1. Create `core/checks/<name>/check.go`.
2. Implement `check.Checker` from `core/check/types.go`.
3. Register the check in `core/checks/register.go`.
4. Add tests next to the new check.
5. Update `README.md`, `ARCHITECTURE.md`, and this guide if the feature changes the public surface.

## Where to add a new plugin

1. Create `core/plugins/<name>/plugin.go`.
2. Implement `plugin.CheckPlugin` (registers a check for `diagnose`) or a plain `plugin.Plugin` (standalone server). See `core/plugins/mcp/plugin.go` and `core/plugins/routetrace/plugin.go` for examples.
3. Register the plugin in `core/plugins/registry.go` (the `Available()` function).
4. Add tests next to the plugin.
5. Add loading logic in `cmd/cli/commands/plugins.go` if the plugin needs special configuration (optional).
6. Update `README.md` and `ARCHITECTURE.md` with the new plugin.

### Plugin loading modes

- **Check plugins** (`route_trace`): loaded with `diagnose --plugins <id>`. The check is registered and executed during diagnosis.
- **Standalone plugins** (`mcp_server`): loaded with `proxyctl --plugins <id>` (no subcommand). The plugin runs as a long-lived process until SIGINT.

## Where to add adapter behavior

- Shared adapter helpers: `core/adapters/common.go`.
- Direct networking: `core/adapters/direct.go`.
- HTTP/HTTPS proxy behavior: `core/adapters/http_proxy.go`.
- SOCKS behavior: `core/adapters/socks.go`.
- Adapter selection: `core/adapters/factory.go`.

## Where to add CLI behavior

- Command wiring and flags: `cmd/cli/commands/diagnose.go`.
- Command tests: `cmd/cli/commands/diagnose_test.go`.
- CLI entry point: `cmd/cli/main.go`.

## Where to add GUI/API behavior

- Current lightweight server and GUI: `cmd/server/main.go` (does NOT load plugins — only built-in checks).

Keep this file updated when adding new extension seams so issue descriptions can point to real code.
