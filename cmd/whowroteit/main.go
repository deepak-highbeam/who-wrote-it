package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/anthropic/who-wrote-it/internal/config"
	"github.com/anthropic/who-wrote-it/internal/daemon"
	"github.com/anthropic/who-wrote-it/internal/ipc"
	"github.com/anthropic/who-wrote-it/internal/report"
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
	rootCmd.AddCommand(analyzeCmd())

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
	var jsonOutput bool

	cmd := &cobra.Command{
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

			if jsonOutput {
				fmt.Println(report.FormatJSON(status))
			} else {
				fmt.Print(report.FormatStatus(status))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func analyzeCmd() *cobra.Command {
	var (
		filePath   string
		jsonOutput bool
		dbPath     string
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Show attribution report from collected data",
		Long: `Analyze collected attribution data and display a report.

Reads the SQLite database directly -- the daemon does not need to be running.
By default, shows a project-level summary. Use --file for single-file detail.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve DB path: flag > config default.
			if dbPath == "" {
				cfg, err := config.Load(config.ConfigPath())
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				dbPath = cfg.DBPath
			}

			if filePath != "" {
				// Single file analysis.
				fr, err := report.GenerateFile(dbPath, filePath)
				if err != nil {
					return fmt.Errorf("generate file report: %w", err)
				}
				if jsonOutput {
					fmt.Println(report.FormatJSON(fr))
				} else {
					fmt.Print(report.FormatFileReport(fr))
				}
			} else {
				// Full project analysis.
				pr, err := report.GenerateProject(dbPath)
				if err != nil {
					return fmt.Errorf("generate project report: %w", err)
				}
				if jsonOutput {
					fmt.Println(report.FormatJSON(pr))
				} else {
					fmt.Print(report.FormatProjectReport(pr))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Analyze a single file")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&dbPath, "db", "", "Override database path (default: from config)")

	return cmd
}
