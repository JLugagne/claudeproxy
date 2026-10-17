package claudeproxy

import (
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/JLugagne/claudeproxy/internal/proxy"
	proxyconfig "github.com/JLugagne/claudeproxy/internal/proxy/config"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

var routeCmd = &cobra.Command{
	Use:   "route MODEL...",
	Short: "Validate the configuration and show where each model would be sent",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, models []string) error {
		cfg, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		p, err := proxy.New(cfg.Proxy, proxyconfig.Metrics{Disabled: true}, slog.New(slog.DiscardHandler))
		if err != nil {
			return err
		}
		defer p.Close()

		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "MODEL\tPROVIDER\tUPSTREAM MODEL")
		for _, model := range models {
			decision, err := p.Routes.ResolveRoute(cmd.Context(), domain.Model(model))
			if err != nil {
				fmt.Fprintf(out, "%s\t-\t%v\n", model, err)
				continue
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", model, decision.Provider, decision.Model)
		}
		return out.Flush()
	},
}
