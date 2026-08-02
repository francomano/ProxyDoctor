package adapters

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"

	"github.com/francomano/proxydoctor/core/check"
)

// DialContextFunc dials a network address, possibly through a proxy.
// It is the common building block used by adapters and by plugins that
// want to route arbitrary traffic (browsing, downloads) through a proxy.
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// NewProxyDialContext returns a DialContextFunc that routes connections
// through the given proxy configuration. For direct connections it returns
// the standard net.Dialer dial function.
func NewProxyDialContext(config check.ProxyConfig) (DialContextFunc, error) {
	switch config.Type {
	case check.ProxyTypeDirect:
		var d net.Dialer
		return d.DialContext, nil
	case check.ProxyTypeSOCKS4:
		d := &socks4Dialer{
			proxyAddr: proxyAddr(config),
			userID:    config.Username,
			timeout:   30 * time.Second,
		}
		return d.DialContext, nil
	case check.ProxyTypeSOCKS5:
		var auth *proxy.Auth
		if config.Username != "" {
			auth = &proxy.Auth{
				User:     config.Username,
				Password: config.Password,
			}
		}
		d := &socks5ManualDialer{
			proxyAddr: proxyAddr(config),
			auth:      auth,
			timeout:   30 * time.Second,
		}
		return d.DialContext, nil
	case check.ProxyTypeHTTP, check.ProxyTypeHTTPS:
		return newHTTPConnectDialer(config), nil
	default:
		var d net.Dialer
		return d.DialContext, nil
	}
}

// newHTTPConnectDialer dials a target by issuing a CONNECT request through an
// HTTP or HTTPS (TLS to the proxy) upstream proxy.
func newHTTPConnectDialer(config check.ProxyConfig) DialContextFunc {
	proxyHost := proxyAddr(config)
	secure := config.Type == check.ProxyTypeHTTPS

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, network, proxyHost)
		if err != nil {
			return nil, fmt.Errorf("could not connect to proxy %s: %w", proxyHost, err)
		}

		if secure {
			tlsConn := tls.Client(conn, &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("TLS handshake with proxy failed: %w", err)
			}
			conn = tlsConn
		}

		req := &http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Scheme: "http", Host: addr},
			Host:   addr,
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
		// Do NOT drain the body: for a 2xx CONNECT response Go exposes the tunnel
		// itself as the body, so reading it would consume tunnel bytes or block.
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = conn.Close()
			return nil, fmt.Errorf("proxy CONNECT returned HTTP %d", resp.StatusCode)
		}

		return conn, nil
	}
}

// ForwardTransport returns an http.Transport that routes requests through the
// given proxy configuration. It is the transport a local forward proxy uses to
// replay incoming HTTP requests upstream.
func ForwardTransport(config check.ProxyConfig) (*http.Transport, error) {
	switch config.Type {
	case check.ProxyTypeSOCKS4, check.ProxyTypeSOCKS5:
		dial, err := NewProxyDialContext(config)
		if err != nil {
			return nil, err
		}
		return &http.Transport{
			DialContext:           dial,
			ResponseHeaderTimeout: 30 * time.Second,
		}, nil
	case check.ProxyTypeHTTP, check.ProxyTypeHTTPS:
		proxyURL := &url.URL{
			Scheme: string(config.Type),
			Host:   proxyAddr(config),
		}
		if config.Username != "" {
			proxyURL.User = url.UserPassword(config.Username, config.Password)
		}
		return &http.Transport{
			Proxy:                 http.ProxyURL(proxyURL),
			ResponseHeaderTimeout: 30 * time.Second,
		}, nil
	default:
		return &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
		}, nil
	}
}
