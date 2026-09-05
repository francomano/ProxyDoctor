# ProxyDoctor

**Test your proxy. Find the leaks. Fix the problem.**

[![CI](https://github.com/francomano/ProxyDoctor/actions/workflows/test.yml/badge.svg)](https://github.com/francomano/ProxyDoctor/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/francomano/ProxyDoctor)](https://goreportcard.com/report/github.com/francomano/ProxyDoctor)
[![Go Reference](https://pkg.go.dev/badge/github.com/francomano/ProxyDoctor.svg)](https://pkg.go.dev/github.com/francomano/proxydoctor)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg?logo=go)](https://go.dev/)

<p align="center">
  <img src="images/proxydoctor-logo.png" alt="ProxyDoctor Logo" width="150">
</p>

> **Built with AI assistance** — GitHub Copilot, OpenCode, GPT 5.5, and Gemini Pro.

## What is ProxyDoctor?

ProxyDoctor is a CLI tool that runs network diagnostics through your proxy and tells you exactly what's broken.

- You're behind a proxy/VPN and **sites don't load** — is it DNS? TLS? The proxy itself?
- You think your proxy is private but **your IP is leaking** — through DNS, IPv6, or WebRTC?
- You want to **use the proxy** after testing it — browse, curl, wget through it

ProxyDoctor answers these questions in seconds, then exposes the proxy as a local forward proxy so you can use it immediately.

**v0.5.0** — 9 checks, plugin system, web GUI, Homebrew cask.

## Install

```bash
# go install
go install github.com/francomano/proxydoctor/cmd/cli@latest
alias proxydoctor="$(go env GOPATH)/bin/cli"

# Homebrew
brew install francomano/proxydoctor/proxydoctor

# Binary download
# https://github.com/francomano/ProxyDoctor/releases
```

## Quick Start

```bash
# Run all checks — direct connection
proxydoctor diagnose --url https://example.com

# Run all checks — through a proxy
proxydoctor diagnose --url https://example.com --proxy socks5://user:pass@1.2.3.4:1080

# Compare direct vs proxied
proxydoctor diagnose --url https://example.com --proxy socks5://1.2.3.4:1080 --compare

# Run specific checks only
proxydoctor diagnose --url https://example.com --checks public_ip,dns_leak,webrtc_leak

# Export as JSON
proxydoctor diagnose --url https://example.com --export json --output report.json
```

### Web GUI

```bash
proxydoctor-server
# Open http://localhost:8080
```

## Shell Completion

Tab completion is available for bash, zsh, fish, and PowerShell. Generate a
script and source it once per shell:

```bash
# bash (Linux)
proxydoctor completion bash | sudo tee /etc/bash_completion.d/proxydoctor >/dev/null

# bash (macOS)
proxydoctor completion bash > "$(brew --prefix)/etc/bash_completion.d/proxydoctor"

# zsh — save to a directory on your $fpath
proxydoctor completion zsh > "${fpath[1]}/_proxydoctor"

# fish
proxydoctor completion fish > ~/.config/fish/completions/proxydoctor.fish

# PowerShell
proxydoctor completion powershell >> $PROFILE
```

Start a new shell, then tab through subcommands and flags. Beyond flag *names*,
flag *values* complete too: `--checks` offers check IDs and categories
(`public_ip`, `dns_leak`, `network`, `all`, …), `--export` offers
`text`/`json`/`html`/`markdown`, and `--proxy-type` offers
`auto`/`http`/`https`/`socks4`/`socks5`.

> The generated script is keyed to the root command name `proxyctl`, so
> completion activates for that name. If you invoke the binary under a different
> name (the `proxydoctor` alias, or a raw `go build` binary), install the script
> for each name you use.

## Checks

Every check tells you **what it tests** and **what service it uses**.

### Built-in Checks

| Check | What it does | Service / Method |
|---|---|---|
| `public_ip` | Detects your public IP address | [ipify.org](https://ipify.org), [icanhazip.com](https://icanhazip.com), [ifconfig.me](https://ifconfig.me) |
| `dns_resolve` | Resolves the target hostname to IPs | System DNS (via Go `net` package) |
| `tls_certificate` | Validates TLS cert (expiry, issuer, cipher) | Direct TLS handshake with the target host |
| `port_connectivity` | Tests TCP connectivity to common ports | TCP connect to ports 80, 443, 8080, 8443 |
| `ipv6_leak` | Detects if IPv6 bypasses the proxy | [api6.ipify.org](https://api6.ipify.org), [ipv6.icanhazip.com](https://icanhazip.com), [v6.ident.me](https://v6.ident.me) |
| `dns_leak` | Compares DNS through proxy vs direct path | System DNS on both adapter paths |
| `webrtc_leak` | Detects if STUN/ICE could leak the real IP | STUN probes to Google, Twilio, and Viagenie servers via UDP |
| `header_leak` | Detects if forwarded headers leak the real client IP or internal network metadata | [httpbin.org/headers](https://httpbin.org/headers), [httpbin.org/ip](https://httpbin.org/ip) |
| `proxy_fingerprint` | Auto-detects the proxy protocol and validates it against the configured type | Probes the proxy with SOCKS5, SOCKS4 and HTTP CONNECT greetings |

### Plugin Checks

| Check | What it does | Service / Method |
|---|---|---|
| `route_trace` | Traces network hops with country flags | System `traceroute`/`tracepath` + [ipapi.co](https://ipapi.co) for geolocation |

## Plugins

Load plugins with `--plugins`:

```bash
# Route trace
proxydoctor diagnose --url https://example.com --plugins route_trace

# MCP server (AI assistant integration)
proxydoctor --plugins mcp_server

# Local forward proxy (browse through the tested proxy)
proxydoctor --plugins local_proxy --proxy socks5://1.2.3.4:1080
```

| Plugin | Type | What it does |
|---|---|---|
| `route_trace` | check | Adds route tracing with country annotations |
| `mcp_server` | standalone | Exposes `diagnose`/`compare` as MCP tools on `:9090` |
| `local_proxy` | standalone | Exposes the proxy on `127.0.0.1:8081` for browser/curl/wget |

### Local Proxy Usage

Once the local proxy is running:

```bash
curl -x http://127.0.0.1:8081 https://example.com
# Or set browser HTTP/HTTPS proxy to 127.0.0.1:8081
```

## Proxy Formats

```
socks5://user:pass@host:port     # with auth
socks5://host:port               # no auth
socks4://host:port               # SOCKS4/4a
http://host:port                 # HTTP proxy
host:port --proxy-type socks5    # bare host + type
host --proxy-type http           # bare host (default port)
```

## API

```bash
# List checks
curl http://localhost:8080/api/checks

# Diagnose
curl -X POST http://localhost:8080/api/diagnose \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","proxy":"socks5://1.2.3.4:1080","proxy_type":"socks5"}'

# Local proxy
curl -X POST http://localhost:8080/api/local-proxy/start \
  -H "Content-Type: application/json" \
  -d '{"proxy":"socks5://1.2.3.4:1080","proxy_type":"socks5"}'
```

## Development

```bash
# Setup
git clone https://github.com/francomano/proxydoctor
cd ProxyDoctor
./setup.sh

# Test
go test ./...

# Build
go build ./cmd/cli && go build ./cmd/server
```

- [CODEBASE_GUIDE.md](docs/CODEBASE_GUIDE.md) — where to add checks, adapters, CLI features
- [CONTRIBUTING.md](CONTRIBUTING.md) — PR process, code style, test patterns

## File Structure

```
cmd/cli/              CLI (diagnose, list-checks, version)
cmd/server/           HTTP server + web GUI
core/engine/          Orchestration engine + dependency DAG
core/check/           Checker interface + result types
core/checks/          Built-in checks (public_ip, dns_resolve, tls_cert, port_scan, ipv6_leak, dns_leak, webrtc_leak, header_leak)
core/adapters/        Proxy implementations (Direct, HTTP, HTTPS, SOCKS4, SOCKS5)
core/plugin/          Plugin system interfaces
core/plugins/         Plugin implementations (route_trace, mcp_server, local_proxy)
internal/testproxy/   Hermetic proxy fixtures for integration tests
```

## Contributing

- [Report a Bug](https://github.com/francomano/ProxyDoctor/issues/new?template=bug_report.md)
- [Request a Feature](https://github.com/francomano/ProxyDoctor/issues/new?template=feature_request.md)
- [Good First Issues](https://github.com/francomano/ProxyDoctor/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)

## License

GPL-3.0
