package adapters

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/francomano/proxydoctor/core/check"
)

// proxyAddr formats a ProxyConfig into host:port
func proxyAddr(c check.ProxyConfig) string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// SOCKS4Adapter handles SOCKS4 proxy connections
type SOCKS4Adapter struct {
	config check.ProxyConfig
	client *http.Client
}

// NewSOCKS4Adapter creates a new SOCKS4 adapter
func NewSOCKS4Adapter(config check.ProxyConfig) check.NetworkAdapter {
	d := &socks4Dialer{
		proxyAddr: proxyAddr(config),
		userID:    config.Username,
		timeout:   30 * time.Second,
	}

	transport := &http.Transport{
		DialContext:           d.DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &SOCKS4Adapter{
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

func (a *SOCKS4Adapter) Type() check.ProxyType {
	return check.ProxyTypeSOCKS4
}

func (a *SOCKS4Adapter) ExecuteHTTPRequest(req *check.HTTPRequest) (*check.HTTPResponse, error) {
	httpReq, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	startTime := time.Now()
	httpResp, err := a.client.Do(httpReq)
	duration := time.Since(startTime)

	if err != nil {
		return nil, fmt.Errorf("HTTP request via SOCKS4 proxy failed: %w", err)
	}
	defer httpResp.Body.Close()

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

func (a *SOCKS4Adapter) FollowRedirects(targetURL string, maxRedirects int) ([]check.RedirectStep, error) {
	redirects := []check.RedirectStep{}
	currentURL := targetURL

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				break
			}

			redirects = append(redirects, check.RedirectStep{
				From:       currentURL,
				To:         location,
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
			})

			currentURL = location
		} else {
			break
		}
	}

	return redirects, nil
}

func (a *SOCKS4Adapter) ResolveDNS(hostname string) ([]string, error) {
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

func (a *SOCKS4Adapter) GetPublicIP() (string, error) {
	req := &check.HTTPRequest{
		Method: "GET",
		URL:    "https://api.ipify.org?format=json",
		Headers: map[string]string{
			"User-Agent": "ProxyDoctor/0.1",
		},
	}

	resp, err := a.ExecuteHTTPRequest(req)
	if err != nil {
		return "", fmt.Errorf("public IP detection via SOCKS4 failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non-200 response from IP service: %d", resp.StatusCode)
	}

	var jsonResp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(resp.Body, &jsonResp); err == nil && jsonResp.IP != "" {
		return jsonResp.IP, nil
	}

	ip := strings.TrimSpace(string(resp.Body))
	if ip != "" {
		return ip, nil
	}

	return "", fmt.Errorf("could not parse public IP response")
}

func (a *SOCKS4Adapter) TestPort(host string, port int, timeout time.Duration) (bool, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr(a.config), timeout)
	if err != nil {
		return false, fmt.Errorf("could not connect to SOCKS4 proxy: %w", err)
	}
	defer conn.Close()

	if err := socks4Connect(conn, host, port, a.config.Username, timeout); err != nil {
		return false, err
	}

	return true, nil
}

func (a *SOCKS4Adapter) GetTLSCertificate(hostname string) (*check.CertificateInfo, error) {
	conn, err := dialTLSThroughSOCKS4(proxyAddr(a.config), hostname, a.config.Username)
	if err != nil {
		return nil, fmt.Errorf("TLS handshake via SOCKS4 failed: %w", err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates received")
	}

	cert := certs[0]
	return &check.CertificateInfo{
		Subject:            cert.Subject.String(),
		Issuer:             cert.Issuer.String(),
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		SANs:               cert.DNSNames,
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		PublicKeyType:      fmt.Sprintf("%T", cert.PublicKey),
		PublicKeyBits:      publicKeyBits(cert.PublicKey),
		IsValid:            time.Now().Before(cert.NotAfter) && time.Now().After(cert.NotBefore),
	}, nil
}

func (a *SOCKS4Adapter) GetTLSCipherSuite(hostname string) (string, error) {
	conn, err := dialTLSThroughSOCKS4(proxyAddr(a.config), hostname, a.config.Username)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via SOCKS4 failed: %w", err)
	}
	defer conn.Close()

	return tls.CipherSuiteName(conn.ConnectionState().CipherSuite), nil
}

func (a *SOCKS4Adapter) GetTLSVersion(hostname string) (string, error) {
	conn, err := dialTLSThroughSOCKS4(proxyAddr(a.config), hostname, a.config.Username)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via SOCKS4 failed: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	return formatTLSVersion(state.Version), nil
}

// --- SOCKS5 Adapter ---

// SOCKS5Adapter handles SOCKS5 proxy connections
type SOCKS5Adapter struct {
	config check.ProxyConfig
	client *http.Client
}

// NewSOCKS5Adapter creates a new SOCKS5 adapter
func NewSOCKS5Adapter(config check.ProxyConfig) check.NetworkAdapter {
	pAddr := proxyAddr(config)

	proxyURL := &url.URL{
		Scheme: "socks5",
		Host:   pAddr,
	}

	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		// Fallback to manual dialer
		var auth *proxy.Auth
		if config.Username != "" {
			auth = &proxy.Auth{
				User:     config.Username,
				Password: config.Password,
			}
		}
		d := &socks5ManualDialer{
			proxyAddr: pAddr,
			auth:      auth,
			timeout:   30 * time.Second,
		}
		return &SOCKS5Adapter{
			config: config,
			client: &http.Client{
				Transport: &http.Transport{
					DialContext:           d.DialContext,
					ResponseHeaderTimeout: 30 * time.Second,
				},
				Timeout: 30 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		}
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.(proxy.ContextDialer).DialContext(ctx, network, addr)
		},
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &SOCKS5Adapter{
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

func (a *SOCKS5Adapter) Type() check.ProxyType {
	return check.ProxyTypeSOCKS5
}

func (a *SOCKS5Adapter) ExecuteHTTPRequest(req *check.HTTPRequest) (*check.HTTPResponse, error) {
	httpReq, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	startTime := time.Now()
	httpResp, err := a.client.Do(httpReq)
	duration := time.Since(startTime)

	if err != nil {
		return nil, fmt.Errorf("HTTP request via SOCKS5 proxy failed: %w", err)
	}
	defer httpResp.Body.Close()

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

func (a *SOCKS5Adapter) FollowRedirects(targetURL string, maxRedirects int) ([]check.RedirectStep, error) {
	redirects := []check.RedirectStep{}
	currentURL := targetURL

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				break
			}

			redirects = append(redirects, check.RedirectStep{
				From:       currentURL,
				To:         location,
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
			})

			currentURL = location
		} else {
			break
		}
	}

	return redirects, nil
}

func (a *SOCKS5Adapter) ResolveDNS(hostname string) ([]string, error) {
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

func (a *SOCKS5Adapter) GetPublicIP() (string, error) {
	req := &check.HTTPRequest{
		Method: "GET",
		URL:    "https://api.ipify.org?format=json",
		Headers: map[string]string{
			"User-Agent": "ProxyDoctor/0.1",
		},
	}

	resp, err := a.ExecuteHTTPRequest(req)
	if err != nil {
		return "", fmt.Errorf("public IP detection via SOCKS5 failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non-200 response from IP service: %d", resp.StatusCode)
	}

	var jsonResp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(resp.Body, &jsonResp); err == nil && jsonResp.IP != "" {
		return jsonResp.IP, nil
	}

	ip := strings.TrimSpace(string(resp.Body))
	if ip != "" {
		return ip, nil
	}

	return "", fmt.Errorf("could not parse public IP response")
}

func (a *SOCKS5Adapter) TestPort(host string, port int, timeout time.Duration) (bool, error) {
	var auth *proxy.Auth
	if a.config.Username != "" {
		auth = &proxy.Auth{
			User:     a.config.Username,
			Password: a.config.Password,
		}
	}

	d := &socks5ManualDialer{
		proxyAddr: proxyAddr(a.config),
		auth:      auth,
		timeout:   timeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}

func (a *SOCKS5Adapter) GetTLSCertificate(hostname string) (*check.CertificateInfo, error) {
	conn, err := dialTLSThroughSOCKS5(proxyAddr(a.config), hostname, a.config.Username, a.config.Password)
	if err != nil {
		return nil, fmt.Errorf("TLS handshake via SOCKS5 failed: %w", err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates received")
	}

	cert := certs[0]
	return &check.CertificateInfo{
		Subject:            cert.Subject.String(),
		Issuer:             cert.Issuer.String(),
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		SANs:               cert.DNSNames,
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		PublicKeyType:      fmt.Sprintf("%T", cert.PublicKey),
		PublicKeyBits:      publicKeyBits(cert.PublicKey),
		IsValid:            time.Now().Before(cert.NotAfter) && time.Now().After(cert.NotBefore),
	}, nil
}

func (a *SOCKS5Adapter) GetTLSCipherSuite(hostname string) (string, error) {
	conn, err := dialTLSThroughSOCKS5(proxyAddr(a.config), hostname, a.config.Username, a.config.Password)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via SOCKS5 failed: %w", err)
	}
	defer conn.Close()

	return tls.CipherSuiteName(conn.ConnectionState().CipherSuite), nil
}

func (a *SOCKS5Adapter) GetTLSVersion(hostname string) (string, error) {
	conn, err := dialTLSThroughSOCKS5(proxyAddr(a.config), hostname, a.config.Username, a.config.Password)
	if err != nil {
		return "", fmt.Errorf("TLS handshake via SOCKS5 failed: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	return formatTLSVersion(state.Version), nil
}

// --- SOCKS4 Protocol Helpers ---

type socks4Dialer struct {
	proxyAddr string
	userID    string
	timeout   time.Duration
}

func (d *socks4Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", addr, err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return nil, fmt.Errorf("invalid port %s: %w", portStr, err)
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("could not connect to SOCKS4 proxy: %w", err)
	}

	if err := socks4Connect(conn, host, port, d.userID, d.timeout); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func socks4Connect(conn net.Conn, host string, port int, userID string, timeout time.Duration) error {
	conn.SetDeadline(time.Now().Add(timeout))

	buf := make([]byte, 0, 16)
	buf = append(buf, 0x04) // VER
	buf = append(buf, 0x01) // CD: CONNECT

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	buf = append(buf, portBytes...)

	ip := net.ParseIP(host)
	if ip != nil {
		ipv4 := ip.To4()
		if ipv4 != nil {
			buf = append(buf, ipv4...)
		} else {
			buf = append(buf, 0, 0, 0, 1)
			buf = append(buf, []byte(host)...)
			buf = append(buf, 0)
		}
	} else {
		// SOCKS4a: use 0.0.0.1 as placeholder IP
		buf = append(buf, 0, 0, 0, 1)
		buf = append(buf, []byte(host)...)
		buf = append(buf, 0)
	}

	if userID != "" {
		buf = append(buf, []byte(userID)...)
	}
	buf = append(buf, 0)

	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("SOCKS4 request write failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp := make([]byte, 8)
	if _, err := io.ReadFull(reader, resp); err != nil {
		return fmt.Errorf("SOCKS4 response read failed: %w", err)
	}

	if resp[0] != 0x00 {
		return fmt.Errorf("SOCKS4 invalid response version: %d", resp[0])
	}

	if resp[1] != 0x5A {
		var reason string
		switch resp[1] {
		case 0x5B:
			reason = "request rejected or failed"
		case 0x5C:
			reason = "client cannot connect to identd"
		case 0x5D:
			reason = "client's identd did not confirm the user ID"
		default:
			reason = fmt.Sprintf("unknown error code 0x%02X", resp[1])
		}
		return fmt.Errorf("SOCKS4 connection rejected: %s", reason)
	}

	return nil
}

func dialTLSThroughSOCKS4(proxyAddress, hostname, userID string) (*tls.Conn, error) {
	d := &socks4Dialer{
		proxyAddr: proxyAddress,
		userID:    userID,
		timeout:   10 * time.Second,
	}

	rawConn, err := d.DialContext(context.Background(), "tcp", hostname+":443")
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify: true,
	})

	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// --- SOCKS5 Protocol Helpers ---

type socks5ManualDialer struct {
	proxyAddr string
	auth      *proxy.Auth
	timeout   time.Duration
}

func (d *socks5ManualDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("could not connect to SOCKS5 proxy: %w", err)
	}

	if err := socks5Handshake(conn, d.auth, d.timeout); err != nil {
		conn.Close()
		return nil, err
	}

	if err := socks5Connect(conn, addr, d.timeout); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func socks5Handshake(conn net.Conn, auth *proxy.Auth, timeout time.Duration) error {
	conn.SetDeadline(time.Now().Add(timeout))

	buf := []byte{0x05, 0x01}
	if auth != nil {
		buf[1] = 0x02
		buf = append(buf, 0x00, 0x02)
	} else {
		buf = append(buf, 0x00)
	}

	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("SOCKS5 greeting write failed: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5 greeting response read failed: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5 invalid version: %d", resp[0])
	}

	if resp[1] == 0x02 {
		if auth == nil {
			return fmt.Errorf("SOCKS5 proxy requires authentication but none provided")
		}
		return socks5Auth(conn, auth)
	} else if resp[1] == 0xFF {
		return fmt.Errorf("SOCKS5 no acceptable authentication method")
	}

	return nil
}

func socks5Auth(conn net.Conn, auth *proxy.Auth) error {
	buf := []byte{0x01}
	buf = append(buf, byte(len(auth.User)))
	buf = append(buf, []byte(auth.User)...)
	buf = append(buf, byte(len(auth.Password)))
	buf = append(buf, []byte(auth.Password)...)

	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("SOCKS5 auth write failed: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5 auth response read failed: %w", err)
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5 authentication failed")
	}

	return nil
}

func socks5Connect(conn net.Conn, addr string, timeout time.Duration) error {
	conn.SetDeadline(time.Now().Add(timeout))

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", addr, err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("invalid port %s: %w", portStr, err)
	}

	buf := []byte{0x05, 0x01, 0x00}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		buf = append(buf, 0x01)
		buf = append(buf, ip4...)
	} else if ip6 := ip.To16(); ip6 != nil {
		buf = append(buf, 0x04)
		buf = append(buf, ip6...)
	} else {
		buf = append(buf, 0x03)
		buf = append(buf, byte(len(host)))
		buf = append(buf, []byte(host)...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	buf = append(buf, portBytes...)

	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("SOCKS5 connect write failed: %w", err)
	}

	resp := make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5 connect response read failed: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5 invalid version: %d", resp[0])
	}

	if resp[1] != 0x00 {
		var reason string
		switch resp[1] {
		case 0x01:
			reason = "general SOCKS server failure"
		case 0x02:
			reason = "connection not allowed by ruleset"
		case 0x03:
			reason = "network unreachable"
		case 0x04:
			reason = "host unreachable"
		case 0x05:
			reason = "connection refused"
		case 0x06:
			reason = "TTL expired"
		case 0x07:
			reason = "command not supported"
		case 0x08:
			reason = "address type not supported"
		default:
			reason = fmt.Sprintf("unknown error code 0x%02X", resp[1])
		}
		return fmt.Errorf("SOCKS5 connect failed: %s", reason)
	}

	switch resp[3] {
	case 0x01:
		remaining := make([]byte, 4+2)
		if _, err := io.ReadFull(conn, remaining); err != nil {
			return fmt.Errorf("SOCKS5 connect response read failed: %w", err)
		}
	case 0x04:
		remaining := make([]byte, 16+2)
		if _, err := io.ReadFull(conn, remaining); err != nil {
			return fmt.Errorf("SOCKS5 connect response read failed: %w", err)
		}
	case 0x03:
		domainLen := make([]byte, 1)
		if _, err := io.ReadFull(conn, domainLen); err != nil {
			return fmt.Errorf("SOCKS5 connect response read failed: %w", err)
		}
		remaining := make([]byte, int(domainLen[0])+2)
		if _, err := io.ReadFull(conn, remaining); err != nil {
			return fmt.Errorf("SOCKS5 connect response read failed: %w", err)
		}
	}

	return nil
}

func dialTLSThroughSOCKS5(proxyAddress, hostname, username, password string) (*tls.Conn, error) {
	var auth *proxy.Auth
	if username != "" {
		auth = &proxy.Auth{
			User:     username,
			Password: password,
		}
	}

	d := &socks5ManualDialer{
		proxyAddr: proxyAddress,
		auth:      auth,
		timeout:   10 * time.Second,
	}

	rawConn, err := d.DialContext(context.Background(), "tcp", hostname+":443")
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify: true,
	})

	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return nil, err
	}

	return tlsConn, nil
}
