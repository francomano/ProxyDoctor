package adapters

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/francomano/proxydoctor/core/check"
	"github.com/francomano/proxydoctor/internal/testproxy"
)

func httpAdapter(addr string) check.NetworkAdapter {
	host, port := testproxy.HostPort(addr)
	return NewHTTPProxyAdapter(check.ProxyConfig{Type: check.ProxyTypeHTTP, Host: host, Port: port})
}

func socks4Adapter(addr string) check.NetworkAdapter {
	host, port := testproxy.HostPort(addr)
	return NewSOCKS4Adapter(check.ProxyConfig{Type: check.ProxyTypeSOCKS4, Host: host, Port: port})
}

func socks5Adapter(addr string, username, password string) check.NetworkAdapter {
	host, port := testproxy.HostPort(addr)
	return NewSOCKS5Adapter(check.ProxyConfig{
		Type: check.ProxyTypeSOCKS5, Host: host, Port: port,
		Username: username, Password: password,
	})
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port := testproxy.HostPort(ln.Addr().String())
	_ = ln.Close()
	return port
}

func wantOriginBody(t *testing.T, resp *check.HTTPResponse) {
	t.Helper()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "origin:") {
		t.Fatalf("unexpected body: %q", string(resp.Body))
	}
}

func TestAdapterDirect_HTTPRequest(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	resp, err := NewDirectAdapter().ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET",
		URL:    origin.HTTPURL + "/probe",
		Headers: map[string]string{"User-Agent": "proxydoctor-test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)
}

func TestAdapterHTTPProxy_HTTPRequest(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	proxy := testproxy.StartHTTPProxy(t, "", "")

	resp, err := httpAdapter(proxy.Addr).ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET",
		URL:    origin.HTTPURL + "/probe",
		Headers: map[string]string{"User-Agent": "proxydoctor-test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)
}

func TestAdapterHTTPProxy_TestPort(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartHTTPProxy(t, "", "")
	host, port := testproxy.HostPort(origin.Addr)

	ok, err := httpAdapter(proxy.Addr).TestPort(host, port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected port to be reachable via CONNECT")
	}
}

func TestAdapterHTTPProxy_TestPortRefused(t *testing.T) {
	proxy := testproxy.StartHTTPProxy(t, "", "")
	port := closedPort(t)

	ok, err := httpAdapter(proxy.Addr).TestPort("127.0.0.1", port, 5*time.Second)
	if err == nil {
		t.Fatalf("expected error for refused CONNECT, got ok=%v", ok)
	}
}

func TestAdapterHTTPProxy_Auth(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	proxy := testproxy.StartHTTPProxy(t, "proxyuser", "secret")
	host, port := testproxy.HostPort(proxy.Addr)

	good := NewHTTPProxyAdapter(check.ProxyConfig{
		Type: check.ProxyTypeHTTP, Host: host, Port: port,
		Username: "proxyuser", Password: "secret",
	})
	resp, err := good.ExecuteHTTPRequest(&check.HTTPRequest{Method: "GET", URL: origin.HTTPURL})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)

	bad := NewHTTPProxyAdapter(check.ProxyConfig{
		Type: check.ProxyTypeHTTP, Host: host, Port: port,
		Username: "proxyuser", Password: "wrong",
	})
	badResp, err := bad.ExecuteHTTPRequest(&check.HTTPRequest{Method: "GET", URL: origin.HTTPURL})
	if err != nil {
		t.Fatal(err)
	}
	if badResp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("expected 407 with wrong credentials, got %d", badResp.StatusCode)
	}
}

func TestAdapterHTTPSProxy_HTTPRequest(t *testing.T) {
	proxy := testproxy.StartHTTPSProxy(t, "", "")
	t.Setenv("SSL_CERT_FILE", proxy.CertFile)
	t.Setenv("SSL_CERT_DIR", "")

	origin := testproxy.StartOriginHTTP(t)
	host, port := testproxy.HostPort(proxy.Addr)
	adapter := NewHTTPSProxyAdapter(check.ProxyConfig{Type: check.ProxyTypeHTTPS, Host: host, Port: port})

	resp, err := adapter.ExecuteHTTPRequest(&check.HTTPRequest{Method: "GET", URL: origin.HTTPURL})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)
}

func TestAdapterSOCKS4_HTTPRequest(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	proxy := testproxy.StartSOCKS4(t)

	resp, err := socks4Adapter(proxy.Addr).ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET",
		URL:    origin.HTTPURL + "/probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)
}

func TestAdapterSOCKS4_TestPort(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartSOCKS4(t)
	host, port := testproxy.HostPort(origin.Addr)

	ok, err := socks4Adapter(proxy.Addr).TestPort(host, port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected port reachable via SOCKS4")
	}
}

func TestAdapterSOCKS5_HTTPRequest(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	proxy := testproxy.StartSOCKS5(t, "", "")

	resp, err := socks5Adapter(proxy.Addr, "", "").ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET",
		URL:    origin.HTTPURL + "/probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)
}

func TestAdapterSOCKS5_TestPort(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartSOCKS5(t, "", "")
	host, port := testproxy.HostPort(origin.Addr)

	ok, err := socks5Adapter(proxy.Addr, "", "").TestPort(host, port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected port reachable via SOCKS5")
	}
}

func TestAdapterSOCKS5_Auth(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	proxy := testproxy.StartSOCKS5(t, "user", "pass")

	resp, err := socks5Adapter(proxy.Addr, "user", "pass").ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET", URL: origin.HTTPURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginBody(t, resp)

	if _, err := socks5Adapter(proxy.Addr, "user", "wrong").ExecuteHTTPRequest(&check.HTTPRequest{
		Method: "GET", URL: origin.HTTPURL,
	}); err == nil {
		t.Fatal("expected SOCKS5 auth failure with wrong credentials")
	}
}

func TestAdapterTLSThroughHTTPProxy(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartHTTPProxy(t, "", "")
	host, port := testproxy.HostPort(proxy.Addr)

	conn, err := connectViaHTTPProxy(check.ProxyConfig{Type: check.ProxyTypeHTTP, Host: host, Port: port}, origin.Addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		t.Fatal("no peer certificates after TLS handshake through HTTP proxy")
	}
}

func TestAdapterTLSThroughSOCKS5(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartSOCKS5(t, "", "")
	host, port := testproxy.HostPort(proxy.Addr)

	d := &socks5ManualDialer{
		proxyAddr: net.JoinHostPort(host, fmt.Sprint(port)),
		timeout:   5 * time.Second,
	}
	conn, err := d.DialContext(context.Background(), "tcp", origin.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		t.Fatal("no peer certificates after TLS handshake through SOCKS5")
	}
}

func TestAdapterTLSThroughSOCKS4(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	proxy := testproxy.StartSOCKS4(t)
	host, port := testproxy.HostPort(proxy.Addr)

	d := &socks4Dialer{
		proxyAddr: net.JoinHostPort(host, fmt.Sprint(port)),
		timeout:   5 * time.Second,
	}
	conn, err := d.DialContext(context.Background(), "tcp", origin.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	if len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		t.Fatal("no peer certificates after TLS handshake through SOCKS4")
	}
}
