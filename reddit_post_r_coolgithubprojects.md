# Reddit Post — r/coolgithubprojects

## Title

I built ProxyDoctor — a CLI tool that tells you exactly what's broken with your proxy/VPN (DNS leaks, TLS issues, route tracing)

Hello everyone,
I've been working on **ProxyDoctor** — a Go CLI tool that diagnoses proxy and VPN issues by running structured network checks and telling you exactly what layer is failing.

**What it does:**
- Runs 5 built-in checks (DNS resolution, public IP detection, TLS certificate validation, port connectivity, IPv6 leak detection)
- Traces network hops with country flags 🗺️
- Compares direct vs proxied connections side-by-side
- Supports HTTP, HTTPS, SOCKS4, SOCKS5 proxies
- Exports to JSON, Markdown, HTML, or plain text

**Why I built this:** I was debugging corporate proxy issues and got tired of running 5 different tools to figure out if it was DNS, TLS, or the proxy itself. Wanted one command that tells me the answer.

Would love feedback on the architecture or feature ideas. Issues welcome!

🔗 https://github.com/francomano/ProxyDoctor