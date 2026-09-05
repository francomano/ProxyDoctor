package checks

import (
	"fmt"

	"github.com/francomano/proxydoctor/core/check"
	dnsleak "github.com/francomano/proxydoctor/core/checks/dns_leak"
	dnsresolve "github.com/francomano/proxydoctor/core/checks/dns_resolve"
	headerleak "github.com/francomano/proxydoctor/core/checks/header_leak"
	ipv6leak "github.com/francomano/proxydoctor/core/checks/ipv6_leak"
	portscan "github.com/francomano/proxydoctor/core/checks/port_scan"
	proxyfingerprint "github.com/francomano/proxydoctor/core/checks/proxy_fingerprint"
	publicip "github.com/francomano/proxydoctor/core/checks/public_ip"
	tlscert "github.com/francomano/proxydoctor/core/checks/tls_cert"
	webrtcleak "github.com/francomano/proxydoctor/core/checks/webrtc_leak"
	"github.com/francomano/proxydoctor/core/engine"
)

// RegisterDefaults registers all built-in diagnostic checks into the provided registry.
func RegisterDefaults(registry *engine.CheckRegistry) error {
	defaults := []check.Checker{
		publicip.NewPublicIPCheck(),
		dnsresolve.NewDNSResolveCheck(),
		tlscert.NewTLSCertCheck(),
		portscan.NewPortScanCheck(),
		ipv6leak.NewIPv6LeakCheck(),
		dnsleak.NewDNSLeakCheck(),
		webrtcleak.NewWebRTCLeakCheck(),
		headerleak.NewHeaderLeakCheck(),
		proxyfingerprint.NewProxyFingerprintCheck(),
	}
	for _, checker := range defaults {
		if err := registry.Register(checker); err != nil {
			return fmt.Errorf("register %s: %w", checker.ID(), err)
		}
	}
	return nil
}
