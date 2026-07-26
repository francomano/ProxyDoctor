package publicip

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

// PublicIPCheck detects the public IP address.
type PublicIPCheck struct{}

// NewPublicIPCheck creates a new public IP check.
func NewPublicIPCheck() check.Checker {
	return &PublicIPCheck{}
}

func (c *PublicIPCheck) ID() string { return "public_ip" }

func (c *PublicIPCheck) Name() string { return "Public IP Detection" }

func (c *PublicIPCheck) Description() string {
	return "Detects your public IP address via the current connection"
}

func (c *PublicIPCheck) Category() check.CheckCategory { return check.CategoryNetwork }

func (c *PublicIPCheck) DependsOn() []string { return []string{} }

func (c *PublicIPCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

	services := []IPService{
		{Name: "ipify.org", URL: "https://api.ipify.org?format=json"},
		{Name: "icanhazip.com", URL: "https://icanhazip.com/"},
		{Name: "ifconfig.me", URL: "https://ifconfig.me/ip"},
	}

	adapter := ctx.GetProxyAdapter()
	if ctx.GetProxyConfig().Type == check.ProxyTypeDirect {
		adapter = ctx.GetDirectAdapter()
	}

	var publicIP string
	var detectionService string
	serviceErrors := make(map[string]string)

	for _, service := range services {
		ip, err := detectIPFromService(adapter, service)
		if err == nil {
			publicIP = ip
			detectionService = service.Name
			break
		}
		serviceErrors[service.Name] = err.Error()
	}

	result.SetExecutionTime(time.Since(startTime))

	if publicIP == "" {
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation("Unable to detect public IP address from any configured service").
			WithConfidence(0).
			AddEvidence("service_errors", serviceErrors).
			AddProbableCause("Network connectivity issues").
			AddProbableCause("All IP detection services are unreachable or returned invalid data")
	}

	ctx.SetSharedData("public_ip", publicIP)

	return *result.WithStatus(check.StatusPassed, check.SeverityInfo).
		WithExplanation(fmt.Sprintf("Public IP detected: %s via %s", publicIP, detectionService)).
		WithConfidence(0.95).
		AddEvidence("ip_address", publicIP).
		AddEvidence("detection_service", detectionService)
}

// IPService represents an IP detection service.
type IPService struct {
	Name string
	URL  string
}

func detectIPFromService(adapter check.NetworkAdapter, service IPService) (string, error) {
	req := &check.HTTPRequest{
		Method: "GET",
		URL:    service.URL,
		Headers: map[string]string{
			"User-Agent": "ProxyDoctor/0.1",
			"Accept":     "application/json, text/plain;q=0.9, */*;q=0.1",
		},
	}

	resp, err := adapter.ExecuteHTTPRequest(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non-200 response: %d", resp.StatusCode)
	}

	var jsonResp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(resp.Body, &jsonResp); err == nil && strings.TrimSpace(jsonResp.IP) != "" {
		return parseIP(jsonResp.IP)
	}

	return parseIP(string(resp.Body))
}

func parseIP(s string) (string, error) {
	candidate := strings.TrimSpace(s)
	if candidate == "" {
		return "", fmt.Errorf("empty IP response")
	}
	addr, err := netip.ParseAddr(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid IP response %q: %w", candidate, err)
	}
	return addr.String(), nil
}
