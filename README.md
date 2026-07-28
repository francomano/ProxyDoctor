# ProxyDoctor — Proxy Diagnostics

**A minimal, testable Go tool for running network diagnostics through proxies**

[![CI](https://github.com/francomano/ProxyDoctor/actions/workflows/test.yml/badge.svg)](https://github.com/francomano/ProxyDoctor/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/francomano/ProxyDoctor)](https://goreportcard.com/report/github.com/francomano/ProxyDoctor)
[![Go Reference](https://pkg.go.dev/badge/github.com/francomano/ProxyDoctor.svg)](https://pkg.go.dev/github.com/francomano/ProxyDoctor)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
[![Contributions Welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg?logo=go)](https://go.dev/)

<p align="center">
  <img src="images/proxydoctor-logo.png" alt="ProxyDoctor Logo" width="150">
</p>

<p align="center">
  <img src="images/proxydoctor-demo.gif" alt="ProxyDoctor CLI Demo" width="750">
</p>

> **Built with AI assistance** — This project was developed with contributions from:
> - **GitHub Copilot** and **OpenCode** (code development and implementation)
> - **GPT 5.5** and **Gemini Pro** (architecture review, especially pull request code review)

## Why ProxyDoctor?

**The Problem:**
You're behind a corporate proxy or using a VPN, and things break mysteriously:
- Sites don't load (proxy misconfiguration?)
- Your IP leaks even through a "private" proxy
- You can't tell if the problem is your proxy, DNS, or the site itself

**The Solution:**
ProxyDoctor is a **lightweight diagnostic tool** that:
- Runs network checks through any proxy (HTTP, HTTPS, SOCKS4, SOCKS5)
- Compares results between direct connection and proxied connection
- Identifies which specific layer is failing (DNS? TLS? IP leak?)
- Gives you actionable insights in seconds

**Perfect for:**
- Developers debugging proxy issues
- DevOps engineers troubleshooting VPN connectivity
- Security teams validating proxy implementations
- Anyone tired of guessing what's broken

## What It Does

ProxyDoctor is a CLI-first tool to:
- Run network checks (DNS resolution, IP detection, TLS certificate validation, port connectivity)
- Compare results between direct connections and proxy-routed connections
- Route tracing with country flags in the GUI and country names in CLI output
- Identify connectivity issues and proxy misconfigurations

## Project Status

> This version was reviewed and bug-fixed with OpenCode using GPT 5.5 before delivery.


**v0.3.0 (Alpha)**
- ✅ Core engine with check registry and dependency DAG
- ✅ CLI with diagnose and list-checks commands
- ✅ HTTP/HTTPS proxy support in `diagnose --proxy`
- ✅ SOCKS4/SOCKS5 proxy support (full protocol implementation, SOCKS4a domain support, SOCKS5 auth per RFC 1929)
- ✅ HTTP server wired to the core engine, with a web GUI at `/` and `/api/checks`, `/api/diagnose` JSON endpoints
- ✅ Web GUI — single-page form at `http://localhost:8080/`, runs a real diagnosis via `POST /api/diagnose` and renders results
- ✅ 6 built-in checks: public_ip, dns_resolve, tls_certificate, port_connectivity, route_trace, ipv6_leak
- ✅ `--export json` and `--export markdown` (working)
- ✅ Unit tests (6 passing tests, engine coverage)
- ✅ Plugin system (CheckPlugin, ExportPlugin, MiddlewarePlugin interfaces)
- 🧭 Focused backlog for optional checks such as DNS leak, WebRTC leak, geolocation and IP reputation
- ✅ MCP server plugin (Model Context Protocol, exposes diagnose/compare tools on port :9090)

## Requirements

- **Go** >= 1.25
- **Git**

## Quick Start

### Setup (first time only)

```bash
git clone https://github.com/francomano/proxydoctor
cd ProxyDoctor
./setup.sh
```

This script will:
- Verify Go installation
- Download and verify dependencies
- Run all tests (6 passing tests)
- Build CLI and server binaries

### Try the Web GUI

```bash
# Start the server
./run.sh server

# Open in browser
open http://localhost:8080
```

Fill in the URL (and optionally a proxy + proxy type), hit "Run diagnosis" — it runs the same `core/engine.DiagnosisOrchestrator` the CLI uses and renders the results as cards.

<p align="center">
  <img src="images/gui.png" alt="ProxyDoctor Web GUI" width="750">
</p>

Two JSON endpoints back the GUI, and can be called directly:

```bash
# List available checks
curl http://localhost:8080/api/checks

# Run a diagnosis
curl -X POST http://localhost:8080/api/diagnose \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","proxy":"socks5://77.245.76.107:1080","proxy_type":"socks5"}'
```

### Use the CLI

```bash
# Get help
./run.sh cli --help

# List available checks
./run.sh cli list-checks

# Run diagnostics (direct connection)
./run.sh cli diagnose --url https://example.com

# Run diagnostics with a custom timeout
./run.sh cli diagnose --url https://example.com --timeout 10s

# Run only selected checks
./run.sh cli diagnose --url https://example.com --checks public_ip,dns_resolve

# Run diagnostics through an HTTP proxy
./run.sh cli diagnose --url https://example.com --proxy http://127.0.0.1:3128 --proxy-type http

# Run diagnostics through a SOCKS5 proxy (with scheme)
./run.sh cli diagnose --url https://example.com --proxy socks5://127.0.0.1:1080 --proxy-type socks5

# Run diagnostics through a SOCKS5 proxy (bare host:port + type)
./run.sh cli diagnose --url https://example.com --proxy 127.0.0.1:1080 --proxy-type socks5

# Compare direct and proxied diagnosis results
./run.sh cli diagnose --url https://example.com --proxy socks5://127.0.0.1:1080 --compare

# Export results as JSON
./run.sh cli diagnose --url https://example.com --export json --output report.json
```

### Run the Server

```bash
# Start HTTP server on :8080
./run.sh server
```

### Run Tests

```bash
# Run all tests
./run.sh test

# Or directly
go test -v ./...
```

## Plugin System

ProxyDoctor has a plugin system for extending functionality. Plugins can add new checks or long-running services.

Available plugins are loaded via the `--plugins` flag on `./run.sh cli`.

### Available plugins

| Plugin | ID | Type | Description |
|---|---|---|---|
| Route Trace | `route_trace` | check | Traces network hops and annotates public hops with country information |
| MCP Server | `mcp_server` | standalone | Exposes diagnose/compare/list_checks tools via the Model Context Protocol on `:9090` |

### Using plugins

**route_trace** — registra un nuovo check utilizzabile con `diagnose`:

```bash
./run.sh cli diagnose --url https://example.com --plugins route_trace
```

**mcp_server** — avvia un server JSON-RPC 2.0 standalone:

```bash
./run.sh cli --plugins mcp_server
```

Una volta avviato, invia richieste a `POST http://localhost:9090/mcp`:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diagnose","arguments":{"url":"https://example.com"}}}
```

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"compare","arguments":{"url":"https://example.com","proxy":"socks5://77.245.76.107:1080"}}}
```

Carica più plugin insieme:

```bash
./run.sh cli --plugins route_trace,mcp_server
```

### Creating a plugin

```go
import (
    "github.com/francomano/proxydoctor/core/engine"
    "github.com/francomano/proxydoctor/core/plugin"
)

type MyPlugin struct{}

func (p *MyPlugin) ID() string          { return "my-plugin" }
func (p *MyPlugin) Name() string        { return "My Plugin" }
func (p *MyPlugin) Version() string     { return "0.1.0" }
func (p *MyPlugin) Description() string { return "Adds custom checks" }
func (p *MyPlugin) Init(_ *plugin.Context) error { return nil }
func (p *MyPlugin) Shutdown() error     { return nil }
func (p *MyPlugin) RegisterChecks(r *engine.CheckRegistry) error {
    r.Register(myNewCheck())
    return nil
}

// Register the plugin in core/plugins/registry.go
```

Plugin interfaces: `CheckPlugin`, `ExportPlugin`, `MiddlewarePlugin`.

## Developer Guide

- `docs/CODEBASE_GUIDE.md` explains where to add checks, adapter behavior, CLI features and GUI/API behavior.
- `docs/ISSUE_STARTING_POINTS.md` maps each open roadmap issue to concrete codebase entry points.

## File Structure

```
ProxyDoctor/
├── setup.sh              ← One-time setup (install deps, test, build)
├── run.sh                ← Convenience launcher (cli, server, test)
├── cmd/
│   ├── cli/              ← CLI application (diagnose, list-checks)
│   └── server/           ← HTTP API server (minimal, stdlib only) + web GUI
├── core/
│   ├── engine/           ← Orchestration engine (tests included)
│   ├── check/            ← Result types and interfaces (tests included)
│   ├── checks/           ← Built-in diagnostic checks (public_ip, dns_resolve, tls_cert, port_scan, ipv6_leak)
│   ├── adapters/         ← Proxy implementations (Direct, HTTP, HTTPS, SOCKS4, SOCKS5)
│   ├── plugin/           ← Plugin system interfaces and lifecycle manager
│   ├── plugins/          ← Plugin implementations (route_trace, mcp_server)
│   └── utils/            ← Shared helpers (proxy URL parsing)
├── go.mod, go.sum        ← Go modules
├── README.md
├── ARCHITECTURE.md
├── CHANGELOG.md
├── NEXT_STEPS.md
└── VERSION
```

## Built-in Checks

| Check | Category | Description |
|---|---|---|
| `public_ip` | network | Detects public IP address via ipify.org, icanhazip.com, ifconfig.me |
| `dns_resolve` | network | Resolves hostname to IP addresses through the current connection |
| `tls_certificate` | tls | Validates TLS certificate (issuer, expiry, cipher suite, TLS version) |
| `port_connectivity` | network | Tests TCP connectivity to ports 80, 443, 8080, 8443 |
| `ipv6_leak` | leak | Detects whether IPv6 traffic bypasses the configured proxy/tunnel and exposes the system's real public IPv6 address |

## Plugin Checks

| Check | Category | Plugin ID | Description |
|---|---|---|---|
| `route_trace` | network | `route_trace` | Traces network hops to the target and annotates public hops with country information |

## Proxy Input Formats

The CLI and GUI accept proxy URLs in multiple formats:

| Format | Example | Notes |
|---|---|---|
| `scheme://host:port` | `socks5://77.245.76.107:1080` | Auto-detects type from scheme |
| `host:port` + type | `77.245.76.107:1080` + `--proxy-type socks5` | Requires explicit type |
| `host` + type | `77.245.76.107` + `--proxy-type socks5` | Uses default port (1080 for SOCKS, 8080 for HTTP) |
| With auth | `socks5://user:pass@host:port` | Credentials extracted from URL |

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on how to get started.

- 🐛 [Report a Bug](https://github.com/francomano/ProxyDoctor/issues/new?template=bug_report.md)
- ✨ [Request a Feature](https://github.com/francomano/ProxyDoctor/issues/new?template=feature_request.md)
- 💡 [Good First Issues](https://github.com/francomano/ProxyDoctor/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Project Activity

<p align="center">
  <a href="https://github.com/francomano/ProxyDoctor/stargazers"><img src="https://img.shields.io/github/stars/francomano/ProxyDoctor?style=for-the-badge&color=yellow&label=Stars" alt="Stars"></a>
  <a href="https://github.com/francomano/ProxyDoctor/network/members"><img src="https://img.shields.io/github/forks/francomano/ProxyDoctor?style=for-the-badge&color=green&label=Forks" alt="Forks"></a>
  <a href="https://github.com/francomano/ProxyDoctor/issues"><img src="https://img.shields.io/github/issues/francomano/ProxyDoctor?style=for-the-badge&color=red&label=Issues" alt="Issues"></a>
  <a href="https://github.com/francomano/ProxyDoctor/pulls"><img src="https://img.shields.io/github/pulls/francomano/ProxyDoctor?style=for-the-badge&color=purple&label=PRs" alt="Pull Requests"></a>
  <a href="https://github.com/francomano/ProxyDoctor/commits"><img src="https://img.shields.io/github/last-commit/francomano/ProxyDoctor?style=for-the-badge&color=blue" alt="Last Commit"></a>
  <a href="https://github.com/francomano/ProxyDoctor"><img src="https://img.shields.io/github/license/francomano/ProxyDoctor?style=for-the-badge&color=orange" alt="License"></a>
</p>

## Known Issues / Limitations

- Optional DNS leak, WebRTC leak, geolocation, and IP reputation checks are tracked as future work in NEXT_STEPS.md.
