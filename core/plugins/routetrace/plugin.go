package routetrace

import (
	"github.com/francomano/proxydoctor/core/checks/route_trace"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
)

const (
	pluginID      = "route_trace"
	pluginName    = "Route Trace Plugin"
	pluginVersion = "0.1.0"
)

// RouteTracePlugin wraps the route_trace check as a plugin.
type RouteTracePlugin struct{}

func New() *RouteTracePlugin { return &RouteTracePlugin{} }

func (p *RouteTracePlugin) ID() string          { return pluginID }
func (p *RouteTracePlugin) Name() string        { return pluginName }
func (p *RouteTracePlugin) Version() string     { return pluginVersion }
func (p *RouteTracePlugin) Description() string { return "Adds the route trace diagnostic check" }

func (p *RouteTracePlugin) Init(_ *plugin.Context) error { return nil }
func (p *RouteTracePlugin) Shutdown() error              { return nil }

func (p *RouteTracePlugin) RegisterChecks(registry *engine.CheckRegistry) error {
	return registry.Register(routetrace.NewRouteTraceCheck())
}

var _ plugin.CheckPlugin = (*RouteTracePlugin)(nil)
