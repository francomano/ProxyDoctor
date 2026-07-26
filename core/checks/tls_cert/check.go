package tlscert

import (
	"fmt"
	"net/url"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

type TLSCertCheck struct{}

func NewTLSCertCheck() check.Checker { return &TLSCertCheck{} }

func (c *TLSCertCheck) ID() string   { return "tls_certificate" }
func (c *TLSCertCheck) Name() string { return "TLS Certificate" }
func (c *TLSCertCheck) Description() string {
	return "Checks the TLS certificate of the target (validity, issuer, expiry, cipher suite, TLS version)"
}
func (c *TLSCertCheck) Category() check.CheckCategory { return check.CategoryTLS }
func (c *TLSCertCheck) DependsOn() []string           { return []string{} }

func (c *TLSCertCheck) Execute(ctx check.ExecutionContext) check.CheckResult {
	result := check.NewCheckResult(c.ID(), c.Category())
	startTime := time.Now()

	targetURL := ctx.GetURL()
	parsed, err := url.Parse(targetURL)
	if err != nil {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("Invalid URL: %v", err)).
			WithConfidence(0)
	}
	if parsed.Scheme != "https" {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusSkipped, check.SeverityInfo).
			WithExplanation(fmt.Sprintf("Skipping TLS check for non-HTTPS URL: %s", targetURL)).
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

	certInfo, err := adapter.GetTLSCertificate(hostname)
	if err != nil {
		result.SetExecutionTime(time.Since(startTime))
		return *result.WithStatus(check.StatusError, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("TLS certificate check failed for %s: %v", hostname, err)).
			WithConfidence(0).
			AddProbableCause("TLS handshake failed").
			AddSuggestedAction("Verify that the target supports HTTPS and the proxy allows TLS traffic")
	}

	cipherSuite, cipherErr := adapter.GetTLSCipherSuite(hostname)
	tlsVersion, versionErr := adapter.GetTLSVersion(hostname)

	result.SetExecutionTime(time.Since(startTime))
	result.AddEvidence("hostname", hostname).
		AddEvidence("subject", certInfo.Subject).
		AddEvidence("issuer", certInfo.Issuer).
		AddEvidence("not_before", certInfo.NotBefore).
		AddEvidence("not_after", certInfo.NotAfter).
		AddEvidence("sans", certInfo.SANs).
		AddEvidence("signature_algorithm", certInfo.SignatureAlgorithm).
		AddEvidence("public_key_type", certInfo.PublicKeyType).
		AddEvidence("public_key_bits", certInfo.PublicKeyBits).
		AddEvidence("is_valid", certInfo.IsValid)
	if cipherErr == nil {
		result.AddEvidence("cipher_suite", cipherSuite)
	} else {
		result.AddEvidence("cipher_suite_error", cipherErr.Error())
	}
	if versionErr == nil {
		result.AddEvidence("tls_version", tlsVersion)
	} else {
		result.AddEvidence("tls_version_error", versionErr.Error())
	}

	now := time.Now()
	if !certInfo.IsValid {
		return *result.WithStatus(check.StatusFailed, check.SeverityCritical).
			WithExplanation(fmt.Sprintf("TLS certificate for %s is not currently valid", hostname)).
			WithConfidence(0.95).
			AddProbableCause("Certificate is expired or not yet valid").
			AddSuggestedAction("Renew or replace the certificate")
	}
	if certInfo.NotAfter.Before(now.Add(30 * 24 * time.Hour)) {
		return *result.WithStatus(check.StatusFailed, check.SeverityWarning).
			WithExplanation(fmt.Sprintf("TLS certificate for %s expires soon: %s", hostname, certInfo.NotAfter.Format(time.RFC3339))).
			WithConfidence(0.9).
			AddSuggestedAction("Schedule certificate renewal before expiration")
	}

	return *result.WithStatus(check.StatusPassed, check.SeverityInfo).
		WithExplanation(fmt.Sprintf("TLS certificate for %s is valid until %s", hostname, certInfo.NotAfter.Format(time.RFC3339))).
		WithConfidence(0.95)
}
