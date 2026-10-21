package claudeproxy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/JLugagne/claudeproxy/internal/pkg/setup"
	"github.com/spf13/cobra"
)

var (
	setupBaseURL   string
	setupClaudeDir string
)

func init() {
	setupCmd.Flags().StringVar(&setupBaseURL, "base-url", "", "base URL to write; defaults to server.listen")
	setupCmd.Flags().StringVar(&setupClaudeDir, "claude-dir", "", "Claude Code config directory (default ~/.claude)")
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Point Claude Code at this proxy via settings.json",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		claudeDir := setupClaudeDir
		if claudeDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			claudeDir = filepath.Join(home, ".claude")
		}
		baseURL := setupBaseURL
		if baseURL == "" {
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			baseURL = "http://" + clientListen(cfg.Server.Listen)
		}
		result, err := setup.Apply(claudeDir, baseURL)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "settings  %s\n", result.SettingsPath)
		fmt.Fprintf(out, "base URL  %s\n", baseURL)
		return nil
	},
}

// clientListen turns a listen address into something a client can dial: a
// wildcard host becomes loopback.
func clientListen(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
