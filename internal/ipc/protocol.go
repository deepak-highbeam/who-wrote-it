package ipc

// Request is a JSON message sent from client to server.
type Request struct {
	Command string            `json:"command"` // "status", "stop", "ping"
	Args    map[string]string `json:"args,omitempty"`
}

// Response is a JSON message sent from server to client.
type Response struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// StatusData is returned by the "status" command.
type StatusData struct {
	Uptime             string   `json:"uptime"`
	DBSizeBytes        int64    `json:"db_size_bytes"`
	FileEventsCount    int64    `json:"file_events_count"`
	SessionEventsCount int64    `json:"session_events_count"`
	GitCommitsCount    int64    `json:"git_commits_count"`
	WatchedPaths       []string `json:"watched_paths"`
}
