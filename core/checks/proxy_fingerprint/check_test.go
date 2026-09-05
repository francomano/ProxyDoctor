package proxyfingerprint

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

// startMockProxy launches a TCP listener that classifies each inbound
// connection by the first byte it receives and replies with the matching
// protocol greeting. dialect selects which protocol the mock "speaks":
// "socks5", "socks4", "http". The returned address is the dial target.
func startMockProxy(t *testing.T, dialect string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				handleMock(c, dialect)
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close(); wg.Wait() })

	return ln.Addr().String()
}

func handleMock(c net.Conn, dialect string) {
	// Read the first byte to inspect which probe is being sent, so a single
	// mock endpoint can service all three probes and reply consistently with
	// its dialect.
	buf := make([]byte, 1)
	if _, err := io.ReadFull(c, buf); err != nil {
		return
	}
	switch dialect {
	case "socks5":
		// Reply 05 00 regardless of the probe byte: only a real SOCKS5 probe
		// (which sent 05 01 00) treats 0x05 as a version match. SOCKS4 and
		// HTTP probes read a 2-byte/line reply that will not match their
		// classifiers, so only the SOCKS5 probe records a match.
		_, _ = c.Write([]byte{0x05, 0x00})
	case "socks4":
		// Reply 00 5a + 6 padding bytes; SOCKS4 probe checks resp[0]==0x00.
		_, _ = c.Write([]byte{0x00, 0x5a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	case "http":
		// Reply with an HTTP status line; HTTP probe checks "HTTP/" prefix.
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	case "silent":
		// Accept, then never reply: drain inbound bytes until the probe gives
		// up and closes. The probe's read must hit its deadline rather than
		// an immediate EOF so we can prove the timeouts are enforced.
		_, _ = io.Copy(io.Discard, c)
	}
}

func runCheck(t *testing.T, c *ProxyFingerprintCheck, proxyType check.ProxyType, addr string) check.CheckResult {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr)
	ctx := &fakeCtx{cfg: check.ProxyConfig{Type: proxyType, Host: host, Port: parsePort(port)}}
	return c.Execute(ctx)
}

func parsePort(s string) int {
	p, _ := net.LookupPort("tcp", s)
	return p
}

type fakeCtx struct {
	cfg check.ProxyConfig
}

func (f *fakeCtx) GetURL() string                              { return "" }
func (f *fakeCtx) GetProxyConfig() check.ProxyConfig           { return f.cfg }
func (f *fakeCtx) GetDirectAdapter() check.NetworkAdapter      { return nil }
func (f *fakeCtx) GetProxyAdapter() check.NetworkAdapter       { return nil }
func (f *fakeCtx) GetSharedData(key string) interface{}        { return nil }
func (f *fakeCtx) SetSharedData(key string, value interface{}) {}
func (f *fakeCtx) GetTimeout() time.Duration                   { return 5 * time.Second }
func (f *fakeCtx) IsCancelled() bool                           { return false }

func TestSkipDirect(t *testing.T) {
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	ctx := &fakeCtx{cfg: check.ProxyConfig{Type: check.ProxyTypeDirect}}
	r := c.Execute(ctx)
	if r.Status != check.StatusSkipped {
		t.Fatalf("direct connection should skip, got %s", r.Status)
	}
}

func TestDetectSOCKS5Matches(t *testing.T) {
	addr := startMockProxy(t, "socks5")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeSOCKS5, addr)
	if r.Status != check.StatusPassed {
		t.Fatalf("socks5 proxy with declared socks5 should pass, got %s: %s", r.Status, r.Explanation)
	}
}

func TestDetectSOCKS4Matches(t *testing.T) {
	addr := startMockProxy(t, "socks4")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeSOCKS4, addr)
	if r.Status != check.StatusPassed {
		t.Fatalf("socks4 proxy with declared socks4 should pass, got %s: %s", r.Status, r.Explanation)
	}
}

func TestDetectHTTPMatches(t *testing.T) {
	addr := startMockProxy(t, "http")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeHTTP, addr)
	if r.Status != check.StatusPassed {
		t.Fatalf("http proxy with declared http should pass, got %s: %s", r.Status, r.Explanation)
	}
}

func TestHTTPSDeclaredSatisfiesHTTPDetected(t *testing.T) {
	addr := startMockProxy(t, "http")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeHTTPS, addr)
	if r.Status != check.StatusPassed {
		t.Fatalf("https-declared proxy that answers plaintext CONNECT should pass, got %s: %s", r.Status, r.Explanation)
	}
}

func TestMismatchSuggestsDetected(t *testing.T) {
	addr := startMockProxy(t, "socks5")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeHTTP, addr)
	if r.Status != check.StatusFailed {
		t.Fatalf("declared http over a socks5 proxy should fail, got %s", r.Status)
	}
	if !strings.Contains(r.Explanation, "socks5") {
		t.Errorf("explanation should name the detected protocol: %q", r.Explanation)
	}
	foundSuggestion := false
	for _, a := range r.SuggestedActions {
		if strings.Contains(a, "socks5") {
			foundSuggestion = true
		}
	}
	if !foundSuggestion {
		t.Errorf("expected a suggested action recommending socks5, got %v", r.SuggestedActions)
	}
}

func TestAutoDeclaresDetected(t *testing.T) {
	addr := startMockProxy(t, "socks4")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, "auto", addr)
	if r.Status != check.StatusPassed {
		t.Fatalf("auto-detection should pass when a protocol is identified, got %s", r.Status)
	}
	if !strings.Contains(r.Explanation, "socks4") {
		t.Errorf("auto verdict should report the detected protocol: %q", r.Explanation)
	}
}

func TestUnreachableEndpointErrors(t *testing.T) {
	// Pick a port that is almost certainly closed.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	c := &ProxyFingerprintCheck{dialer: stdDialer{}}
	r := runCheck(t, c, check.ProxyTypeSOCKS5, addr)
	if r.Status != check.StatusError {
		t.Fatalf("unreachable proxy should yield error, got %s: %s", r.Status, r.Explanation)
	}
}

// TestSilentPeerIsBounded guards the per-connection read/write deadlines: a
// peer that accepts the TCP handshake but never answers a greeting must not
// stall the check. With a shortened probeTimeout the whole run must finish
// well under the default 5s per-probe budget and report an error.
func TestSilentPeerIsBounded(t *testing.T) {
	orig := probeTimeout
	probeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { probeTimeout = orig })

	addr := startMockProxy(t, "silent")
	c := &ProxyFingerprintCheck{dialer: stdDialer{}}

	start := time.Now()
	r := runCheck(t, c, check.ProxyTypeSOCKS5, addr)
	elapsed := time.Since(start)

	// Three probes each wait out probeTimeout before giving up, so the upper
	// bound is generous compared to the ~600ms the shortened deadline implies.
	if elapsed > 3*time.Second {
		t.Fatalf("silent peer stalled the check for %v, exceeding the deadline budget", elapsed)
	}
	if r.Status != check.StatusError {
		t.Fatalf("a silent peer should yield an error, got %s: %s", r.Status, r.Explanation)
	}
}

// --- pure verdict tests (no network) ---

func TestEvaluateNoDetectionErrors(t *testing.T) {
	v := evaluateFingerprint(nil, "socks5", nil)
	if v.status != check.StatusError {
		t.Fatalf("expected error, got %s", v.status)
	}
}

func TestEvaluateAutoPasses(t *testing.T) {
	v := evaluateFingerprint([]string{"http"}, "auto", nil)
	if v.status != check.StatusPassed {
		t.Fatalf("expected passed, got %s", v.status)
	}
}

func TestEvaluateMismatchFails(t *testing.T) {
	v := evaluateFingerprint([]string{"socks5"}, "socks4", nil)
	if v.status != check.StatusFailed {
		t.Fatalf("expected failed, got %s", v.status)
	}
	if !strings.Contains(v.explanation, "socks5") {
		t.Errorf("explanation should name detected protocol: %q", v.explanation)
	}
}

func TestEvaluateHTTPSSHTTPSMatch(t *testing.T) {
	if !typeMatches("https", "http") {
		t.Fatal("https-declared should match http-detected")
	}
	if typeMatches("socks4", "socks5") {
		t.Fatal("socks4/socks5 should not match")
	}
}

func TestEvaluateAmbiguousFails(t *testing.T) {
	v := evaluateFingerprint([]string{"socks5", "http"}, "socks5", nil)
	if v.status != check.StatusFailed {
		t.Fatalf("expected failed for multiprotocol, got %s", v.status)
	}
}

// ensure the check still completes promptly; the hermetic mock makes this a
// fast test, but a regression that drops the per-probe deadline would hang.
func TestProbeTimeoutIsBounded(t *testing.T) {
	if probeTimeout > 10*time.Second {
		t.Fatalf("probeTimeout grew too large: %v", probeTimeout)
	}
}
