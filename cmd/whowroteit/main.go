package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/anthropic/who-wrote-it/internal/config"
	"github.com/anthropic/who-wrote-it/internal/daemon"
	"github.com/anthropic/who-wrote-it/internal/ipc"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "whowroteit",
		Short: "Track human vs AI code authorship",
		Long:  "who-wrote-it is a daemon that monitors your development workflow to attribute code to human or AI authors.",
	}

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(pingCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startCmd() *cobra.Command {
	var foreground bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the who-wrote-it daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Check if daemon is already running.
			client := ipc.NewClient(cfg.SocketPath)
			if err := client.Ping(); err == nil {
				fmt.Println("daemon is already running")
				return nil
			}

			// Remove stale socket file (from a prior crash).
			if _, err := os.Stat(cfg.SocketPath); err == nil {
				log.Println("removing stale socket file")
				_ = os.Remove(cfg.SocketPath)
			}

			if !foreground {
				// For now, only foreground mode is supported.
				// Background daemonization will be added later.
				fmt.Println("hint: use --foreground to run in the current terminal")
				fmt.Println("background daemonization not yet implemented, running in foreground")
			}

			// Create IPC server first (with nil store -- daemon will set it).
			// We pass nil for daemon too; we need to create daemon first,
			// then set the daemon reference on the server.
			ipcServer := ipc.NewServer(nil, nil, cfg.WatchPaths)

			// Create daemon with the IPC server.
			d := daemon.New(cfg, ipcServer)

			// Now wire the daemon back into the IPC server.
			ipcServer.SetDaemon(d)

			// Start blocks until signal or error.
			return d.Start()
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in the foreground (don't daemonize)")

	return cmd
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the who-wrote-it daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			client := ipc.NewClient(cfg.SocketPath)
			if err := client.RequestStop(); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}

			fmt.Println("daemon stopping")
			return nil
		},
	}
}

func pingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check if daemon is alive",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			client := ipc.NewClient(cfg.SocketPath)
			if err := client.Ping(); err != nil {
				fmt.Println("daemon is not running")
				return err
			}

			fmt.Println("daemon is alive")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			client := ipc.NewClient(cfg.SocketPath)
			status, err := client.Status()
			if err != nil {
				return fmt.Errorf("daemon not running or unreachable: %w", err)
			}

			data, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}
