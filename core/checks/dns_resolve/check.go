package dnsresolve

import (
	"fmt"
	"net/url"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

type DNSResolveCheck struct{}

func NewDNSResolveCheck() check.Checker { return &DNSResolveCheck{} }

func (c *DNSResolveCheck) ID() string   { return "dns_resolve" }
func (c *DNSResolveCheck) Name() string { return "DNS Resolution" }
func (c *DNSResolveCheck) Description() string {
	return "Resolves the target hostname to IP addresses through the current connection"
}
func (c *DNSResolveCheck) Category() check.CheckCategory { return check.CategoryNetwork }
func (c *DNSResolveCheck) DependsOn() []string           { return []string{} }

func (c *DNSResolveCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

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

	adapter := ctx.GetProxyAdapter()
	if ctx.GetProxyConfig().Type == check.ProxyTypeDirect {
		adapter = ctx.GetDirectAdapter()
	}

	ips, err := adapter.ResolveDNS(hostname)
	if err != nil {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityWarning).
			WithExplanation(fmt.Sprintf("DNS resolution failed for %s: %v", hostname, err)).
			WithConfidence(0)
	}
	if len(ips) == 0 {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusFailed, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("No DNS records found for %s", hostname)).
			WithConfidence(0.9)
	}

	ctx.SetSharedData("dns_ips", ips)

	result.SetExecutionTime(time.Since(startTime))
	return *result.WithStatus(check.StatusPassed, check.SeverityInfo).
		WithExplanation(fmt.Sprintf("Resolved %s to %d IP(s)", hostname, len(ips))).
		WithConfidence(0.95).
		AddEvidence("hostname", hostname).
		AddEvidence("ips", ips).
		AddEvidence("ip_count", len(ips))
}
