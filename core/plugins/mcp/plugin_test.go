package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
)

func TestPluginMetadata(t *testing.T) {
	p := New()
	if p.ID() != PluginID {
		t.Fatalf("ID: got %q, want %q", p.ID(), PluginID)
	}
	if p.Name() != PluginName {
		t.Fatalf("Name: got %q, want %q", p.Name(), PluginName)
	}
	if p.Version() != PluginVersion {
		t.Fatalf("Version: got %q, want %q", p.Version(), PluginVersion)
	}
	if p.Description() == "" {
		t.Fatal("Description should not be empty")
	}
}

func TestPluginInitAndShutdown(t *testing.T) {
	p := New()
	registry := engine.NewCheckRegistry()
	checkspkg.RegisterDefaults(registry)

	ctx := &plugin.Context{
		Registry: registry,
		Config: map[string]interface{}{
			"port": 19091,
		},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if p.port != 19091 {
		t.Fatalf("port: got %d, want %d", p.port, 19091)
	}
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestPluginInitDefaultPort(t *testing.T) {
	p := New()
	registry := engine.NewCheckRegistry()
	checkspkg.RegisterDefaults(registry)

	// Use a non-default port to avoid conflict; test that config key works
	ctx := &plugin.Context{
		Registry: registry,
		Config: map[string]interface{}{
			"port": 19092,
		},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if p.port != 19092 {
		t.Fatalf("port: got %d, want %d", p.port, 19092)
	}
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestPluginInitDefaultPortWhenNoConfig(t *testing.T) {
	p := New()
	registry := engine.NewCheckRegistry()
	checkspkg.RegisterDefaults(registry)

	// Override port in config to avoid actually listening on 9090
	ctx := &plugin.Context{
		Registry: registry,
		Config: map[string]interface{}{
			"port": 19093,
		},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if p.port != 19093 {
		t.Fatalf("port: got %d, want %d", p.port, 19093)
	}
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestPluginViaManager(t *testing.T) {
	m := plugin.NewManager()
	registry := engine.NewCheckRegistry()
	checkspkg.RegisterDefaults(registry)

	ctx := &plugin.Context{
		Registry: registry,
		Config: map[string]interface{}{
			"port": 19094,
		},
	}

	p := New()
	if err := m.Register(p, ctx); err != nil {
		t.Fatalf("Manager.Register: %v", err)
	}

	plugins := m.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	if err := m.ShutdownAll(); err != nil {
		t.Fatalf("ShutdownAll: %v", err)
	}
}

func TestHandleToolsList(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	resp := p.handleToolsList(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type: got %T, want map", resp.Result)
	}

	tools, ok := result["tools"].([]mcpTool)
	if !ok {
		t.Fatalf("tools type: got %T, want []mcpTool", result["tools"])
	}

	expectedTools := map[string]bool{"diagnose": false, "list_checks": false, "compare": false}
	for _, tool := range tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
		if tool.Description == "" {
			t.Fatalf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Fatalf("tool %q has nil inputSchema", tool.Name)
		}
	}
	for name, found := range expectedTools {
		if !found {
			t.Fatalf("expected tool %q not found", name)
		}
	}
}

func TestHandleListChecks(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	rawArgs, _ := json.Marshal(toolsCallParams{Name: "list_checks"})
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  rawArgs,
	}

	resp := p.handleToolsCall(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}
}

func TestHandleUnknownTool(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	rawArgs, _ := json.Marshal(toolsCallParams{Name: "nonexistent"})
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  rawArgs,
	}

	resp := p.handleToolsCall(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestHandleDiagnoseMissingURL(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	rawArgs, _ := json.Marshal(toolsCallParams{
		Name:      "diagnose",
		Arguments: json.RawMessage(`{"url": ""}`),
	})
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  rawArgs,
	}

	resp := p.handleToolsCall(req)
	if resp.Error == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestHandleInvalidArguments(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	rawArgs, _ := json.Marshal(toolsCallParams{
		Name:      "diagnose",
		Arguments: json.RawMessage(`not valid json`),
	})
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  rawArgs,
	}

	resp := p.handleToolsCall(req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "unknown_method",
	}

	resp := p.handleMCPRequest(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Fatalf("error code: got %d, want %d", resp.Error.Code, -32601)
	}
}

func TestHTTPHandlerToolsList(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	body := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	b, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(b))
	w := httptest.NewRecorder()

	p.handleMCP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}
}

func TestHTTPHandlerInvalidJSON(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{invalid`)))
	w := httptest.NewRecorder()

	p.handleMCP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHTTPHandlerSSE(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		p.handleMCP(w, r)
		close(done)
	}()

	// Let the handler write the endpoint event.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type: got %q, want %q", ct, "text/event-stream")
	}
	if !strings.Contains(w.Body.String(), "event: endpoint") {
		t.Fatal("expected endpoint event in SSE stream")
	}
}

func TestHTTPHandlerUnsupportedMethod(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	r := httptest.NewRequest(http.MethodPut, "/mcp", nil)
	w := httptest.NewRecorder()

	p.handleMCP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHTTPHandlerWrongVersion(t *testing.T) {
	p := newHandlerPlugin(t)
	defer p.Shutdown()

	body := map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      1,
		"method":  "tools/list",
	}
	b, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(b))
	w := httptest.NewRecorder()

	p.handleMCP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStartAndStopServer(t *testing.T) {
	port := 19095
	p := newHandlerPlugin(t)
	p.port = port

	if err := p.startServer(); err != nil {
		t.Fatalf("startServer: %v", err)
	}
	defer p.Shutdown()

	// Give the server goroutine time to bind.
	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/mcp", port)
	resp, err := http.Post(
		url,
		"application/json",
		bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rpcResp.Error != nil {
		t.Fatalf("unexpected error: %s", rpcResp.Error.Message)
	}
}

// newHandlerPlugin creates a plugin with a registry but without starting an HTTP server.
func newHandlerPlugin(t *testing.T) *MCPServerPlugin {
	t.Helper()
	p := New()
	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		t.Fatalf("RegisterDefaults: %v", err)
	}
	p.registry = registry
	p.port = 0 // no server
	return p
}

// handleMCPRequest routes an MCP method without the HTTP layer.
func (p *MCPServerPlugin) handleMCPRequest(req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return p.handleInitialize(req)
	case "tools/list":
		return p.handleToolsList(req)
	case "tools/call":
		return p.handleToolsCall(req)
	case "notifications/initialized":
		return jsonRPCResponse{JSONRPC: "2.0"}
	default:
		return newErrorResponse(req.ID, -32601, fmt.Sprintf("Method %q not found", req.Method))
	}
}
