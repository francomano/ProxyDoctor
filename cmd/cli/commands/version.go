package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ProxyDoctor v0.3.0")
		fmt.Println("Comprehensive proxy diagnostics tool")
		fmt.Println("by Marco Francomano")
	},
}
