package ipv6leak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

const (
	// probeTimeout bounds the direct (non-proxied) IPv6 reachability probes.
	probeTimeout = 8 * time.Second

	// ipv6ProbeHost/ipv6ProbePort is a stable, well-known IPv6 address (Cloudflare)
	// used to test whether the configured proxy/tunnel can forward IPv6 destinations.
	ipv6ProbeHost = "2606:4700:4700::1111"
	ipv6ProbePort = 443
)

// ipv6DetectionServices are IPv6-only endpoints used to discover the system's real
// public IPv6 address when reaching out directly, bypassing the configured proxy.
var ipv6DetectionServices = []string{
	"https://api6.ipify.org?format=json",
	"https://ipv6.icanhazip.com/",
	"https://v6.ident.me/",
}

// IPv6LeakCheck detects whether IPv6 traffic bypasses the configured proxy/tunnel.
type IPv6LeakCheck struct{}

// NewIPv6LeakCheck creates a new IPv6 leak detection check.
func NewIPv6LeakCheck() check.Checker { return &IPv6LeakCheck{} }

func (c *IPv6LeakCheck) ID() string   { return "ipv6_leak" }
func (c *IPv6LeakCheck) Name() string { return "IPv6 Leak Detection" }
func (c *IPv6LeakCheck) Description() string {
	return "Detects whether IPv6 traffic bypasses the configured proxy/tunnel and exposes the system's real public IPv6 address"
}
func (c *IPv6LeakCheck) Category() check.CheckCategory { return check.CategoryLeakDetection }
func (c *IPv6LeakCheck) DependsOn() []string           { return []string{"dns_resolve"} }

func (c *IPv6LeakCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

	proxyConfig := ctx.GetProxyConfig()
	if proxyConfig.Type == check.ProxyTypeDirect {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusSkipped, check.SeverityInfo).
			WithExplanation("No proxy or tunnel is configured; IPv6 leak detection only applies when traffic is expected to be tunneled").
			WithConfidence(0)
	}

	parsed, err := url.Parse(ctx.GetURL())
	if err != nil {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("Invalid URL: %v", err)).
			WithConfidence(0)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation("Invalid URL: missing hostname").
			WithConfidence(0)
	}

	targetIPv6Addrs := targetIPv6Addresses(ctx.GetSharedData("dns_ips"), hostname)
	targetSupportsIPv6 := len(targetIPv6Addrs) > 0
	result.AddEvidence("target_ipv6_addresses", targetIPv6Addrs).
		AddEvidence("target_supports_ipv6", targetSupportsIPv6)

	systemIPv6, systemErr := detectDirectPublicIPv6(probeTimeout)
	systemSupportsIPv6 := systemErr == nil && systemIPv6 != ""
	result.AddEvidence("system_supports_ipv6", systemSupportsIPv6)
	if systemSupportsIPv6 {
		result.AddEvidence("system_public_ipv6", systemIPv6)
	} else if systemErr != nil {
		result.AddEvidence("system_ipv6_error", systemErr.Error())
	}

	var proxyForwardsIPv6 bool
	if systemSupportsIPv6 {
		adapter := ctx.GetProxyAdapter()
		forwards, proxyErr := adapter.TestPort(ipv6ProbeHost, ipv6ProbePort, probeTimeout)
		proxyForwardsIPv6 = forwards
		result.AddEvidence("proxy_forwards_ipv6", proxyForwardsIPv6)
		if proxyErr != nil {
			result.AddEvidence("proxy_ipv6_error", proxyErr.Error())
		}
	}

	result.SetExecutionTime(time.Since(startTime))

	verdict := evaluateIPv6Leak(systemSupportsIPv6, proxyForwardsIPv6, targetSupportsIPv6, hostname, systemIPv6, targetIPv6Addrs)

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

// leakVerdict is the pure, testable outcome of the IPv6 leak decision.
type leakVerdict struct {
	status      check.Status
	severity    check.Severity
	explanation string
	confidence  float64
	causes      []string
	actions     []string
}

// evaluateIPv6Leak decides whether IPv6 traffic would bypass the configured proxy/tunnel.
func evaluateIPv6Leak(systemSupportsIPv6, proxyForwardsIPv6, targetSupportsIPv6 bool, hostname, systemIPv6 string, targetIPv6Addrs []string) leakVerdict {
	if !systemSupportsIPv6 {
		return leakVerdict{
			status:      check.StatusPassed,
			severity:    check.SeverityInfo,
			explanation: "System has no outbound IPv6 connectivity; IPv6 cannot bypass the proxy/tunnel",
			confidence:  0.85,
		}
	}

	if proxyForwardsIPv6 {
		return leakVerdict{
			status:      check.StatusPassed,
			severity:    check.SeverityInfo,
			explanation: "System has working IPv6 connectivity, but the configured proxy/tunnel successfully forwards IPv6 connections; no leak detected",
			confidence:  0.8,
		}
	}

	severity := check.SeverityWarning
	explanation := fmt.Sprintf(
		"System has working IPv6 connectivity (public IPv6: %s) but the configured proxy/tunnel does not forward IPv6 traffic",
		systemIPv6,
	)
	if targetSupportsIPv6 {
		severity = check.SeverityCritical
		explanation = fmt.Sprintf(
			"%s; %s is reachable over IPv6 (%s) and connections to it would bypass the proxy, exposing your real public IPv6 address (%s)",
			explanation, hostname, strings.Join(targetIPv6Addrs, ", "), systemIPv6,
		)
	} else {
		explanation += "; this target has no IPv6 records today, but any IPv6-capable destination would bypass the tunnel the same way"
	}

	return leakVerdict{
		status:      check.StatusFailed,
		severity:    severity,
		explanation: explanation,
		confidence:  0.8,
		causes: []string{
			"The proxy/tunnel protocol or implementation does not support forwarding IPv6 destinations (for example, SOCKS4 has no IPv6 address type)",
			"The operating system prefers a direct IPv6 route over the tunnel's default route",
		},
		actions: []string{
			"Disable IPv6 system-wide or block outbound IPv6 while using this proxy/tunnel",
			"Use a proxy/tunnel implementation that supports IPv6 forwarding, or configure the client to force IPv4-only traffic",
		},
	}
}

// targetIPv6Addresses extracts globally routable IPv6 addresses for hostname from
// shared DNS results when available, falling back to a direct lookup otherwise.
func targetIPv6Addresses(sharedDNSIPs interface{}, hostname string) []string {
	ips := stringsFromShared(sharedDNSIPs)
	if len(ips) == 0 {
		if resolved, err := net.LookupHost(hostname); err == nil {
			ips = resolved
		}
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(ips))
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if !addr.Is6() || addr.Is4In6() {
			continue
		}
		s := addr.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}
	return result
}

func stringsFromShared(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// detectDirectPublicIPv6 discovers the system's real public IPv6 address by
// reaching out directly (bypassing any configured proxy) over a forced IPv6
// connection to a set of IPv6-only echo services.
func detectDirectPublicIPv6(timeout time.Duration) (string, error) {
	dialer := &net.Dialer{Timeout: timeout}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp6", addr)
			},
		},
	}

	var lastErr error
	for _, serviceURL := range ipv6DetectionServices {
		ip, err := fetchIPFromService(client, serviceURL)
		if err != nil {
			lastErr = err
			continue
		}
		addr, err := netip.ParseAddr(ip)
		if err != nil || !addr.Is6() || addr.Is4In6() {
			lastErr = fmt.Errorf("service %s did not return a valid IPv6 address: %q", serviceURL, ip)
			continue
		}
		return addr.String(), nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no IPv6 detection service reachable")
}

func fetchIPFromService(client *http.Client, serviceURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, serviceURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ProxyDoctor/0.1")
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to %s failed: %w", serviceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned HTTP %d", serviceURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("failed to read response from %s: %w", serviceURL, err)
	}

	return parseIPResponse(body)
}

func parseIPResponse(body []byte) (string, error) {
	var jsonResp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &jsonResp); err == nil && strings.TrimSpace(jsonResp.IP) != "" {
		return strings.TrimSpace(jsonResp.IP), nil
	}
	candidate := strings.TrimSpace(string(body))
	if candidate == "" {
		return "", fmt.Errorf("empty IP response")
	}
	return candidate, nil
}
