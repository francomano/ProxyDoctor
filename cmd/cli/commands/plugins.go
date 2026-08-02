package commands

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins"
)

var pluginNames string

var (
	localProxyPort int
	localProxyHost string
)

func init() {
	RootCmd.PersistentFlags().StringVar(&pluginNames, "plugins", "", "Comma-separated plugin IDs to load (e.g., route_trace, local_proxy) or 'all'")
	RootCmd.PersistentFlags().StringVarP(&proxyStr, "proxy", "p", "", "Proxy URL (e.g., http://localhost:8080, socks5://localhost:1080)")
	RootCmd.PersistentFlags().StringVar(&proxyType, "proxy-type", "auto", "Proxy type: auto, http, https, socks4, socks5")
	RootCmd.PersistentFlags().IntVar(&localProxyPort, "port", 8081, "Local port for standalone proxy plugins (e.g. local_proxy)")
	RootCmd.PersistentFlags().StringVar(&localProxyHost, "host", "127.0.0.1", "Local bind address for standalone proxy plugins")
}

// pluginConfig builds the config map passed to plugins. Only explicitly-set
// flags are forwarded so each plugin keeps its own defaults otherwise.
func pluginConfig(changed func(name string) bool) map[string]interface{} {
	cfg := map[string]interface{}{
		"proxy":      proxyStr,
		"proxy_type": proxyType,
	}
	if changed("port") {
		cfg["port"] = localProxyPort
	}
	if changed("host") {
		cfg["host"] = localProxyHost
	}
	return cfg
}

func loadPlugins(registry *engine.CheckRegistry, changed func(name string) bool) (*plugin.Manager, error) {
	if pluginNames == "" {
		return nil, nil
	}

	mgr := plugin.NewManager()
	names := strings.Split(pluginNames, ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	ctx := &plugin.Context{Registry: registry, Config: pluginConfig(changed)}
	if err := plugins.Load(names, mgr, ctx); err != nil {
		return nil, err
	}
	return mgr, nil
}

func runPlugins(cmd *cobra.Command) {
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
		fmt.Println("  local_proxy   Starts a local forward proxy on :8081 (use: --plugins local_proxy --proxy <upstream>)")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  proxyctl --plugins mcp_server")
		fmt.Println("  proxyctl --plugins local_proxy --proxy socks5://77.245.76.107:1080")
		fmt.Println("  proxyctl diagnose --url https://example.com --plugins route_trace")
		return
	}

	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		fmt.Printf("failed to register checks: %v\n", err)
		return
	}

	mgr, err := loadPlugins(registry, cmd.PersistentFlags().Changed)
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
