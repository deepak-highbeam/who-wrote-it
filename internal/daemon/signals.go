package daemon

import (
	"context"
	"os/signal"
	"syscall"
)

// signalContext returns a context that is cancelled when SIGTERM or SIGINT
// is received. The returned stop function must be called to release resources.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGTERM, syscall.SIGINT)
}
