package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/anthropic/who-wrote-it/internal/config"
	"github.com/anthropic/who-wrote-it/internal/store"
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
	startTime time.Time

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

	// Stop IPC server first (stops accepting, drains connections).
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
