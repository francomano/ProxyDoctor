package portscan

import (
	"fmt"
	"net/url"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

type PortScanCheck struct{}

func NewPortScanCheck() check.Checker { return &PortScanCheck{} }

func (c *PortScanCheck) ID() string   { return "port_connectivity" }
func (c *PortScanCheck) Name() string { return "Port Connectivity" }
func (c *PortScanCheck) Description() string {
	return "Tests TCP connectivity to common ports (80, 443, 8080, 8443) on the target host"
}
func (c *PortScanCheck) Category() check.CheckCategory { return check.CategoryNetwork }
func (c *PortScanCheck) DependsOn() []string           { return []string{} }

func (c *PortScanCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
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

	ports := []int{80, 443, 8080, 8443}
	timeout := 5 * time.Second
	openPorts := []int{}
	closedPorts := []int{}
	portErrors := map[int]string{}

	for _, port := range ports {
		open, err := adapter.TestPort(hostname, port, timeout)
		if err != nil {
			portErrors[port] = err.Error()
			continue
		}
		if open {
			openPorts = append(openPorts, port)
		} else {
			closedPorts = append(closedPorts, port)
		}
	}

	ctx.SetSharedData("open_ports", openPorts)
	result.SetExecutionTime(time.Since(startTime))
	result.AddEvidence("hostname", hostname).
		AddEvidence("open_ports", openPorts).
		AddEvidence("closed_ports", closedPorts)
	if len(portErrors) > 0 {
		result.AddEvidence("port_errors", portErrors)
	}

	if len(openPorts) == 0 && len(portErrors) == len(ports) {
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("Could not test any configured port on %s", hostname)).
			WithConfidence(0).
			AddProbableCause("Proxy or network connectivity errors prevented port testing").
			AddSuggestedAction("Verify the proxy configuration and target hostname")
	}

	if len(openPorts) == 0 {
		return *result.WithStatus(check.StatusFailed, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("No open ports detected on %s (tested: %v)", hostname, ports)).
			WithConfidence(0.9).
			AddProbableCause("Host may be unreachable through the selected connection").
			AddProbableCause("Firewall may be blocking connections").
			AddSuggestedAction("Verify the proxy is working and the target host is online")
	}

	return *result.WithStatus(check.StatusPassed, check.SeverityInfo).
		WithExplanation(fmt.Sprintf("%d open port(s) on %s: %v", len(openPorts), hostname, openPorts)).
		WithConfidence(0.9)
}
