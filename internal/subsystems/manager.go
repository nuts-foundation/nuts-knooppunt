package subsystems

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/internal/proxy"
	"github.com/rs/zerolog/log"
)

// Manager manages multiple subsystems
type Manager struct {
	subsystems []Subsystem
	proxies    map[string]*proxy.Handler
	mu         sync.RWMutex
}

// NewManager creates a new subsystem manager
func NewManager() *Manager {
	return &Manager{
		subsystems: make([]Subsystem, 0),
		proxies:    make(map[string]*proxy.Handler),
	}
}

// Register registers a subsystem with the manager
func (m *Manager) Register(subsystem Subsystem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().
		Str("name", subsystem.Name()).
		Str("prefix", subsystem.RoutePrefix()).
		Msg("Registering subsystem")

	m.subsystems = append(m.subsystems, subsystem)

	// Create proxy handler for the subsystem
	proxyHandler, err := proxy.NewHandler(subsystem.RoutePrefix(), subsystem.PublicAddress())
	if err != nil {
		return fmt.Errorf("failed to create proxy for subsystem %s: %w", subsystem.Name(), err)
	}

	m.proxies[subsystem.RoutePrefix()] = proxyHandler

	return nil
}

// Start starts all registered subsystems
func (m *Manager) Start(ctx context.Context) error {
	log.Info().Msg("Starting all subsystems")

	for _, subsystem := range m.subsystems {
		if err := subsystem.Start(ctx); err != nil {
			return fmt.Errorf("failed to start subsystem %s: %w", subsystem.Name(), err)
		}
	}

	log.Info().Int("count", len(m.subsystems)).Msg("All subsystems started")
	return nil
}

// Stop stops all registered subsystems
func (m *Manager) Stop(ctx context.Context) error {
	log.Info().Msg("Stopping all subsystems")

	for _, subsystem := range m.subsystems {
		if err := subsystem.Stop(ctx); err != nil {
			log.Error().Err(err).Str("name", subsystem.Name()).Msg("Error stopping subsystem")
		}
	}

	log.Info().Msg("All subsystems stopped")
	return nil
}

// CreateHandler creates an HTTP handler that routes requests to appropriate subsystems
func (m *Manager) CreateHandler() http.Handler {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mux := http.NewServeMux()

	// Register proxy handlers for each subsystem
	for prefix, proxyHandler := range m.proxies {
		log.Info().Str("prefix", prefix).Msg("Registering route for subsystem")
		mux.Handle(prefix+"/", proxyHandler)
	}

	return mux
}