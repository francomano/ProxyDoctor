package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/francomano/proxydoctor/core/adapters"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/utils"
)

const (
	PluginID      = "mcp_server"
	PluginName    = "MCP Server Plugin"
	PluginVersion = "0.1.0"
	DefaultPort   = 9090
)

// MCPServerPlugin exposes ProxyDoctor diagnostic tools via the Model Context Protocol.
type MCPServerPlugin struct {
	port     int
	registry *engine.CheckRegistry
	server   *http.Server
}

func New() *MCPServerPlugin {
	return &MCPServerPlugin{}
}

func (p *MCPServerPlugin) ID() string          { return PluginID }
func (p *MCPServerPlugin) Name() string        { return PluginName }
func (p *MCPServerPlugin) Version() string     { return PluginVersion }
func (p *MCPServerPlugin) Description() string {
	return "Exposes ProxyDoctor diagnostic tools via the Model Context Protocol (MCP)"
}

func (p *MCPServerPlugin) Init(ctx *plugin.Context) error {
	p.registry = ctx.Registry
	p.port = DefaultPort
	if portVal, ok := ctx.Config["port"]; ok {
		if port, ok := portVal.(int); ok {
			p.port = port
		}
	}
	return p.startServer()
}

func (p *MCPServerPlugin) Shutdown() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// startServer starts the MCP HTTP server in a background goroutine.
func (p *MCPServerPlugin) startServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", p.handleMCP)
	mux.HandleFunc("/mcp/", p.handleMCP)

	p.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", p.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("[mcp] MCP server listening on :%d", p.port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[mcp] server error: %v", err)
		}
	}()

	return nil
}

// ---------------------------------------------------------------------------
// MCP HTTP handler
// ---------------------------------------------------------------------------

func (p *MCPServerPlugin) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, newErrorResponse(nil, -32000, "Method not allowed"))
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, newErrorResponse(nil, -32700, "Parse error: "+err.Error()))
		return
	}

	if req.JSONRPC != "2.0" {
		writeJSON(w, http.StatusBadRequest, newErrorResponse(req.ID, -32600, "Invalid JSON-RPC version"))
		return
	}

	var resp jsonRPCResponse
	switch req.Method {
	case "tools/list":
		resp = p.handleToolsList(req)
	case "tools/call":
		resp = p.handleToolsCall(req)
	default:
		resp = newErrorResponse(req.ID, -32601, fmt.Sprintf("Method %q not found", req.Method))
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// tools/list
// ---------------------------------------------------------------------------

func (p *MCPServerPlugin) handleToolsList(req jsonRPCRequest) jsonRPCResponse {
	tools := []mcpTool{
		{
			Name:        "diagnose",
			Description: "Run a proxy diagnosis on a URL. Returns check results with status, evidence, and recommendations.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to diagnose (required)",
					},
					"proxy": map[string]interface{}{
						"type":        "string",
						"description": "Proxy URL (e.g. http://host:port, socks5://host:1080)",
					},
					"proxy_type": map[string]interface{}{
						"type":        "string",
						"description": "Proxy type: auto, http, https, socks4, socks5",
						"enum":        []string{"auto", "http", "https", "socks4", "socks5"},
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "list_checks",
			Description: "List all available diagnostic checks with their metadata.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "compare",
			Description: "Run a comparison diagnosis both directly and through a proxy. Shows differences between direct and proxied results.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to diagnose (required)",
					},
					"proxy": map[string]interface{}{
						"type":        "string",
						"description": "Proxy URL (e.g. http://host:port, socks5://host:1080) (required)",
					},
					"proxy_type": map[string]interface{}{
						"type":        "string",
						"description": "Proxy type: auto, http, https, socks4, socks5",
						"enum":        []string{"auto", "http", "https", "socks4", "socks5"},
					},
				},
				"required": []string{"url", "proxy"},
			},
		},
	}

	return newSuccessResponse(req.ID, map[string]interface{}{
		"tools": tools,
	})
}

// ---------------------------------------------------------------------------
// tools/call
// ---------------------------------------------------------------------------

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (p *MCPServerPlugin) handleToolsCall(req jsonRPCRequest) jsonRPCResponse {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, -32602, "Invalid params: "+err.Error())
	}

	switch params.Name {
	case "diagnose":
		return p.callDiagnose(req.ID, params.Arguments)
	case "list_checks":
		return p.callListChecks(req.ID)
	case "compare":
		return p.callCompare(req.ID, params.Arguments)
	default:
		return newErrorResponse(req.ID, -32602, fmt.Sprintf("Unknown tool: %q", params.Name))
	}
}

type diagnoseArgs struct {
	URL       string `json:"url"`
	Proxy     string `json:"proxy"`
	ProxyType string `json:"proxy_type"`
}

func (p *MCPServerPlugin) callDiagnose(id interface{}, raw json.RawMessage) jsonRPCResponse {
	var args diagnoseArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return newErrorResponse(id, -32602, "Invalid arguments: "+err.Error())
	}
	if args.URL == "" {
		return newErrorResponse(id, -32602, "\"url\" is required")
	}

	proxyConfig, err := utils.ParseProxyConfig(args.Proxy, args.ProxyType)
	if err != nil {
		return newErrorResponse(id, -32602, err.Error())
	}

	adapterFactory := adapters.NewAdapterFactory()
	orchestrator := engine.NewDiagnosisOrchestrator(p.registry, adapterFactory, 4)

	report, err := orchestrator.Execute(engine.DiagnosisRequest{
		URL:         args.URL,
		ProxyConfig: proxyConfig,
		Timeout:     30 * time.Second,
	})
	if err != nil {
		return newErrorResponse(id, -32603, "Diagnosis failed: "+err.Error())
	}

	return p.reportToResult(id, report)
}

func (p *MCPServerPlugin) callListChecks(id interface{}) jsonRPCResponse {
	checks := p.registry.ListChecks()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Available checks (%d):\n", len(checks)))
	for _, c := range checks {
		b.WriteString(fmt.Sprintf("\n  • %s (%s)\n", c.ID(), c.Category()))
		b.WriteString(fmt.Sprintf("    %s\n", c.Description()))
		if deps := c.DependsOn(); len(deps) > 0 {
			b.WriteString(fmt.Sprintf("    Depends on: %s\n", strings.Join(deps, ", ")))
		}
	}
	b.WriteString("\nUse the diagnose tool with a URL to run checks.")

	return newSuccessResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": b.String()},
		},
	})
}

func (p *MCPServerPlugin) callCompare(id interface{}, raw json.RawMessage) jsonRPCResponse {
	var args diagnoseArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return newErrorResponse(id, -32602, "Invalid arguments: "+err.Error())
	}
	if args.URL == "" {
		return newErrorResponse(id, -32602, "\"url\" is required")
	}
	if args.Proxy == "" {
		return newErrorResponse(id, -32602, "\"proxy\" is required")
	}

	proxyConfig, err := utils.ParseProxyConfig(args.Proxy, args.ProxyType)
	if err != nil {
		return newErrorResponse(id, -32602, err.Error())
	}

	adapterFactory := adapters.NewAdapterFactory()
	orchestrator := engine.NewDiagnosisOrchestrator(p.registry, adapterFactory, 4)

	comparison, err := orchestrator.ExecuteComparison(engine.DiagnosisRequest{
		URL:         args.URL,
		ProxyConfig: proxyConfig,
		Timeout:     30 * time.Second,
	})
	if err != nil {
		return newErrorResponse(id, -32603, "Comparison failed: "+err.Error())
	}

	directJSON, _ := json.MarshalIndent(comparison.DirectReport, "", "  ")
	proxyJSON, _ := json.MarshalIndent(comparison.ProxyReport, "", "  ")

	var diffText string
	if len(comparison.Differences) == 0 {
		diffText = "No differences detected."
	} else {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d difference(s):\n", len(comparison.Differences)))
		for _, d := range comparison.Differences {
			b.WriteString(fmt.Sprintf("  - %s\n", d.Summary))
		}
		diffText = b.String()
	}

	text := fmt.Sprintf(`Comparison Results for %s:

%s

Direct Report:
%s

Proxy Report:
%s

Execution Time: %s`,
		comparison.URL, diffText, string(directJSON), string(proxyJSON), comparison.ExecutionTime)

	return newSuccessResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	})
}

func (p *MCPServerPlugin) reportToResult(id interface{}, report *engine.DiagnosisReport) jsonRPCResponse {
	b, _ := json.MarshalIndent(report, "", "  ")

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Diagnosis Results for %s:\n\n", report.RequestMetadata.URL))
	summary.WriteString(fmt.Sprintf("  Checks Executed: %d\n", report.ChecksExecuted))
	summary.WriteString(fmt.Sprintf("  Failed: %d\n", report.ChecksFailed))
	summary.WriteString(fmt.Sprintf("  Critical Findings: %d\n", report.CriticalFindings))
	summary.WriteString(fmt.Sprintf("  Execution Time: %s\n\n", report.ExecutionTime))

	for _, r := range report.Results {
		statusIcon := "✅"
		if r.IsFailed() {
			statusIcon = "❌"
		} else if r.IsError() {
			statusIcon = "⚠️"
		}

		summary.WriteString(fmt.Sprintf("%s %s (%s, %s, %.0f%% confidence)\n", statusIcon, r.ID, r.Status, r.Severity, r.Confidence*100))
		summary.WriteString(fmt.Sprintf("   %s\n", r.Explanation))
		if len(r.ProbableCauses) > 0 {
			summary.WriteString(fmt.Sprintf("   Causes: %s\n", strings.Join(r.ProbableCauses, ", ")))
		}
		if len(r.SuggestedActions) > 0 {
			summary.WriteString(fmt.Sprintf("   Actions: %s\n", strings.Join(r.SuggestedActions, ", ")))
		}
		summary.WriteString("\n")
	}

	return newSuccessResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": summary.String()},
			{"type": "text", "text": "Full JSON:\n" + string(b)},
		},
	})
}

// ---------------------------------------------------------------------------
// JSON-RPC types and helpers
// ---------------------------------------------------------------------------

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

func newSuccessResponse(id interface{}, result interface{}) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func newErrorResponse(id interface{}, code int, message string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
