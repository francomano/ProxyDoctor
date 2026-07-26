package utils

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/francomano/proxydoctor/core/check"
)

// ParseProxyConfig parses a proxy URL string (e.g. "http://user:pass@host:port")
// together with an explicit proxy type hint ("auto", "http", "https", "socks4", "socks5")
// into a check.ProxyConfig. An empty proxyStr means "no proxy" (direct connection).
//
// Supports three formats:
//   - scheme://host:port  (e.g. socks5://77.245.76.107:1080)
//   - host:port           (e.g. 77.245.76.107:1080) — requires explicit --proxy-type
//   - host                (e.g. 77.245.76.107) — requires explicit --proxy-type, uses default port
func ParseProxyConfig(proxyStr, proxyTypeStr string) (check.ProxyConfig, error) {
	proxyStr = strings.TrimSpace(proxyStr)
	proxyTypeStr = strings.TrimSpace(proxyTypeStr)
	if proxyStr == "" {
		return check.ProxyConfig{Type: check.ProxyTypeDirect}, nil
	}

	// Detect if the input has a scheme (contains "://")
	hasScheme := strings.Contains(proxyStr, "://")

	if hasScheme {
		return parseWithScheme(proxyStr, proxyTypeStr)
	}
	return parseBareHost(proxyStr, proxyTypeStr)
}

// parseWithScheme handles URLs like "socks5://host:port"
func parseWithScheme(proxyStr, proxyTypeStr string) (check.ProxyConfig, error) {
	parsed, err := url.Parse(proxyStr)
	if err != nil {
		return check.ProxyConfig{}, fmt.Errorf("invalid proxy URL %q: %w", proxyStr, err)
	}
	if parsed.Host == "" {
		return check.ProxyConfig{}, fmt.Errorf("invalid proxy URL %q: missing host", proxyStr)
	}

	// Determine proxy type
	var proxyType check.ProxyType
	switch strings.ToLower(proxyTypeStr) {
	case "http":
		proxyType = check.ProxyTypeHTTP
	case "https":
		proxyType = check.ProxyTypeHTTPS
	case "socks4":
		proxyType = check.ProxyTypeSOCKS4
	case "socks5", "socks":
		proxyType = check.ProxyTypeSOCKS5
	case "auto", "":
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			proxyType = check.ProxyTypeHTTP
		case "https":
			proxyType = check.ProxyTypeHTTPS
		case "socks4":
			proxyType = check.ProxyTypeSOCKS4
		case "socks5", "socks":
			proxyType = check.ProxyTypeSOCKS5
		default:
			return check.ProxyConfig{}, fmt.Errorf("cannot infer proxy type from scheme %q — use http, https, socks4, or socks5", parsed.Scheme)
		}
	default:
		return check.ProxyConfig{}, fmt.Errorf("unknown proxy type %q", proxyTypeStr)
	}

	host := parsed.Hostname()
	portStr := parsed.Port()
	var port int
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return check.ProxyConfig{}, fmt.Errorf("invalid proxy port %q: %w", portStr, err)
		}
		port = p
	} else {
		port = defaultPort(proxyType)
	}

	config := check.ProxyConfig{
		Type: proxyType,
		Host: host,
		Port: port,
	}
	if parsed.User != nil {
		config.Username = parsed.User.Username()
		if pw, ok := parsed.User.Password(); ok {
			config.Password = pw
		}
	}

	return config, nil
}

// parseBareHost handles inputs like "host:port" or just "host" (no scheme)
func parseBareHost(proxyStr, proxyTypeStr string) (check.ProxyConfig, error) {
	var proxyType check.ProxyType
	switch strings.ToLower(proxyTypeStr) {
	case "http":
		proxyType = check.ProxyTypeHTTP
	case "https":
		proxyType = check.ProxyTypeHTTPS
	case "socks4":
		proxyType = check.ProxyTypeSOCKS4
	case "socks5", "socks":
		proxyType = check.ProxyTypeSOCKS5
	case "auto", "":
		return check.ProxyConfig{}, fmt.Errorf("cannot auto-detect proxy type from %q — add a scheme (e.g. socks5://%s) or pass an explicit proxy type", proxyStr, proxyStr)
	default:
		return check.ProxyConfig{}, fmt.Errorf("unknown proxy type %q", proxyTypeStr)
	}

	host, portStr, err := net.SplitHostPort(proxyStr)
	if err != nil {
		// No port in the string, treat it as just a host
		host = proxyStr
		portStr = ""
	}

	var port int
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return check.ProxyConfig{}, fmt.Errorf("invalid proxy port %q: %w", portStr, err)
		}
		port = p
	} else {
		port = defaultPort(proxyType)
	}

	return check.ProxyConfig{
		Type: proxyType,
		Host: host,
		Port: port,
	}, nil
}

func defaultPort(pt check.ProxyType) int {
	switch pt {
	case check.ProxyTypeHTTP:
		return 8080
	case check.ProxyTypeHTTPS:
		return 443
	case check.ProxyTypeSOCKS4, check.ProxyTypeSOCKS5:
		return 1080
	default:
		return 80
	}
}
