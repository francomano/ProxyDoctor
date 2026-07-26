package adapters

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/francomano/proxydoctor/core/check"
)

// HTTPProxyAdapter handles HTTP proxy connections.
type HTTPProxyAdapter struct {
	config check.ProxyConfig
	client *http.Client
}

// NewHTTPProxyAdapter creates a new HTTP proxy adapter.
func NewHTTPProxyAdapter(config check.ProxyConfig) check.NetworkAdapter {
	parsedURL := &url.URL{Scheme: "http", Host: proxyAddr(config)}
	if config.Username != "" {
		parsedURL.User = url.UserPassword(config.Username, config.Password)
	}

	transport := &http.Transport{Proxy: http.ProxyURL(parsedURL)}
	return &HTTPProxyAdapter{
		config: config,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (a *HTTPProxyAdapter) Type() check.ProxyType { return check.ProxyTypeHTTP }

func (a *HTTPProxyAdapter) ExecuteHTTPRequest(req *check.HTTPRequest) (*check.HTTPResponse, error) {
	return executeViaClient(a.client, req, "HTTP request via proxy failed")
}

func (a *HTTPProxyAdapter) FollowRedirects(targetURL string, maxRedirects int) ([]check.RedirectStep, error) {
	return followRedirectsViaClient(a.client, targetURL, maxRedirects)
}

func (a *HTTPProxyAdapter) ResolveDNS(hostname string) ([]string, error) {
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed: %w", err)
	}
	result := make([]string, len(ips))
	for i, ip := range ips {
		result[i] = ip.String()
	}
	return result, nil
}

func (a *HTTPProxyAdapter) GetPublicIP() (string, error) {
	return publicIPViaHTTPRequest(a)
}

func (a *HTTPProxyAdapter) TestPort(host string, port int, timeout time.Duration) (bool, error) {
	return testPortViaHTTPProxy(a.config, host, port, timeout)
}

func (a *HTTPProxyAdapter) GetTLSCertificate(hostname string) (*check.CertificateInfo, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TLS handshake via HTTP proxy failed: %w", err)
	}
	defer conn.Close()
	return certificateInfoFromState(conn.ConnectionState())
}

func (a *HTTPProxyAdapter) GetTLSCipherSuite(hostname string) (string, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via HTTP proxy failed: %w", err)
	}
	defer conn.Close()
	return tls.CipherSuiteName(conn.ConnectionState().CipherSuite), nil
}

func (a *HTTPProxyAdapter) GetTLSVersion(hostname string) (string, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via HTTP proxy failed: %w", err)
	}
	defer conn.Close()
	return formatTLSVersion(conn.ConnectionState().Version), nil
}

// HTTPSProxyAdapter handles HTTPS proxy connections.
type HTTPSProxyAdapter struct {
	config check.ProxyConfig
	client *http.Client
}

// NewHTTPSProxyAdapter creates a new HTTPS proxy adapter.
func NewHTTPSProxyAdapter(config check.ProxyConfig) check.NetworkAdapter {
	parsedURL := &url.URL{Scheme: "https", Host: proxyAddr(config)}
	if config.Username != "" {
		parsedURL.User = url.UserPassword(config.Username, config.Password)
	}

	transport := &http.Transport{Proxy: http.ProxyURL(parsedURL)}
	return &HTTPSProxyAdapter{
		config: config,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (a *HTTPSProxyAdapter) Type() check.ProxyType { return check.ProxyTypeHTTPS }

func (a *HTTPSProxyAdapter) ExecuteHTTPRequest(req *check.HTTPRequest) (*check.HTTPResponse, error) {
	return executeViaClient(a.client, req, "HTTP request via HTTPS proxy failed")
}

func (a *HTTPSProxyAdapter) FollowRedirects(targetURL string, maxRedirects int) ([]check.RedirectStep, error) {
	return followRedirectsViaClient(a.client, targetURL, maxRedirects)
}

func (a *HTTPSProxyAdapter) ResolveDNS(hostname string) ([]string, error) {
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed: %w", err)
	}
	result := make([]string, len(ips))
	for i, ip := range ips {
		result[i] = ip.String()
	}
	return result, nil
}

func (a *HTTPSProxyAdapter) GetPublicIP() (string, error) {
	return publicIPViaHTTPRequest(a)
}

func (a *HTTPSProxyAdapter) TestPort(host string, port int, timeout time.Duration) (bool, error) {
	return testPortViaHTTPProxy(a.config, host, port, timeout)
}

func (a *HTTPSProxyAdapter) GetTLSCertificate(hostname string) (*check.CertificateInfo, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TLS handshake via HTTPS proxy failed: %w", err)
	}
	defer conn.Close()
	return certificateInfoFromState(conn.ConnectionState())
}

func (a *HTTPSProxyAdapter) GetTLSCipherSuite(hostname string) (string, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via HTTPS proxy failed: %w", err)
	}
	defer conn.Close()
	return tls.CipherSuiteName(conn.ConnectionState().CipherSuite), nil
}

func (a *HTTPSProxyAdapter) GetTLSVersion(hostname string) (string, error) {
	conn, err := dialTLSThroughHTTPProxy(a.config, hostname, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via HTTPS proxy failed: %w", err)
	}
	defer conn.Close()
	return formatTLSVersion(conn.ConnectionState().Version), nil
}

func executeViaClient(client *http.Client, req *check.HTTPRequest, errorPrefix string) (*check.HTTPResponse, error) {
	httpReq, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	startTime := time.Now()
	httpResp, err := client.Do(httpReq)
	duration := time.Since(startTime)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorPrefix, err)
	}
	defer closeResponseBody(httpResp)

	body, err := readLimitedBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &check.HTTPResponse{
		StatusCode: httpResp.StatusCode,
		Headers:    httpResp.Header,
		Body:       body,
		Duration:   duration,
	}, nil
}

func followRedirectsViaClient(client *http.Client, targetURL string, maxRedirects int) ([]check.RedirectStep, error) {
	redirects := []check.RedirectStep{}
	currentURL := targetURL

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			closeResponseBody(resp)
			break
		}

		location := resp.Header.Get("Location")
		if location == "" {
			closeResponseBody(resp)
			break
		}
		nextURL, err := req.URL.Parse(location)
		if err != nil {
			closeResponseBody(resp)
			return nil, err
		}

		redirects = append(redirects, check.RedirectStep{
			From:       currentURL,
			To:         nextURL.String(),
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
		})
		closeResponseBody(resp)
		currentURL = nextURL.String()
	}
	return redirects, nil
}
