# ProxyDoctor Architecture

![ProxyDoctor architecture](images/architecture.png)

ProxyDoctor is a Go CLI and lightweight HTTP UI for diagnosing how a target URL behaves through direct, HTTP, HTTPS, SOCKS4, and SOCKS5 connections.

This document describes the implemented architecture only. Ideas and follow-up work are tracked in `NEXT_STEPS.md`.

## Package layout

```text
cmd/
  cli/                 Cobra-based command line entry point
  server/              Web UI and JSON API over the same diagnosis engine
core/
  adapters/            Direct, HTTP(S), SOCKS4 and SOCKS5 network adapters
  check/               Shared check, result, proxy, HTTP and TLS types
  checks/              Built-in diagnostic checks
  engine/              Registry, dependency ordering and orchestration
  plugin/              In-process extension interfaces and lifecycle manager
  utils/               Proxy configuration parsing helpers
```

## Runtime flow

1. The CLI or server receives a target URL plus optional proxy settings.
2. `core/utils.ParseProxyConfig` normalizes proxy input into `check.ProxyConfig`.
3. `core/checks.RegisterDefaults` registers the built-in checks in `core/engine.CheckRegistry`.
4. `core/engine.DiagnosisOrchestrator` creates direct/proxy adapters, resolves dependencies and executes the selected checks.
5. Results are returned as `check.CheckResult` values and rendered by the CLI or HTTP server.

## Built-in checks

- `public_ip`: detects and validates the public IP returned by external IP services.
- `dns_resolve`: resolves the target hostname through the selected connection path.
- `tls_certificate`: checks certificate validity, issuer, SANs, public key metadata, TLS version and cipher suite for HTTPS targets.
- `port_connectivity`: tests common TCP ports on the target host.
- `route_trace`: runs a best-effort traceroute and annotates public hops with country information.
- `ipv6_leak`: checks whether the system and target support IPv6, compares the system's direct public IPv6 address against the proxy's IPv6 forwarding capability, and reports whether IPv6 traffic can bypass the configured proxy/tunnel.

## Network adapters

Every adapter implements `check.NetworkAdapter`:

- `DirectAdapter`: native networking without a proxy.
- `HTTPProxyAdapter`: HTTP proxy support, including CONNECT for tunneled checks.
- `HTTPSProxyAdapter`: HTTPS proxy support, including CONNECT for tunneled checks.
- `SOCKS4Adapter`: SOCKS4/SOCKS4a-style proxy connections.
- `SOCKS5Adapter`: SOCKS5 proxy connections via `golang.org/x/net/proxy`.

## Result model

Checks return structured results with:

- status and severity;
- confidence;
- human-readable explanation;
- evidence map;
- probable causes and suggested actions when useful;
- execution time and timestamp.

## Design notes

- Adapter methods return errors instead of hiding failed network operations.
- Checks classify outcomes as `passed`, `failed`, `skipped` or `error`.
- Shared context allows checks to reuse collected evidence such as DNS results or public IP.
- The same engine is used by CLI and server entry points.

## Contributor entry points

- New checks start from `core/check/types.go`, `core/checks/register.go`, and the existing packages under `core/checks/`.
- Adapter work starts from `core/adapters/`.
- CLI work starts from `cmd/cli/commands/diagnose.go`.
- GUI/API work starts from `cmd/server/main.go`.
- Roadmap issues are mapped to concrete files in `docs/ISSUE_STARTING_POINTS.md`.

