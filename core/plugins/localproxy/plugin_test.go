package localproxy

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/internal/testproxy"
)

func startPlugin(t *testing.T, proxy, proxyType string) *LocalProxyPlugin {
	t.Helper()
	p := New()
	if err := p.Init(&plugin.Context{
		Config: map[string]interface{}{
			"proxy":      proxy,
			"proxy_type": proxyType,
			"port":       0,
			"host":       "127.0.0.1",
		},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = p.Shutdown() })
	return p
}

// proxyClient returns an http.Client whose HTTP/HTTPS traffic is routed
// through the local proxy, like a browser configured to use it.
func proxyClient(addr string) *http.Client {
	proxyURL := &url.URL{Scheme: "http", Host: addr}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}
}

func getBody(t *testing.T, c *http.Client, target string) string {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestLocalProxyDirectMode(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	p := startPlugin(t, "", "")

	addr := p.Addr()
	if addr == "" {
		t.Fatal("plugin address is empty")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		t.Fatalf("invalid plugin address %q: %v", addr, err)
	}

	body := getBody(t, proxyClient(addr), origin.HTTPURL+"/direct")
	if !strings.Contains(body, "origin:method=GET;path=/direct") {
		t.Fatalf("unexpected origin response: %q", body)
	}
}

func TestLocalProxyViaHTTPProxy(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	up := testproxy.StartHTTPProxy(t, "proxyuser", "proxypass")
	p := startPlugin(t, "http://proxyuser:proxypass@"+up.Addr, "http")

	body := getBody(t, proxyClient(p.Addr()), origin.HTTPURL+"/via-http")
	if !strings.Contains(body, "origin:method=GET;path=/via-http") {
		t.Fatalf("unexpected origin response: %q", body)
	}
}

func TestLocalProxyViaSOCKS5(t *testing.T) {
	origin := testproxy.StartOriginHTTP(t)
	up := testproxy.StartSOCKS5(t, "socksuser", "sockspass")
	p := startPlugin(t, "socks5://socksuser:sockspass@"+up.Addr, "socks5")

	body := getBody(t, proxyClient(p.Addr()), origin.HTTPURL+"/via-socks5")
	if !strings.Contains(body, "origin:method=GET;path=/via-socks5") {
		t.Fatalf("unexpected origin response: %q", body)
	}
}

func TestLocalProxyConnectDirect(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	p := startPlugin(t, "", "")

	body := getBody(t, proxyClient(p.Addr()), origin.HTTPSURL+"/connect")
	if !strings.Contains(body, "origin:method=GET;path=/connect") {
		t.Fatalf("unexpected origin response: %q", body)
	}
}

func TestLocalProxyConnectViaHTTPUpstream(t *testing.T) {
	origin := testproxy.StartOriginTLS(t)
	up := testproxy.StartHTTPProxy(t, "", "")
	p := startPlugin(t, "http://"+up.Addr, "http")

	body := getBody(t, proxyClient(p.Addr()), origin.HTTPSURL+"/nested-connect")
	if !strings.Contains(body, "origin:method=GET;path=/nested-connect") {
		t.Fatalf("unexpected origin response: %q", body)
	}
}

func TestLocalProxyShutdown(t *testing.T) {
	p := startPlugin(t, "", "")
	addr := p.Addr()

	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	c := &http.Client{Timeout: 2 * time.Second}
	if _, err := c.Get("http://" + addr + "/"); err == nil {
		t.Fatal("expected error after shutdown, got none")
	}
}
