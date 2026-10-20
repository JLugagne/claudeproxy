package claudeproxy

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/JLugagne/claudeproxy/internal/proxy"
)

var listenOverride string
var allowNonLoopback bool

func init() {
	serveCmd.Flags().StringVar(&listenOverride, "listen", "", "listen address, overrides server.listen")
	serveCmd.Flags().BoolVar(&allowNonLoopback, "allow-non-loopback", false, "bind a non-loopback address; the proxy becomes reachable beyond localhost")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the proxy",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		if listenOverride != "" {
			cfg.Server.Listen = listenOverride
		}
		if !allowNonLoopback {
			if err := cfg.Server.ValidateListen(); err != nil {
				return err
			}
		}
		logger, err := cfg.Server.NewLogger(os.Stderr)
		if err != nil {
			return err
		}
		p, err := proxy.New(cfg.Proxy, cfg.Metrics, logger)
		if err != nil {
			return err
		}
		defer p.Close()

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		listener, err := net.Listen("tcp", cfg.Server.Listen)
		if err != nil {
			return err
		}
		server := &http.Server{
			Handler:           p.Handler,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		}
		server.RegisterOnShutdown(p.CloseLiveFeeds)
		served := make(chan error, 1)
		go func() { served <- server.Serve(listener) }()
		logger.Info("claudeproxy listening", "addr", listener.Addr().String(), "config", configPath)

		select {
		case err := <-served:
			return err
		case <-ctx.Done():
		}
		logger.Info("shutting down", "grace", cfg.Server.ShutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Server.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	},
}
