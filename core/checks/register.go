package checks

import (
	"fmt"

	"github.com/francomano/proxydoctor/core/check"
	dnsresolve "github.com/francomano/proxydoctor/core/checks/dns_resolve"
	portscan "github.com/francomano/proxydoctor/core/checks/port_scan"
	publicip "github.com/francomano/proxydoctor/core/checks/public_ip"
	routetrace "github.com/francomano/proxydoctor/core/checks/route_trace"
	tlscert "github.com/francomano/proxydoctor/core/checks/tls_cert"
	"github.com/francomano/proxydoctor/core/engine"
)

// RegisterDefaults registers all built-in diagnostic checks into the provided registry.
func RegisterDefaults(registry *engine.CheckRegistry) error {
	defaults := []check.Checker{
		publicip.NewPublicIPCheck(),
		dnsresolve.NewDNSResolveCheck(),
		tlscert.NewTLSCertCheck(),
		portscan.NewPortScanCheck(),
		routetrace.NewRouteTraceCheck(),
	}
	for _, checker := range defaults {
		if err := registry.Register(checker); err != nil {
			return fmt.Errorf("register %s: %w", checker.ID(), err)
		}
	}
	return nil
}
