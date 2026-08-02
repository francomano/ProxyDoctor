// Package testproxy provides local, hermetic proxy fixtures used by the
// integration tests. All servers bind to 127.0.0.1 with ephemeral ports, so
// `go test ./...` works offline and in CI.
package testproxy

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Origin is a local target HTTP(S) server that tests route to.
type Origin struct {
	Addr     string
	HTTPURL  string
	HTTPSURL string
	CertFile string
}

// StartOriginHTTP starts a local HTTP server that echoes the request.
func StartOriginHTTP(t testing.TB) *Origin {
	t.Helper()
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close")
			_, _ = fmt.Fprintf(w, "origin:method=%s;path=%s;host=%s", r.Method, r.URL.Path, r.Host)
		}),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return &Origin{Addr: ln.Addr().String(), HTTPURL: "http://" + ln.Addr().String()}
}

// StartOriginTLS starts a local server serving both plain HTTP and HTTPS with
// a self-signed certificate (the client must trust it or skip verification).
func StartOriginTLS(t testing.TB) *Origin {
	t.Helper()
	cert := selfSignedCert(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		_, _ = fmt.Fprintf(w, "origin:method=%s;path=%s;host=%s", r.Method, r.URL.Path, r.Host)
	})

	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: handler}
	go func() { _ = httpSrv.Serve(httpLn) }()
	t.Cleanup(func() { _ = httpSrv.Close() })

	tlsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsSrv := &http.Server{Handler: handler}
	go func() { _ = tlsSrv.Serve(tls.NewListener(tlsLn, &tls.Config{Certificates: []tls.Certificate{cert}})) }()
	t.Cleanup(func() { _ = tlsSrv.Close() })

	return &Origin{
		Addr:     tlsLn.Addr().String(),
		HTTPURL:  "http://" + httpLn.Addr().String(),
		HTTPSURL: "https://" + tlsLn.Addr().String(),
		CertFile: writeCertFile(t, cert),
	}
}

// HostPort splits an addr into host and port.
func HostPort(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "127.0.0.1", 0
	}
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

// ---------------------------------------------------------------------------
// HTTP / HTTPS forward proxy fixtures
// ---------------------------------------------------------------------------

// HTTPProxy is a local forward HTTP proxy supporting absolute-form requests
// and CONNECT tunneling.
type HTTPProxy struct {
	Addr     string
	CertFile string
	username string
	password string
}

// StartHTTPProxy starts a local forward HTTP proxy. Empty username/password
// disables authentication.
func StartHTTPProxy(t testing.TB, username, password string) *HTTPProxy {
	t.Helper()
	return startProxy(t, username, password, false)
}

// StartHTTPSProxy starts a TLS-wrapped forward HTTP proxy with a self-signed
// certificate. Trust it in tests with t.Setenv("SSL_CERT_FILE", proxy.CertFile).
func StartHTTPSProxy(t testing.TB, username, password string) *HTTPProxy {
	t.Helper()
	return startProxy(t, username, password, true)
}

func startProxy(t testing.TB, username, password string, secure bool) *HTTPProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cert := selfSignedCert(t)
	if secure {
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	}

	p := &HTTPProxy{Addr: ln.Addr().String(), username: username, password: password}
	if secure {
		p.CertFile = writeCertFile(t, cert)
	}

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				p.handleConn(c)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return p
}

func (p *HTTPProxy) handleConn(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))

	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})

	if p.username != "" && !basicAuthOK(req, p.username, p.password) {
		_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}

	if req.Method == http.MethodConnect {
		p.handleConnect(c, req, br)
		return
	}
	p.handlePlain(c, req)
}

func (p *HTTPProxy) handlePlain(c net.Conn, req *http.Request) {
	req.RequestURI = ""
	resp, err := proxyRoundTripper.RoundTrip(req)
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	defer resp.Body.Close()
	_ = resp.Write(c)
}

func (p *HTTPProxy) handleConnect(c net.Conn, req *http.Request, br *bufio.Reader) {
	target := req.Host
	if target == "" {
		target = req.URL.Host
	}
	if target == "" {
		_, _ = io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}

	up, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = up.Close()
		return
	}
	tunnel(c, up, br)
}

// ---------------------------------------------------------------------------
// SOCKS5 fixture
// ---------------------------------------------------------------------------

// SOCKS5 is a local SOCKS5 proxy supporting no-auth and username/password auth.
type SOCKS5 struct {
	Addr     string
	username string
	password string
}

// StartSOCKS5 starts a local SOCKS5 proxy. Empty username/password disables
// authentication.
func StartSOCKS5(t testing.TB, username, password string) *SOCKS5 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &SOCKS5{Addr: ln.Addr().String(), username: username, password: password}

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				s.handle(c)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return s
}

func (s *SOCKS5) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(c)

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(br, greeting); err != nil || greeting[0] != 0x05 {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}

	want := byte(0x00)
	if s.username != "" {
		want = 0x02
	}
	chosen := byte(0xFF)
	for _, m := range methods {
		if m == want {
			chosen = want
			break
		}
	}
	if chosen == 0xFF {
		_, _ = c.Write([]byte{0x05, 0xFF})
		return
	}
	if _, err := c.Write([]byte{0x05, chosen}); err != nil {
		return
	}

	if chosen == 0x02 {
		if err := s.readAuth(br, c); err != nil {
			return
		}
	}

	up, err := s.readConnect(br, c)
	if err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	tunnel(c, up, br)
}

func (s *SOCKS5) readAuth(br *bufio.Reader, c net.Conn) error {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(br, hdr); err != nil || hdr[0] != 0x01 {
		return fmt.Errorf("bad auth header")
	}
	user := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(br, user); err != nil {
		return err
	}
	passLen, err := br.ReadByte()
	if err != nil {
		return err
	}
	pass := make([]byte, int(passLen))
	if _, err := io.ReadFull(br, pass); err != nil {
		return err
	}
	if string(user) != s.username || string(pass) != s.password {
		_, _ = c.Write([]byte{0x01, 0x01})
		return fmt.Errorf("bad credentials")
	}
	_, _ = c.Write([]byte{0x01, 0x00})
	return nil
}

func (s *SOCKS5) readConnect(br *bufio.Reader, c net.Conn) (net.Conn, error) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, err
	}
	if hdr[0] != 0x05 || hdr[1] != 0x01 {
		_, _ = c.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, fmt.Errorf("unsupported command")
	}

	var host string
	switch hdr[3] {
	case 0x01:
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	case 0x03:
		n, err := br.ReadByte()
		if err != nil {
			return nil, err
		}
		b := make([]byte, int(n))
		if _, err := io.ReadFull(br, b); err != nil {
			return nil, err
		}
		host = string(b)
	case 0x04:
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	default:
		_, _ = c.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, fmt.Errorf("bad atyp")
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(br, portBytes); err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(portBytes)

	up, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 5*time.Second)
	if err != nil {
		_, _ = c.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return nil, err
	}
	_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0x00, 0x00})
	return up, nil
}

// ---------------------------------------------------------------------------
// SOCKS4 fixture
// ---------------------------------------------------------------------------

// SOCKS4 is a local SOCKS4/SOCKS4a proxy (no auth).
type SOCKS4 struct {
	Addr string
}

// StartSOCKS4 starts a local SOCKS4 proxy.
func StartSOCKS4(t testing.TB) *SOCKS4 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &SOCKS4{Addr: ln.Addr().String()}

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				s.handle(c)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return s
}

func (s *SOCKS4) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(c)

	hdr := make([]byte, 8)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return
	}
	if hdr[0] != 0x04 || hdr[1] != 0x01 {
		return
	}

	port := binary.BigEndian.Uint16(hdr[2:4])
	ip := net.IPv4(hdr[4], hdr[5], hdr[6], hdr[7])

	var host string
	if ip.Equal(net.IPv4zero) || ip[3] == 1 && ip[0] == 0 && ip[1] == 0 && ip[2] == 0 {
		if _, err := readCString(br); err != nil {
			return
		}
		name, err := readCString(br)
		if err != nil {
			return
		}
		host = name
	} else {
		if _, err := readCString(br); err != nil {
			return
		}
		host = ip.String()
	}

	up, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 5*time.Second)
	if err != nil {
		_, _ = c.Write([]byte{0x00, 0x5B, hdr[2], hdr[3], hdr[4], hdr[5], hdr[6], hdr[7]})
		return
	}
	_, _ = c.Write([]byte{0x00, 0x5A, hdr[2], hdr[3], hdr[4], hdr[5], hdr[6], hdr[7]})
	_ = c.SetDeadline(time.Time{})
	tunnel(c, up, br)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var proxyRoundTripper = &http.Transport{Proxy: nil, DisableKeepAlives: true}

func tunnel(c net.Conn, up net.Conn, br *bufio.Reader) {
	defer c.Close()
	defer up.Close()

	// Bound the tunnel so idle keep-alive connections cannot block test cleanup.
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	_ = up.SetDeadline(time.Now().Add(15 * time.Second))

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(up, br)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(c, up)
	}()
	<-done
}

func readCString(br *bufio.Reader) (string, error) {
	var out []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			return string(out), nil
		}
		out = append(out, b)
	}
}

func basicAuthOK(req *http.Request, user, pass string) bool {
	h := req.Header.Get("Proxy-Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, prefix))
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	return len(parts) == 2 && parts[0] == user && parts[1] == pass
}

func selfSignedCert(t testing.TB) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func writeCertFile(t testing.TB, cert tls.Certificate) string {
	t.Helper()
	block := &pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}
	dir := t.TempDir()
	path := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
