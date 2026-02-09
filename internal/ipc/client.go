package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client communicates with the daemon over a Unix domain socket.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a new IPC client that connects to the given socket path.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    5 * time.Second,
	}
}

// Ping tests if the daemon is alive.
func (c *Client) Ping() error {
	_, err := c.send(Request{Command: "ping"})
	return err
}

// Status returns the daemon's status data.
func (c *Client) Status() (*StatusData, error) {
	resp, err := c.send(Request{Command: "status"})
	if err != nil {
		return nil, err
	}

	// resp.Data is a map[string]interface{} from JSON unmarshal.
	// Re-marshal and unmarshal into StatusData.
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal status data: %w", err)
	}

	var status StatusData
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, fmt.Errorf("unmarshal status data: %w", err)
	}

	return &status, nil
}

// RequestStop asks the daemon to shut down gracefully.
func (c *Client) RequestStop() error {
	_, err := c.send(Request{Command: "stop"})
	return err
}

// send dials the socket, sends a JSON request, reads the JSON response.
func (c *Client) send(req Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	// Send request.
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Read response.
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("empty response from daemon")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !resp.OK {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	return &resp, nil
}
