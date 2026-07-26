# Codebase Guide

## Where to add a new check

1. Create `core/checks/<name>/check.go`.
2. Implement `check.Checker` from `core/check/types.go`.
3. Register the check in `core/checks/register.go`.
4. Add tests next to the new check.
5. Update `README.md`, `ARCHITECTURE.md`, and this guide if the feature changes the public surface.

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

- Current lightweight server and GUI: `cmd/server/main.go`.

Keep this file updated when adding new extension seams so issue descriptions can point to real code.
