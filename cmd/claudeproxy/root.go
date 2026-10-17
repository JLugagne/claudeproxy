// Package claudeproxy is the cobra command tree of the claudeproxy binary.
package claudeproxy

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:           "claudeproxy",
	Short:         "Model-routing proxy for Claude Code",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "configuration file (env CLAUDEPROXY_CONFIG)")
	rootCmd.AddCommand(serveCmd, routeCmd, setupCmd)
}

// Execute runs the command tree and exits 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "claudeproxy:", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	if path := os.Getenv("CLAUDEPROXY_CONFIG"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "claudeproxy", "config.yaml")
}
