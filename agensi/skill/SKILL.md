---
name: proxydoctor-network-diagnostics
description: Diagnose proxy connectivity, routing, DNS, TLS, IP, IPv6, WebRTC, and header leaks with the ProxyDoctor MCP tools. Use when a user needs to troubleshoot or compare an authorized proxy connection; do not use it for host discovery, port scanning, or testing systems they are not authorized to assess.
---

# ProxyDoctor network diagnostics

Use the ProxyDoctor MCP server to explain why an authorized proxy connection
fails, whether it leaks identifying network information, and how its behaviour
differs from a direct connection. Its tools return evidence and recommendations;
they do not change proxy, browser, firewall, or system configuration.

## Before running a diagnostic

- Confirm the URL and proxy belong to the user or are explicitly authorized for
  testing. Treat a proxy as sensitive infrastructure.
- Use the smallest useful test. Do not turn this skill into reconnaissance,
  target discovery, bulk scanning, or an attempt to bypass network controls.
- Do not repeat a proxy URL containing credentials in the final answer, logs,
  tickets, or chat. Refer to it as “the supplied proxy”.

If the ProxyDoctor tools are not available, tell the user that the local MCP
server must be running and connected to their agent. In a local installation,
start it with:

```sh
proxydoctor --plugins mcp_server
```

The server listens on port 9090 and exposes its MCP endpoint at `/mcp`.

## Choose the right tool

1. Call `list_checks` when the user asks what can be tested or when a focused
   diagnosis needs a check inventory.
2. Call `diagnose` for one authorized URL, directly or through the supplied
   proxy. Pass `url`, and pass `proxy` and `proxy_type` when applicable.
3. Call `compare` when the user wants to identify what changes between the
   direct path and the proxy path. It requires both `url` and `proxy`.

Use `proxy_type: "auto"` unless the user knows the type. Accepted explicit
values are `http`, `https`, `socks4`, and `socks5`.

## Interpret and report results

Summarize the material findings first: connectivity failures, protocol mismatch,
DNS differences, certificate issues, and any IP, IPv6, WebRTC, or forwarded
header leak. Distinguish a confirmed result from an inconclusive result caused
by timeouts, blocked third-party services, or unavailable IPv6/UDP support.

For a comparison, state which path produced each observation. Do not present a
different public IP by itself as a leak; it is normally expected when traffic
uses a proxy. Flag it only when the proxy path reveals the direct-path identity
or when the tool's evidence identifies a bypass.

Offer the least disruptive next step based on the evidence, such as checking
proxy credentials or protocol, correcting DNS routing, renewing a certificate,
or disabling an unneeded IPv6/WebRTC path. Ask for approval before changing
configuration or running further tests.
