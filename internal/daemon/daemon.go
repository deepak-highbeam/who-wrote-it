package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/anthropic/who-wrote-it/internal/authorship"
	"github.com/anthropic/who-wrote-it/internal/config"
	"github.com/anthropic/who-wrote-it/internal/correlation"
	"github.com/anthropic/who-wrote-it/internal/gitint"
	"github.com/anthropic/who-wrote-it/internal/sessionparser"
	"github.com/anthropic/who-wrote-it/internal/store"
	"github.com/anthropic/who-wrote-it/internal/watcher"
	"github.com/anthropic/who-wrote-it/internal/worktype"
)

// IPCServer is the interface the daemon uses to start/stop the IPC listener.
// This avoids a circular dependency with the ipc package.
type IPCServer interface {
	Listen(socketPath string, ctx context.Context) error
	Stop() error
}

// StoreAware can receive a store reference after it becomes available.
type StoreAware interface {
	SetStore(store interface{})
}

// Daemon manages the lifecycle of the who-wrote-it background process.
type Daemon struct {
	cfg       *config.Config
	store     *store.Store
	ipc       IPCServer
	watcher   *watcher.Watcher
	startTime time.Time

	sessionParser *sessionparser.ClaudeCodeParser
	gitRepo       *gitint.Repository
	sessionCancel context.CancelFunc
	gitCancel     context.CancelFunc
	attrCancel    context.CancelFunc

	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
}

// New creates a new Daemon with the given config.
// The IPC server is injected to avoid circular imports.
func New(cfg *config.Config, ipcServer IPCServer) *Daemon {
	return &Daemon{
		cfg: cfg,
		ipc: ipcServer,
	}
}

// Start initialises the store, runs migrations, starts the IPC server,
// and blocks until the context is cancelled (via signal or Stop).
func (d *Daemon) Start() error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon is already running")
	}
	d.mu.Unlock()

	// Ensure data directory exists.
	if err := d.cfg.EnsureDataDir(); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Open store (runs migrations).
	s, err := store.New(d.cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	d.store = s

	// If the IPC server is StoreAware, give it the store reference.
	if sa, ok := d.ipc.(StoreAware); ok {
		sa.SetStore(s)
	}

	// Create a signal-aware context.
	ctx, cancel := signalContext(context.Background())
	d.ctx = ctx
	d.cancel = cancel
	d.startTime = time.Now()

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	// Start IPC server in a goroutine.
	ipcErrCh := make(chan error, 1)
	go func() {
		ipcErrCh <- d.ipc.Listen(d.cfg.SocketPath, d.ctx)
	}()

	// Start file system watcher if watch paths are configured.
	if len(d.cfg.WatchPaths) > 0 {
		d.watcher = watcher.New(s, d.cfg)
		go func() {
			if err := d.watcher.Start(d.ctx); err != nil {
				log.Printf("watcher error: %v", err)
			}
		}()
	}

	// --- Session parser integration ---
	// Discover existing Claude Code session files and start tailing them.
	d.sessionParser = sessionparser.NewClaudeCodeParser("", 0)
	sessionFiles, err := d.sessionParser.Discover(d.ctx)
	if err != nil {
		log.Printf("session discover error: %v", err)
	}
	// discovered session files silently

	sessionCtx, sessionCancel := context.WithCancel(d.ctx)
	d.sessionCancel = sessionCancel

	for _, sf := range sessionFiles {
		d.startSessionTailer(sessionCtx, sf)
	}

	// Watch for new session files (e.g. session rotation).
	newSessions := make(chan sessionparser.SessionFile, 10)
	go func() {
		if err := d.sessionParser.WatchForNew(sessionCtx, newSessions); err != nil {
			log.Printf("session watcher error: %v", err)
		}
	}()
	go func() {
		for {
			select {
			case <-sessionCtx.Done():
				return
			case sf := <-newSessions:
				d.startSessionTailer(sessionCtx, sf)
			}
		}
	}()

	// --- Git integration ---
	// Open the git repository at the first watch path and start periodic sync.
	if len(d.cfg.WatchPaths) > 0 {
		repo, err := gitint.Open(d.cfg.WatchPaths[0], d.store)
		if err != nil {
			log.Printf("git open warning (not a git repo?): %v", err)
		} else {
			d.gitRepo = repo

			gitCtx, gitCancel := context.WithCancel(d.ctx)
			d.gitCancel = gitCancel

			// Initial sync: look back 30 days.
			if err := repo.SyncCommits(gitCtx, time.Now().Add(-gitint.DefaultLookback())); err != nil {
				log.Printf("git initial sync error: %v", err)
			}

			// Periodic sync goroutine.
			go func() {
				ticker := time.NewTicker(gitint.SyncInterval())
				defer ticker.Stop()
				for {
					select {
					case <-gitCtx.Done():
						return
					case <-ticker.C:
						since := time.Now().Add(-gitint.DefaultLookback())
						if err := repo.SyncCommits(gitCtx, since); err != nil {
							log.Printf("git sync error: %v", err)
						}
					}
				}
			}()
		}
	}

	// --- Attribution processor ---
	// Background goroutine that processes file events into attributions
	// by running the correlation engine, authorship classifier, and work-type
	// classifier on each unprocessed file event.
	attrCtx, attrCancel := context.WithCancel(d.ctx)
	d.attrCancel = attrCancel
	d.startAttributionProcessor(attrCtx)

	log.Printf("daemon started (pid %d, db %s, socket %s)", os.Getpid(), d.cfg.DBPath, d.cfg.SocketPath)

	// Block until context is cancelled or IPC server fails.
	select {
	case <-d.ctx.Done():
		log.Println("shutdown signal received")
	case err := <-ipcErrCh:
		if err != nil {
			log.Printf("IPC server error: %v", err)
		}
	}

	// Clean shutdown.
	return d.shutdown()
}

// Stop triggers a graceful shutdown from outside (e.g. via IPC stop command).
func (d *Daemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
}

// shutdown performs ordered teardown: IPC server, then store, then socket cleanup.
func (d *Daemon) shutdown() error {
	log.Println("shutting down...")

	// Cancel session tailers first (allows offset persistence before store closes).
	if d.sessionCancel != nil {
		d.sessionCancel()
	}

	// Cancel git sync goroutine.
	if d.gitCancel != nil {
		d.gitCancel()
	}

	// Cancel attribution processor (after session and git have flushed).
	if d.attrCancel != nil {
		d.attrCancel()
	}

	// Stop watcher (drains pending debounced events to store).
	if d.watcher != nil {
		d.watcher.Stop()
	}

	// Stop IPC server (stops accepting, drains connections).
	if d.ipc != nil {
		if err := d.ipc.Stop(); err != nil {
			log.Printf("ipc stop: %v", err)
		}
	}

	// Close the store.
	if d.store != nil {
		if err := d.store.Close(); err != nil {
			log.Printf("store close: %v", err)
		}
	}

	// Remove socket file.
	_ = os.Remove(d.cfg.SocketPath)

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	log.Println("daemon stopped")
	return nil
}

// Running returns true if the daemon is currently running.
func (d *Daemon) Running() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// Store returns the daemon's data store (for use by IPC handlers).
func (d *Daemon) Store() *store.Store {
	return d.store
}

// Uptime returns how long the daemon has been running.
func (d *Daemon) Uptime() time.Duration {
	if d.startTime.IsZero() {
		return 0
	}
	return time.Since(d.startTime)
}

// Config returns the daemon's configuration.
func (d *Daemon) Config() *config.Config {
	return d.cfg
}

// startSessionTailer starts a goroutine that tails a single session file,
// parsing each line and storing events. It resumes from the last persisted
// offset for the file.
func (d *Daemon) startSessionTailer(ctx context.Context, sf sessionparser.SessionFile) {
	// Restore offset from daemon_state for resume across daemon restarts.
	offsetKey := "tailer_offset:" + sf.Path
	offsetStr, _ := d.store.GetDaemonState(offsetKey)
	var offset int64
	if offsetStr != "" {
		offset, _ = strconv.ParseInt(offsetStr, 10, 64)
	}

	tailer := sessionparser.NewTailer(sf.Path, offset, 0)
	lines := make(chan []byte, 100)

	go func() {
		finalOffset, err := tailer.Tail(ctx, lines)
		if err != nil {
			log.Printf("session tailer %s error: %v", sf.Path, err)
		}
		// Persist final offset for resume.
		_ = d.store.SetDaemonState(offsetKey, strconv.FormatInt(finalOffset, 10))
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case line := <-lines:
				event, err := d.sessionParser.ParseLine(line)
				if err != nil {
					log.Printf("session parse error: %v", err)
					continue
				}
				if event == nil {
					continue
				}
				event.SessionID = sf.SessionID
				if err := d.store.InsertSessionEvent(
					event.SessionID, event.EventType, event.ToolName,
					event.FilePath, event.ContentHash, event.Timestamp, event.RawJSON,
				); err != nil {
					log.Printf("session store error: %v", err)
				}
			}
		}
	}()

	// tailing session silently
}

// startAttributionProcessor runs a background goroutine that periodically
// queries for unprocessed file events and runs them through the full
// attribution pipeline: correlation -> authorship classification ->
// work-type classification -> store.
func (d *Daemon) startAttributionProcessor(ctx context.Context) {
	correlator := correlation.New(d.store)
	classifier := authorship.NewClassifier()
	wtClassifier := worktype.NewClassifier(d.store)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, err := d.store.QueryUnprocessedFileEvents(100)
				if err != nil {
					log.Printf("attribution: query error: %v", err)
					continue
				}
				if len(events) == 0 {
					continue
				}

				for _, fe := range events {
					// Step 1: Correlate file event with session events.
					result, err := correlator.CorrelateFileEvent(fe)
					if err != nil {
						log.Printf("attribution: correlate error for %s: %v", fe.FilePath, err)
						continue
					}

					// Step 2: Classify authorship level (with history for mixed attributions).
					var prior *authorship.Attribution
					if priorRecord, err := d.store.QueryLatestAttributionByFile(fe.FilePath); err == nil && priorRecord != nil {
						prior = &authorship.Attribution{
							FirstAuthor: priorRecord.FirstAuthor,
							Level:       authorship.AuthorshipLevel(priorRecord.AuthorshipLevel),
						}
					}
					attr := classifier.ClassifyWithHistory(*result, prior)

					// Step 3: Classify work type (empty diff/commit -- path heuristics still work).
					wt := wtClassifier.ClassifyFile(attr.FilePath, "", "")

					// Step 4: Build store record and persist.
					record := store.AttributionRecord{
						FilePath:            attr.FilePath,
						ProjectPath:         attr.ProjectPath,
						FileEventID:         attr.FileEventID,
						SessionEventID:      attr.SessionEventID,
						AuthorshipLevel:     string(attr.Level),
						Confidence:          attr.Confidence,
						Uncertain:           attr.Uncertain,
						FirstAuthor:         attr.FirstAuthor,
						CorrelationWindowMs: attr.CorrelationWindowMs,
						Timestamp:           attr.Timestamp,
					}

					id, err := d.store.InsertAttribution(record)
					if err != nil {
						log.Printf("attribution: insert error for %s: %v", fe.FilePath, err)
						continue
					}

					// Step 5: Set work type on the attribution record.
					if id > 0 {
						if err := d.store.UpdateAttributionWorkType(id, string(wt)); err != nil {
							log.Printf("attribution: update work type error for %s: %v", fe.FilePath, err)
						}
					}
				}

				// processed silently
			}
		}
	}()
}
