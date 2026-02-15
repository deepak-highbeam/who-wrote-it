package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/anthropic/gap-map/internal/config"
	"github.com/anthropic/gap-map/internal/daemon"
	ghub "github.com/anthropic/gap-map/internal/github"
	"github.com/anthropic/gap-map/internal/ipc"
	"github.com/anthropic/gap-map/internal/report"
	"github.com/anthropic/gap-map/internal/store"
	"github.com/anthropic/gap-map/internal/survival"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gapmap",
		Short: "Track human vs AI code authorship",
		Long:  "gap-map is a daemon that monitors your development workflow to attribute code to human or AI authors.",
	}

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(pingCmd())
	rootCmd.AddCommand(analyzeCmd())
	rootCmd.AddCommand(prCommentCmd())
	rootCmd.AddCommand(survivalCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startCmd() *cobra.Command {
	var foreground bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the gap-map daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			pidPath := filepath.Join(cfg.DataDir, "gapmap.pid")

			// The already-running checks only apply to the user-facing
			// entry point (non-foreground). The foreground child skips
			// these because the parent already validated and wrote the
			// PID file for the child's own PID.
			if !foreground {
				// Check if daemon is already running via PID file.
				if data, err := os.ReadFile(pidPath); err == nil {
					if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
						if process, err := os.FindProcess(pid); err == nil {
							if err := process.Signal(syscall.Signal(0)); err == nil {
								fmt.Printf("daemon is already running (pid %d)\n", pid)
								return nil
							}
						}
					}
					// Stale PID file — remove it.
					_ = os.Remove(pidPath)
				}

				// Also check via IPC ping (covers case where PID file is missing).
				client := ipc.NewClient(cfg.SocketPath)
				if err := client.Ping(); err == nil {
					fmt.Println("daemon is already running")
					return nil
				}
			}

			// Remove stale socket file (from a prior crash).
			if _, err := os.Stat(cfg.SocketPath); err == nil {
				log.Println("removing stale socket file")
				_ = os.Remove(cfg.SocketPath)
			}

			if !foreground {
				// Re-exec ourselves with --foreground to daemonize.
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("resolve executable path: %w", err)
				}

				// Ensure data directory exists for log file.
				_ = cfg.EnsureDataDir()

				logPath := filepath.Join(cfg.DataDir, "daemon.log")
				logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					return fmt.Errorf("open daemon log: %w", err)
				}

				child := exec.Command(exe, "start", "--foreground")
				child.Stdin = nil
				child.Stdout = logFile
				child.Stderr = logFile
				child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

				if err := child.Start(); err != nil {
					logFile.Close()
					return fmt.Errorf("start background daemon: %w", err)
				}

				// Parent no longer needs the log file handle.
				logFile.Close()

				childPID := child.Process.Pid
				if err := os.WriteFile(pidPath, []byte(strconv.Itoa(childPID)), 0644); err != nil {
					return fmt.Errorf("write pid file: %w", err)
				}

				// Detach from the child so it won't become a zombie.
				_ = child.Process.Release()

				// Poll IPC ping to confirm child is healthy (up to 5s).
				client := ipc.NewClient(cfg.SocketPath)
				healthy := false
				for i := 0; i < 25; i++ {
					time.Sleep(200 * time.Millisecond)
					if err := client.Ping(); err == nil {
						healthy = true
						break
					}
				}

				if !healthy {
					_ = os.Remove(pidPath)
					return fmt.Errorf("daemon failed to start (check logs)")
				}

				printBanner()
				fmt.Printf("  daemon started (pid %d)\n\n", childPID)
				return nil
			}

			// Foreground mode: run daemon directly.
			// Create IPC server first (with nil store -- daemon will set it).
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
		Short: "Stop the gap-map daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			client := ipc.NewClient(cfg.SocketPath)
			if err := client.RequestStop(); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}

			// Remove PID file in case daemon crashes before its own cleanup.
			_ = os.Remove(filepath.Join(cfg.DataDir, "gapmap.pid"))

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
		branch     string
		baseBranch string
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Show attribution report from collected data",
		Long: `Analyze collected attribution data and display a report.

Reads the SQLite database directly -- the daemon does not need to be running.
By default, shows a project-level summary. Use --file for single-file detail.

Use --branch and --base to scope the report to a specific branch's changes
relative to a base branch (e.g. main). This uses git merge-base to compute
only the lines that changed on the branch.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve DB path: flag > config default.
			if dbPath == "" {
				cfg, err := config.Load(config.ConfigPath())
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				dbPath = cfg.DBPath
			}

			if branch != "" && baseBranch == "" {
				baseBranch = "main"
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
			} else if branch != "" {
				// Branch-scoped analysis.
				s, err := store.New(dbPath)
				if err != nil {
					return fmt.Errorf("open store: %w", err)
				}
				defer s.Close()

				pr, err := report.GenerateProjectForBranch(s, branch, baseBranch)
				if err != nil {
					return fmt.Errorf("generate branch report: %w", err)
				}
				if jsonOutput {
					fmt.Println(report.FormatJSON(pr))
				} else {
					fmt.Print(report.FormatProjectReport(pr))
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
	cmd.Flags().StringVar(&branch, "branch", "", "Scope report to a specific branch")
	cmd.Flags().StringVar(&baseBranch, "base", "", "Base branch for comparison (default: main)")

	return cmd
}

func prCommentCmd() *cobra.Command {
	var (
		token  string
		pr     int
		owner  string
		repo   string
		dbPath string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "pr-comment",
		Short: "Post a collaboration summary comment to a GitHub PR",
		Long: `Generate and post a collaboration summary as a comment on a GitHub PR.

The comment includes authorship breakdown by work type, insight callouts,
and per-file collaboration patterns for notable files.

Use --dry-run to preview the Markdown without posting.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve DB path.
			if dbPath == "" {
				cfg, err := config.Load(config.ConfigPath())
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				dbPath = cfg.DBPath
			}

			// Generate the project report.
			projectReport, err := report.GenerateProject(dbPath)
			if err != nil {
				return fmt.Errorf("generate project report: %w", err)
			}

			// Generate the comment body.
			body := ghub.GenerateComment(projectReport)

			// Dry run: print and exit.
			if dryRun {
				fmt.Println(body)
				return nil
			}

			// Resolve token.
			if token == "" {
				token = os.Getenv("GITHUB_TOKEN")
			}
			if token == "" {
				return fmt.Errorf("GitHub token required: set --token flag or GITHUB_TOKEN env var")
			}

			// Auto-detect owner/repo from git remote if not provided.
			if owner == "" || repo == "" {
				remoteURL, err := detectRemoteURL()
				if err != nil {
					return fmt.Errorf("auto-detect remote (set --owner and --repo flags): %w", err)
				}
				detectedOwner, detectedRepo, err := ghub.ParseGitHubRemote(remoteURL)
				if err != nil {
					return fmt.Errorf("parse remote URL %q: %w", remoteURL, err)
				}
				if owner == "" {
					owner = detectedOwner
				}
				if repo == "" {
					repo = detectedRepo
				}
			}

			// Auto-detect PR number if not provided.
			if pr == 0 {
				detected, err := ghub.DetectPRNumber()
				if err != nil {
					return fmt.Errorf("auto-detect PR number: %w", err)
				}
				pr = detected
			}

			// Post the comment.
			if err := ghub.PostComment(owner, repo, pr, body, token); err != nil {
				return fmt.Errorf("post comment: %w", err)
			}

			fmt.Printf("Comment posted to PR #%d\n", pr)
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "GitHub token (default: GITHUB_TOKEN env var)")
	cmd.Flags().IntVar(&pr, "pr", 0, "PR number (default: auto-detect)")
	cmd.Flags().StringVar(&owner, "owner", "", "Repository owner (default: auto-detect from git remote)")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name (default: auto-detect from git remote)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Override database path (default: from config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print comment body without posting")

	return cmd
}

func survivalCmd() *cobra.Command {
	var (
		dbPath     string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "survival",
		Short: "Show AI code survival rates",
		Long: `Analyze how much AI-written code survives across subsequent commits.

Compares AI attributions against current git blame data to measure
code persistence by authorship level and work type.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve DB path.
			if dbPath == "" {
				cfg, err := config.Load(config.ConfigPath())
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				dbPath = cfg.DBPath
			}

			// Open store and discover project path.
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()

			projectPath, err := discoverProjectPath(s)
			if err != nil {
				return fmt.Errorf("discover project: %w", err)
			}

			// Run survival analysis.
			sr, err := survival.Analyze(s, projectPath)
			if err != nil {
				return fmt.Errorf("survival analysis: %w", err)
			}

			if jsonOutput {
				fmt.Println(report.FormatJSON(sr))
			} else {
				// Convert to the github package's type for formatting.
				formatted := &ghub.SurvivalReport{
					TotalTracked:  sr.TotalTracked,
					SurvivedCount: sr.SurvivedCount,
					SurvivalRate:  sr.SurvivalRate,
					ByAuthorship:  make(map[string]ghub.SurvivalBreakdown),
					ByWorkType:    make(map[string]ghub.SurvivalBreakdown),
				}
				for k, v := range sr.ByAuthorship {
					formatted.ByAuthorship[k] = ghub.SurvivalBreakdown{
						Tracked: v.Tracked, Survived: v.Survived, Rate: v.Rate,
					}
				}
				for k, v := range sr.ByWorkType {
					formatted.ByWorkType[k] = ghub.SurvivalBreakdown{
						Tracked: v.Tracked, Survived: v.Survived, Rate: v.Rate,
					}
				}
				fmt.Print(ghub.FormatSurvivalReport(formatted))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Override database path (default: from config)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

// discoverProjectPath finds the project path from the attributions table.
func discoverProjectPath(s *store.Store) (string, error) {
	rows, err := s.DB().Query("SELECT DISTINCT project_path FROM attributions ORDER BY project_path LIMIT 1")
	if err != nil {
		return "", fmt.Errorf("discover project path: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return "", fmt.Errorf("no attribution data found in database")
	}

	var path string
	if err := rows.Scan(&path); err != nil {
		return "", fmt.Errorf("scan project path: %w", err)
	}

	return path, rows.Err()
}

func printBanner() {
	const purple = "\033[38;5;135m"
	const dim = "\033[38;5;99m"
	const bold = "\033[1m"
	const reset = "\033[0m"

	fmt.Print(purple + bold + `

   __ _   __ _  _ __   _ __ ___    __ _  _ __
  / _` + "`" + ` | / _` + "`" + ` || '_ \ | '_ ` + "`" + ` _ \  / _` + "`" + ` || '_ \
 | (_| || (_| || |_) || | | | | || (_| || |_) |
  \__, | \__,_|| .__/ |_| |_| |_| \__,_|| .__/
  |___/        |_|                       |_|
` + reset + dim + `
    Map your knowledge gaps
` + reset + "\n")
}

// detectRemoteURL runs `git remote get-url origin` to get the remote URL.
func detectRemoteURL() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("get git remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
