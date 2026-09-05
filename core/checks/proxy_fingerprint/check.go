package proxyfingerprint

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

// probeDialer is the minimal dial surface the fingerprint probes need. It is an
// interface so the unit tests can inject a hermetic listener without touching
// real network endpoints.
type probeDialer interface {
	DialTimeout(network, address string, timeout time.Duration) (net.Conn, error)
}

// stdDialer adapts the net package to probeDialer.
type stdDialer struct{}

func (stdDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}

// probeTimeout is the per-handshake deadline. Fingerprinting opens one fresh
// connection per protocol so a misbehaving peer cannot starve the others. It is
// a var (not a const) so tests can shorten it and exercise the timeout path
// without waiting 5s per probe.
var probeTimeout = 5 * time.Second

// setProbeDeadline bounds every read and write on a freshly dialed connection
// so a peer that accepts the TCP handshake but then stays silent cannot stall
// the check past probeTimeout.
func setProbeDeadline(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(probeTimeout))
}

// fingerprintTarget is the host:port used inside the SOCKS4/HTTP-CONNECT probe
// payloads. The proxy is only asked to *attempt* a connect; whether it reaches
// the target is irrelevant — we classify by how it answers the greeting.
const fingerprintTarget = "1.1.1.1:53"

// ProxyFingerprintCheck auto-detects which proxy protocol a server actually
// speaks by probing it with SOCKS5, SOCKS4 and HTTP CONNECT greetings, then
// validates that against the user-configured proxy type.
type ProxyFingerprintCheck struct {
	dialer probeDialer
}

// NewProxyFingerprintCheck creates a new proxy protocol fingerprint check.
func NewProxyFingerprintCheck() check.Checker {
	return &ProxyFingerprintCheck{dialer: stdDialer{}}
}

func (c *ProxyFingerprintCheck) ID() string { return "proxy_fingerprint" }

func (c *ProxyFingerprintCheck) Name() string { return "Proxy Protocol Fingerprint" }

func (c *ProxyFingerprintCheck) Description() string {
	return "Auto-detects the proxy protocol (SOCKS5, SOCKS4 or HTTP CONNECT) by probing the server and validates that it matches the configured proxy type, suggesting the correct type on mismatch"
}

func (c *ProxyFingerprintCheck) Category() check.CheckCategory { return check.CategoryProtocol }

func (c *ProxyFingerprintCheck) DependsOn() []string { return []string{} }

func (c *ProxyFingerprintCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

	cfg := ctx.GetProxyConfig()
	if cfg.Type == check.ProxyTypeDirect {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusSkipped, check.SeverityInfo).
			WithExplanation("No proxy is configured; protocol fingerprinting only applies to a configured proxy endpoint").
			WithConfidence(0)
	}

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	result.AddEvidence("proxy_endpoint", address)
	result.AddEvidence("declared_type", string(cfg.Type))

	// Use the injected dialer (tests override it); fall back to the real net
	// dialer when the check was constructed via NewProxyFingerprintCheck.
	dialer := c.dialer
	if dialer == nil {
		dialer = stdDialer{}
	}

	probes := map[string]func(string, probeDialer) probeResult{
		"socks5": probeSOCKS5,
		"socks4": probeSOCKS4,
		"http":   probeHTTPConnect,
	}

	var detected []string
	details := make(map[string]string, len(probes))
	for proto, fn := range probes {
		pr := fn(address, dialer)
		result.AddEvidence(proto+"_probe", pr.detail)
		if pr.matched {
			detected = append(detected, proto)
		}
	}

	verdict := evaluateFingerprint(detected, string(cfg.Type), details)
	result.SetExecutionTime(time.Since(startTime))

	result.WithStatus(verdict.status, verdict.severity).
		WithExplanation(verdict.explanation).
		WithConfidence(verdict.confidence)
	for _, cause := range verdict.causes {
		result.AddProbableCause(cause)
	}
	for _, action := range verdict.actions {
		result.AddSuggestedAction(action)
	}
	return *result
}

// probeResult is the outcome of a single protocol greeting.
type probeResult struct {
	matched bool
	detail  string
}

// probeSOCKS5 sends a no-auth method negotiation (VER=5, NMETHODS=1, 0x00) and
// classifies the peer as SOCKS5 iff the reply's version byte is 0x05.
func probeSOCKS5(address string, dialer probeDialer) probeResult {
	conn, err := dialer.DialTimeout("tcp", address, probeTimeout)
	if err != nil {
		return probeResult{detail: fmt.Sprintf("dial: %v", err)}
	}
	defer conn.Close()
	setProbeDeadline(conn)

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return probeResult{detail: fmt.Sprintf("greeting write: %v", err)}
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return probeResult{detail: fmt.Sprintf("greeting read: %v", err)}
	}
	if resp[0] != 0x05 {
		return probeResult{detail: fmt.Sprintf("reply version byte 0x%02x (want 0x05)", resp[0])}
	}
	switch resp[1] {
	case 0x00:
		return probeResult{matched: true, detail: "no-auth accepted (05 00)"}
	case 0x02:
		return probeResult{matched: true, detail: "username/password required (05 02)"}
	case 0xFF:
		return probeResult{matched: true, detail: "no acceptable method (05 ff)"}
	default:
		return probeResult{matched: true, detail: fmt.Sprintf("method 0x%02x", resp[1])}
	}
}

// probeSOCKS4 sends a CONNECT request for fingerprintTarget and classifies the
// peer as SOCKS4 iff the 8-byte reply's version byte is 0x00 (the SOCKS4 reply
// version, distinct from the request version 0x04).
func probeSOCKS4(address string, dialer probeDialer) probeResult {
	conn, err := dialer.DialTimeout("tcp", address, probeTimeout)
	if err != nil {
		return probeResult{detail: fmt.Sprintf("dial: %v", err)}
	}
	defer conn.Close()
	setProbeDeadline(conn)

	host, port, ok := splitHostPort(fingerprintTarget)
	if !ok {
		return probeResult{detail: "internal: bad fingerprint target"}
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return probeResult{detail: "internal: fingerprint target must be IPv4"}
	}

	// VER=4, CMD=1 (CONNECT), port, IP, userid (empty, null-terminated).
	req := []byte{0x04, 0x01, byte(port >> 8), byte(port), ip[0], ip[1], ip[2], ip[3], 0x00}
	if _, err := conn.Write(req); err != nil {
		return probeResult{detail: fmt.Sprintf("request write: %v", err)}
	}

	resp := make([]byte, 8)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return probeResult{detail: fmt.Sprintf("reply read: %v", err)}
	}
	if resp[0] != 0x00 {
		return probeResult{detail: fmt.Sprintf("reply version byte 0x%02x (want 0x00)", resp[0])}
	}
	switch resp[1] {
	case 0x5A:
		return probeResult{matched: true, detail: "request granted (00 5a)"}
	case 0x5B:
		return probeResult{matched: true, detail: "request rejected or failed (00 5b)"}
	case 0x5C:
		return probeResult{matched: true, detail: "identd unavailable (00 5c)"}
	case 0x5D:
		return probeResult{matched: true, detail: "identd mismatch (00 5d)"}
	default:
		return probeResult{matched: true, detail: fmt.Sprintf("status 0x%02x", resp[1])}
	}
}

// probeHTTPConnect sends an HTTP/1.1 CONNECT for fingerprintTarget and
// classifies the peer as an HTTP proxy iff the reply begins with "HTTP/".
func probeHTTPConnect(address string, dialer probeDialer) probeResult {
	conn, err := dialer.DialTimeout("tcp", address, probeTimeout)
	if err != nil {
		return probeResult{detail: fmt.Sprintf("dial: %v", err)}
	}
	defer conn.Close()
	setProbeDeadline(conn)

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: ProxyDoctor-fingerprint\r\n\r\n", fingerprintTarget, fingerprintTarget)
	if _, err := conn.Write([]byte(req)); err != nil {
		return probeResult{detail: fmt.Sprintf("request write: %v", err)}
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		return probeResult{detail: fmt.Sprintf("reply read: %v", err)}
	}
	line := strings.TrimSpace(string(buf[:n]))
	if strings.HasPrefix(strings.ToUpper(line), "HTTP/") {
		return probeResult{matched: true, detail: firstLine(line)}
	}
	return probeResult{detail: fmt.Sprintf("non-HTTP reply: %q", truncate(line, 48))}
}

// fingerprintVerdict is the pure, testable outcome of the fingerprint decision.
type fingerprintVerdict struct {
	status      check.Status
	severity    check.Severity
	explanation string
	confidence  float64
	causes      []string
	actions     []string
}

// evaluateFingerprint decides the check verdict from the detected protocols
// and the user-declared type. detected lists every protocol that answered its
// greeting correctly (normally exactly one). details maps protocol -> probe
// detail for evidence already recorded by the caller.
func evaluateFingerprint(detected []string, declared string, details map[string]string) fingerprintVerdict {
	declared = strings.TrimSpace(strings.ToLower(declared))

	if len(detected) == 0 {
		return fingerprintVerdict{
			status:      check.StatusError,
			severity:    check.SeverityWarning,
			explanation: "The proxy endpoint did not answer any of the SOCKS5, SOCKS4 or HTTP CONNECT greetings; it may be offline, require TLS (an https:// proxy), or speak an unsupported protocol",
			confidence:  0.6,
			causes: []string{
				"The proxy host:port is unreachable or not listening",
				"The proxy expects TLS (configure it as https://) and rejects plaintext handshakes",
				"The proxy speaks a protocol ProxyDoctor cannot fingerprint",
			},
			actions: []string{
				"Verify the proxy address and port with `proxydoctor diagnose --checks port_connectivity`",
				"If the proxy is reached over TLS, configure it with --proxy-type https",
			},
		}
	}

	// Normalise: an HTTP CONNECT reply is consistent with both an http and an
	// https declared proxy, since https is the same application-layer protocol
	// over TLS — the plaintext probe cannot distinguish them.
	primary := detected[0]
	if len(detected) > 1 {
		// Ambiguous but informative: report what spoke.
		return fingerprintVerdict{
			status:      check.StatusFailed,
			severity:    check.SeverityWarning,
			explanation: fmt.Sprintf("The proxy answered more than one greeting (%s); it may be a multiprotocol proxy. Configure it as %s", strings.Join(detected, ", "), primary),
			confidence:  0.7,
			actions:     []string{fmt.Sprintf("Use --proxy-type %s to match the detected protocol", primary)},
		}
	}

	if declared == "" || declared == "auto" {
		return fingerprintVerdict{
			status:      check.StatusPassed,
			severity:    check.SeverityInfo,
			explanation: fmt.Sprintf("Auto-detection configured; the proxy speaks %s", primary),
			confidence:  0.85,
			actions:     []string{fmt.Sprintf("Pin the type with --proxy-type %s to skip detection on future runs", primary)},
		}
	}

	if typeMatches(declared, primary) {
		return fingerprintVerdict{
			status:      check.StatusPassed,
			severity:    check.SeverityInfo,
			explanation: fmt.Sprintf("The proxy speaks %s, matching the configured type %q", primary, declared),
			confidence:  0.9,
		}
	}

	return fingerprintVerdict{
		status:      check.StatusFailed,
		severity:    check.SeverityWarning,
		explanation: fmt.Sprintf("Configured proxy type %q does not match the detected protocol %s; the proxy speaks %s", declared, primary, primary),
		confidence:  0.85,
		causes: []string{
			fmt.Sprintf("The proxy endpoint answers the %s greeting but not the %s greeting", primary, declared),
			"The --proxy-type flag (or proxy URL scheme) was set to a protocol the server does not speak",
		},
		actions: []string{
			fmt.Sprintf("Reconfigure with --proxy-type %s to match the detected protocol", primary),
		},
	}
}

// typeMatches reconciles the declared proxy type with the detected protocol,
// allowing https-declared proxies to satisfy an http detection (same
// application layer over TLS).
func typeMatches(declared, detected string) bool {
	if declared == detected {
		return true
	}
	if declared == string(check.ProxyTypeHTTPS) && detected == string(check.ProxyTypeHTTP) {
		return true
	}
	return false
}

func splitHostPort(s string) (string, int, bool) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return "", 0, false
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return "", 0, false
	}
	return host, port, true
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
