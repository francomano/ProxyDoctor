package routetrace

import (
	"testing"

	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
)

func TestPluginMetadata(t *testing.T) {
	p := New()
	if p.ID() != "route_trace" {
		t.Fatalf("ID: got %q, want %q", p.ID(), "route_trace")
	}
	if p.Name() == "" {
		t.Fatal("Name should not be empty")
	}
	if p.Version() == "" {
		t.Fatal("Version should not be empty")
	}
	if p.Description() == "" {
		t.Fatal("Description should not be empty")
	}
}

func TestPluginInitAndShutdown(t *testing.T) {
	p := New()
	ctx := &plugin.Context{
		Registry: engine.NewCheckRegistry(),
		Config:   map[string]interface{}{},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestPluginRegisterChecks(t *testing.T) {
	p := New()
	registry := engine.NewCheckRegistry()

	if err := p.RegisterChecks(registry); err != nil {
		t.Fatalf("RegisterChecks: %v", err)
	}

	check, ok := registry.GetCheck("route_trace")
	if !ok {
		t.Fatal("route_trace check was not registered")
	}
	if check.ID() != "route_trace" {
		t.Fatalf("check ID: got %q, want %q", check.ID(), "route_trace")
	}
}

func TestPluginViaManager(t *testing.T) {
	m := plugin.NewManager()
	registry := engine.NewCheckRegistry()
	ctx := &plugin.Context{
		Registry: registry,
		Config:   map[string]interface{}{},
	}

	p := New()
	if err := m.Register(p, ctx); err != nil {
		t.Fatalf("Manager.Register: %v", err)
	}

	check, ok := registry.GetCheck("route_trace")
	if !ok {
		t.Fatal("route_trace check not registered via Manager")
	}
	if check.Category() == "" {
		t.Fatal("check category should not be empty")
	}

	plugins := m.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	if err := m.ShutdownAll(); err != nil {
		t.Fatalf("ShutdownAll: %v", err)
	}
}
