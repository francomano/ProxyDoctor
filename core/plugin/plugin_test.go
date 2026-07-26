package plugin

import (
	"testing"

	"github.com/francomano/proxydoctor/core/engine"
)

// stubPlugin is a minimal plugin implementation for testing.
type stubPlugin struct {
	id         string
	name       string
	initErr    error
	shutErr    error
	initCalled bool
	shutCalled bool
}

func (s *stubPlugin) ID() string          { return s.id }
func (s *stubPlugin) Name() string        { return s.name }
func (s *stubPlugin) Version() string     { return "0.1.0" }
func (s *stubPlugin) Description() string { return "test plugin" }
func (s *stubPlugin) Init(_ *Context) error {
	s.initCalled = true
	return s.initErr
}
func (s *stubPlugin) Shutdown() error {
	s.shutCalled = true
	return s.shutErr
}

func TestManager_RegisterAndGet(t *testing.T) {
	m := NewManager()
	p := &stubPlugin{id: "test-1", name: "Test Plugin"}
	ctx := &Context{Registry: engine.NewCheckRegistry(), Config: map[string]interface{}{}}

	if err := m.Register(p, ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !p.initCalled {
		t.Fatal("Init was not called")
	}

	got, ok := m.GetPlugin("test-1")
	if !ok || got.ID() != "test-1" {
		t.Fatalf("GetPlugin: got %v, %v", got, ok)
	}

	if len(m.Plugins()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m.Plugins()))
	}
}

func TestManager_Unregister(t *testing.T) {
	m := NewManager()
	p := &stubPlugin{id: "test-1", name: "Test Plugin"}
	ctx := &Context{Registry: engine.NewCheckRegistry()}

	m.Register(p, ctx)

	if err := m.Unregister("test-1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if !p.shutCalled {
		t.Fatal("Shutdown was not called")
	}
	if len(m.Plugins()) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(m.Plugins()))
	}
}

func TestManager_UnregisterNotFound(t *testing.T) {
	m := NewManager()
	if err := m.Unregister("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestManager_ShutdownAll(t *testing.T) {
	m := NewManager()
	ctx := &Context{Registry: engine.NewCheckRegistry()}

	p1 := &stubPlugin{id: "a", name: "A"}
	p2 := &stubPlugin{id: "b", name: "B"}
	m.Register(p1, ctx)
	m.Register(p2, ctx)

	m.ShutdownAll()
	if !p1.shutCalled || !p2.shutCalled {
		t.Fatal("not all plugins were shut down")
	}
}

func TestManager_InitError(t *testing.T) {
	m := NewManager()
	p := &stubPlugin{id: "bad", name: "Bad", initErr: &testError{"init failed"}}
	ctx := &Context{Registry: engine.NewCheckRegistry()}

	if err := m.Register(p, ctx); err == nil {
		t.Fatal("expected error on init failure")
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
