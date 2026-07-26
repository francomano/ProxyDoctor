package adapters

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"encoding/base64"
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

const maxHTTPResponseBodyBytes int64 = 1 << 20

func readLimitedBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBodyBytes))
}

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
}

func parsePublicIPResponse(body []byte) (string, error) {
	var jsonResp struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &jsonResp); err == nil && strings.TrimSpace(jsonResp.IP) != "" {
		return normalizeIP(jsonResp.IP)
	}
	return normalizeIP(string(body))
}

func normalizeIP(raw string) (string, error) {
	ip := strings.TrimSpace(raw)
	if ip == "" {
		return "", fmt.Errorf("empty IP response")
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %q: %w", ip, err)
	}
	return addr.String(), nil
}

func publicIPViaHTTPRequest(adapter check.NetworkAdapter) (string, error) {
	services := []string{
		"https://api.ipify.org?format=json",
		"https://icanhazip.com/",
		"https://ifconfig.me/ip",
	}

	var lastErr error
	for _, serviceURL := range services {
		resp, err := adapter.ExecuteHTTPRequest(&check.HTTPRequest{
			Method: "GET",
			URL:    serviceURL,
			Headers: map[string]string{
				"User-Agent": "ProxyDoctor/0.1",
				"Accept":     "application/json, text/plain;q=0.9, */*;q=0.1",
			},
		})
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s returned HTTP %d", serviceURL, resp.StatusCode)
			continue
		}
		ip, err := parsePublicIPResponse(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}
		return ip, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("public IP detection failed: %w", lastErr)
	}
	return "", fmt.Errorf("public IP detection failed")
}

func formatTLSVersion(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown TLS version 0x%04x", version)
	}
}

func certificateInfoFromState(state tls.ConnectionState) (*check.CertificateInfo, error) {
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no certificates received")
	}
	cert := state.PeerCertificates[0]
	return &check.CertificateInfo{
		Subject:            cert.Subject.String(),
		Issuer:             cert.Issuer.String(),
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		SANs:               cert.DNSNames,
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		PublicKeyType:      fmt.Sprintf("%T", cert.PublicKey),
		PublicKeyBits:      publicKeyBits(cert.PublicKey),
		IsValid:            time.Now().After(cert.NotBefore) && time.Now().Before(cert.NotAfter),
	}, nil
}

func publicKeyBits(key interface{}) int {
	switch k := key.(type) {
	case *rsa.PublicKey:
		return k.N.BitLen()
	case *ecdsa.PublicKey:
		return k.Curve.Params().BitSize
	case ed25519.PublicKey:
		return len(k) * 8
	default:
		return 0
	}
}

func connectViaHTTPProxy(config check.ProxyConfig, target string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr(config), timeout)
	if err != nil {
		return nil, fmt.Errorf("could not connect to proxy: %w", err)
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Scheme: "http", Host: target},
		Host:   target,
		Header: make(http.Header),
	}
	if config.Username != "" {
		token := base64.StdEncoding.EncodeToString([]byte(config.Username + ":" + config.Password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("could not send CONNECT request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("could not read CONNECT response: %w", err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT returned HTTP %d", resp.StatusCode)
	}

	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func testPortViaHTTPProxy(config check.ProxyConfig, host string, port int, timeout time.Duration) (bool, error) {
	conn, err := connectViaHTTPProxy(config, net.JoinHostPort(host, fmt.Sprint(port)), timeout)
	if err != nil {
		return false, err
	}
	_ = conn.Close()
	return true, nil
}

func dialTLSThroughHTTPProxy(config check.ProxyConfig, hostname string, timeout time.Duration) (*tls.Conn, error) {
	conn, err := connectViaHTTPProxy(config, net.JoinHostPort(hostname, "443"), timeout)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(conn, &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12})
	if err := tlsConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}
	_ = tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}
