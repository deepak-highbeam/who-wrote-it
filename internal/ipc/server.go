package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// DaemonQuerier is the interface the IPC server uses to query daemon state.
// This avoids importing the daemon package (which would be circular).
type DaemonQuerier interface {
	Uptime() time.Duration
	Stop()
}

// StoreQuerier provides data access methods needed by the IPC server.
type StoreQuerier interface {
	FileEventsCount() (int64, error)
	SessionEventsCount() (int64, error)
	GitCommitsCount() (int64, error)
	DBSizeBytes() (int64, error)
}

// Server is a Unix domain socket server for CLI-to-daemon communication.
type Server struct {
	daemon   DaemonQuerier
	store    StoreQuerier
	watchPaths []string

	listener net.Listener
	mu       sync.Mutex
	wg       sync.WaitGroup
	stopped  bool
}

// NewServer creates a new IPC server.
func NewServer(daemon DaemonQuerier, store StoreQuerier, watchPaths []string) *Server {
	return &Server{
		daemon:     daemon,
		store:      store,
		watchPaths: watchPaths,
	}
}

// Listen starts accepting connections on the given Unix socket path.
// It blocks until the context is cancelled or an error occurs.
func (s *Server) Listen(socketPath string, ctx context.Context) error {
	// Remove stale socket file if it exists.
	if _, err := os.Stat(socketPath); err == nil {
		_ = os.Remove(socketPath)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", socketPath, err)
	}

	// Set socket permissions to owner-only.
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	s.mu.Lock()
	s.listener = ln
	s.stopped = false
	s.mu.Unlock()

	log.Printf("IPC server listening on %s", socketPath)

	// Close the listener when context is cancelled.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			stopped := s.stopped
			s.mu.Unlock()
			if stopped {
				return nil
			}
			// Context cancelled causes listener to close.
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Stop stops accepting connections and waits for in-flight connections to drain.
func (s *Server) Stop() error {
	s.mu.Lock()
	s.stopped = true
	ln := s.listener
	s.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}

	// Wait for in-flight connections with a timeout.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("drain timeout: connections still open after 5s")
	}
}

// SetStore updates the store reference after daemon startup.
// Accepts interface{} to satisfy daemon.StoreAware without circular imports.
// The concrete value must implement StoreQuerier.
func (s *Server) SetStore(st interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sq, ok := st.(StoreQuerier); ok {
		s.store = sq
	}
}

// SetDaemon sets the daemon reference. This is called after daemon creation
// to break the circular construction dependency (daemon needs server, server needs daemon).
func (s *Server) SetDaemon(d DaemonQuerier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.daemon = d
}

// handleConn reads a single JSON request, dispatches it, and writes the response.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	// Set a read/write deadline.
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		writeError(conn, "empty request")
		return
	}

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeError(conn, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	switch req.Command {
	case "ping":
		writeResponse(conn, Response{OK: true, Data: "pong"})

	case "status":
		s.handleStatus(conn)

	case "stop":
		writeResponse(conn, Response{OK: true, Data: "shutting down"})
		// Trigger daemon shutdown after sending response.
		if s.daemon != nil {
			s.daemon.Stop()
		}

	default:
		writeError(conn, fmt.Sprintf("unknown command: %q", req.Command))
	}
}

func (s *Server) handleStatus(conn net.Conn) {
	data := StatusData{
		WatchedPaths: s.watchPaths,
	}

	if s.daemon != nil {
		data.Uptime = s.daemon.Uptime().Truncate(time.Second).String()
	}

	if s.store != nil {
		if v, err := s.store.DBSizeBytes(); err == nil {
			data.DBSizeBytes = v
		}
		if v, err := s.store.FileEventsCount(); err == nil {
			data.FileEventsCount = v
		}
		if v, err := s.store.SessionEventsCount(); err == nil {
			data.SessionEventsCount = v
		}
		if v, err := s.store.GitCommitsCount(); err == nil {
			data.GitCommitsCount = v
		}
	}

	writeResponse(conn, Response{OK: true, Data: data})
}

func writeResponse(conn net.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = conn.Write(data)
}

func writeError(conn net.Conn, msg string) {
	writeResponse(conn, Response{OK: false, Error: msg})
}
