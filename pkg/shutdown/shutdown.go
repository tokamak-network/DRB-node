package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Manager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	timeout    time.Duration
	shutdownCh chan struct{}
}

func NewManager(timeout time.Duration) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:        ctx,
		cancel:     cancel,
		timeout:    timeout,
		shutdownCh: make(chan struct{}),
	}
}

func (m *Manager) Context() context.Context {
	return m.ctx
}

// Wait blocks until a shutdown signal is received (SIGINT or SIGTERM)
// When a signal is received, it cancels the context and returns
func (m *Manager) Wait() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigCh
	log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

	// Cancel the context to signal all goroutines to stop
	m.cancel()

	// Close the shutdown channel to notify that shutdown has started
	close(m.shutdownCh)
}
