package localproxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/francomano/proxydoctor/core/adapters"
	"github.com/francomano/proxydoctor/core/check"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/utils"
)

const (
	PluginID      = "local_proxy"
	PluginName    = "Local Forward Proxy Plugin"
	PluginVersion = "0.1.0"
	DefaultPort   = 8081
	DefaultHost   = "127.0.0.1"
)

// LocalProxyPlugin exposes the proxy you just tested as a local forward proxy,
// so browsers, curl, wget and any other tool can browse through it.
type LocalProxyPlugin struct {
	proxyConfig check.ProxyConfig
	bindHost    string
	port        int
	server      *http.Server
	listener    net.Listener
	upstream    adapters.DialContextFunc
}

// New creates a LocalProxyPlugin with defaults.
func New() *LocalProxyPlugin {
	return &LocalProxyPlugin{
		bindHost: DefaultHost,
		port:     DefaultPort,
	}
}

func (p *LocalProxyPlugin) ID() string          { return PluginID }
func (p *LocalProxyPlugin) Name() string        { return PluginName }
func (p *LocalProxyPlugin) Version() string     { return PluginVersion }
func (p *LocalProxyPlugin) Description() string {
	return "Exposes the tested proxy as a local forward proxy for browsing and downloads"
}

func (p *LocalProxyPlugin) Init(ctx *plugin.Context) error {
	proxyStr, _ := ctx.Config["proxy"].(string)
	proxyTypeStr, _ := ctx.Config["proxy_type"].(string)

	if portVal, ok := ctx.Config["port"].(int); ok && portVal >= 0 {
		p.port = portVal
	}
	if hostVal, ok := ctx.Config["host"].(string); ok && hostVal != "" {
		p.bindHost = hostVal
	}

	cfg, err := utils.ParseProxyConfig(proxyStr, proxyTypeStr)
	if err != nil {
		return fmt.Errorf("invalid upstream proxy config: %w", err)
	}
	p.proxyConfig = cfg

	dial, err := adapters.NewProxyDialContext(cfg)
	if err != nil {
		return err
	}
	p.upstream = dial

	return p.startServer()
}

func (p *LocalProxyPlugin) Shutdown() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// Addr returns the bound local address of the proxy ("" if not started).
func (p *LocalProxyPlugin) Addr() string {
	if p.listener != nil {
		return p.listener.Addr().String()
	}
	return ""
}

// startServer starts the local forward proxy in a background goroutine.
func (p *LocalProxyPlugin) startServer() error {
	transport, err := adapters.ForwardTransport(p.proxyConfig)
	if err != nil {
		return err
	}

	handler := &proxyHandler{
		upstream:  p.upstream,
		transport: transport,
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", p.bindHost, p.port))
	if err != nil {
		return fmt.Errorf("could not listen on %s:%d: %w", p.bindHost, p.port, err)
	}
	p.listener = ln

	p.server = &http.Server{
		Handler:     handler,
		ReadTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("[local_proxy] local forward proxy listening on %s", p.Addr())
		p.printSetup()
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[local_proxy] server error: %v", err)
		}
	}()

	return nil
}

// printSetup prints the "appeal moment": how to point every tool at the proxy.
func (p *LocalProxyPlugin) printSetup() {
	addr := fmt.Sprintf("http://%s", p.Addr())
	fmt.Println()
	fmt.Println("  🚀 Local forward proxy ready — route your traffic through it:")
	fmt.Println()
	fmt.Printf("     Browser   → set HTTP/HTTPS proxy to %s\n", addr)
	fmt.Printf("     curl      → curl -x %s https://example.com\n", addr)
	fmt.Printf("     wget      → wget -e use_proxy=yes -e http_proxy=%s https://example.com\n", addr)
	fmt.Println()
	if p.proxyConfig.Type != check.ProxyTypeDirect {
		fmt.Printf("  Upstream: %s://%s:%d (credentials kept local)\n", p.proxyConfig.Type, p.proxyConfig.Host, p.proxyConfig.Port)
	} else {
		fmt.Println("  Upstream: direct connection (no proxy)")
	}
	fmt.Println()
}

// proxyHandler implements the local forward proxy (CONNECT + plain HTTP).
type proxyHandler struct {
	upstream  adapters.DialContextFunc
	transport *http.Transport
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
		return
	}
	h.handleHTTP(w, r)
}

// handleHTTP forwards a plain HTTP request through the upstream transport.
func (h *proxyHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() {
		r.URL.Scheme = "http"
		r.URL.Host = r.Host
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""

	resp, err := h.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer closeBody(resp)

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleConnect tunnels a CONNECT (HTTPS) request through the upstream.
func (h *proxyHandler) handleConnect(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if target == "" {
		target = r.URL.Host
	}
	if target == "" {
		http.Error(w, "missing CONNECT target", http.StatusBadRequest)
		return
	}

	upstream, err := h.upstream(r.Context(), "tcp", target)
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to reach %s: %v", target, err), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, buf, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = clientConn.Close()
		_ = upstream.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		_ = clientConn.Close()
		_ = upstream.Close()
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(upstream, buf)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(clientConn, upstream)
	}()
	<-done

	_ = clientConn.Close()
	_ = upstream.Close()
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func closeBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_ = resp.Body.Close()
}
