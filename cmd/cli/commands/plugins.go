package commands

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins"
)

var pluginNames string

func init() {
	RootCmd.PersistentFlags().StringVar(&pluginNames, "plugins", "", "Comma-separated plugin IDs to load (e.g., route_trace, mcp_server) or 'all'")
}

func loadPlugins(registry *engine.CheckRegistry) (*plugin.Manager, error) {
	if pluginNames == "" {
		return nil, nil
	}

	mgr := plugin.NewManager()
	names := strings.Split(pluginNames, ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}
	if err := plugins.Load(names, mgr, ctx); err != nil {
		return nil, err
	}
	return mgr, nil
}

func runPlugins() {
	if pluginNames == "" {
		fmt.Println("proxyctl: use --plugins to specify plugins to load")
		fmt.Println()
		fmt.Println("Available commands:")
		fmt.Println("  diagnose      Run a comprehensive diagnosis on a URL")
		fmt.Println("  list-checks   List all available checks")
		fmt.Println("  version       Show version information")
		fmt.Println()
		fmt.Println("Plugins:")
		fmt.Println("  route_trace   Registers a check (use with: diagnose --plugins route_trace)")
		fmt.Println("  mcp_server    Starts a JSON-RPC 2.0 server on :9090 (use: --plugins mcp_server)")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  proxyctl --plugins mcp_server")
		fmt.Println("  proxyctl diagnose --url https://example.com --plugins route_trace")
		return
	}

	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		fmt.Printf("failed to register checks: %v\n", err)
		return
	}

	mgr, err := loadPlugins(registry)
	if err != nil {
		fmt.Printf("failed to load plugins: %v\n", err)
		return
	}
	if mgr == nil {
		return
	}
	defer mgr.ShutdownAll()

	fmt.Printf("Plugins loaded: %s\n", pluginNames)
	fmt.Println("Press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down...")
}
