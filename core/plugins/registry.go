package plugins

import (
	"fmt"

	mcpplugin "github.com/francomano/proxydoctor/core/plugins/mcp"
	routetraceplugin "github.com/francomano/proxydoctor/core/plugins/routetrace"
	"github.com/francomano/proxydoctor/core/plugin"
)

// Available returns all compiled-in plugins keyed by ID.
func Available() map[string]plugin.Plugin {
	return map[string]plugin.Plugin{
		"route_trace": routetraceplugin.New(),
		"mcp_server":  mcpplugin.New(),
	}
}

// Load initializes the requested plugins and registers their checks.
// If names is empty or contains "all", every available plugin is loaded.
func Load(names []string, mgr *plugin.Manager, ctx *plugin.Context) error {
	available := Available()

	if len(names) == 0 {
		return nil
	}

	loadAll := false
	for _, n := range names {
		if n == "all" {
			loadAll = true
			break
		}
	}

	for id, p := range available {
		if loadAll || contains(names, id) {
			if err := mgr.Register(p, ctx); err != nil {
				return fmt.Errorf("loading plugin %s: %w", id, err)
			}
		}
	}
	return nil
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
