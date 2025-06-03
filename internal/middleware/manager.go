package middleware

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/logging"
)

// Manager manages middleware processes for a provider configuration
type Manager interface {
	// Lifecycle methods

	// Start initializes and starts all middleware processes
	Start(ctx context.Context) error

	// Stop gracefully shuts down all middleware processes
	Stop(ctx context.Context) error

	// Hook methods for resources and data sources

	// PrePlan is called before planning a resource or data source
	PrePlan(ctx context.Context, params PrePlanParams) (*HookResult, error)

	// PostPlan is called after planning a resource or data source
	PostPlan(ctx context.Context, params PostPlanParams) (*HookResult, error)

	// PreApply is called before applying changes to a resource or data source
	PreApply(ctx context.Context, params PreApplyParams) (*HookResult, error)

	// PostApply is called after applying changes to a resource or data source
	PostApply(ctx context.Context, params PostApplyParams) (*HookResult, error)

	// PreRefresh is called before refreshing a resource or data source
	PreRefresh(ctx context.Context, params PreRefreshParams) (*HookResult, error)

	// PostRefresh is called after refreshing a resource or data source
	PostRefresh(ctx context.Context, params PostRefreshParams) (*HookResult, error)
}

// manager implements the Manager interface
type manager struct {
	clients []*Client
	configs []*configs.Middleware
	mu      sync.RWMutex
	started bool
}

var _ Manager = (*manager)(nil) // Ensure manager implements Manager interface

// NewManager creates a new middleware manager
func NewManager(middlewareConfigs []*configs.Middleware) Manager {
	return &manager{
		configs: middlewareConfigs,
		clients: make([]*Client, 0, len(middlewareConfigs)),
	}
}

// Start initializes and starts all middleware processes
func (m *manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	log := logging.HCLogger()
	log.Info("starting middleware", "count", len(m.configs))

	// Create and start clients
	for _, config := range m.configs {
		client, err := NewClient(config.Name, config)
		if err != nil {
			// Clean up any already started clients
			m.stopAllLocked(ctx)
			return fmt.Errorf("failed to create middleware client %q: %w", config.Name, err)
		}

		if err := client.Start(ctx); err != nil {
			// Clean up any already started clients
			m.stopAllLocked(ctx)
			return fmt.Errorf("failed to start middleware %q: %w", config.Name, err)
		}

		m.clients = append(m.clients, client)
	}

	m.started = true
	return nil
}

// Stop gracefully shuts down all middleware processes
func (m *manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopAllLocked(ctx)
}

// stopAllLocked stops all clients (must be called with lock held)
func (m *manager) stopAllLocked(ctx context.Context) error {
	var firstErr error

	for _, client := range m.clients {
		if err := client.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	m.clients = nil
	m.started = false

	return firstErr
}

// PrePlan calls pre-plan hook on all middleware in order
func (m *manager) PrePlan(ctx context.Context, params PrePlanParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// Accumulate metadata from all middleware
	aggregatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		result, err := client.PrePlan(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-plan failed: %w", m.configs[i].Name, err)
		}

		// Check if middleware failed
		if result.Status == "fail" {
			// Return immediately on failure
			return result, nil
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// All middleware passed
	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// PostPlan calls post-plan hook on all middleware in order
func (m *manager) PostPlan(ctx context.Context, params PostPlanParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	aggregatedMetadata := make(map[string]interface{})
	var firstFailure *HookResult

	for i, client := range m.clients {
		result, err := client.PostPlan(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-plan failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Track first failure but continue
		if result.Status == "fail" && firstFailure == nil {
			firstFailure = result
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// Return first failure if any, otherwise success
	if firstFailure != nil {
		firstFailure.Metadata = aggregatedMetadata
		return firstFailure, nil
	}

	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// PreApply calls pre-apply hook on all middleware in order
func (m *manager) PreApply(ctx context.Context, params PreApplyParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// Accumulate metadata from all middleware
	aggregatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		result, err := client.PreApply(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-apply failed: %w", m.configs[i].Name, err)
		}

		// Check if middleware failed
		if result.Status == "fail" {
			// Return immediately on failure
			return result, nil
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// All middleware passed
	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// PostApply calls post-apply hook on all middleware in order
func (m *manager) PostApply(ctx context.Context, params PostApplyParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	aggregatedMetadata := make(map[string]interface{})
	var firstFailure *HookResult

	for i, client := range m.clients {
		result, err := client.PostApply(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-apply failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Track first failure but continue
		if result.Status == "fail" && firstFailure == nil {
			firstFailure = result
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// Return first failure if any, otherwise success
	if firstFailure != nil {
		firstFailure.Metadata = aggregatedMetadata
		return firstFailure, nil
	}

	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// PreRefresh calls pre-refresh hook on all middleware in order
func (m *manager) PreRefresh(ctx context.Context, params PreRefreshParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// Accumulate metadata from all middleware
	aggregatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		result, err := client.PreRefresh(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-refresh failed: %w", m.configs[i].Name, err)
		}

		// Check if middleware failed
		if result.Status == "fail" {
			// Return immediately on failure
			return result, nil
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// All middleware passed
	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// PostRefresh calls post-refresh hook on all middleware in order
func (m *manager) PostRefresh(ctx context.Context, params PostRefreshParams) (*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	aggregatedMetadata := make(map[string]interface{})
	var firstFailure *HookResult

	for i, client := range m.clients {
		result, err := client.PostRefresh(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-refresh failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Track first failure but continue
		if result.Status == "fail" && firstFailure == nil {
			firstFailure = result
		}

		// Accumulate metadata namespaced by middleware name
		if len(result.Metadata) > 0 {
			aggregatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	// Return first failure if any, otherwise success
	if firstFailure != nil {
		firstFailure.Metadata = aggregatedMetadata
		return firstFailure, nil
	}

	return &HookResult{
		Status:   "pass",
		Metadata: aggregatedMetadata,
	}, nil
}

// ValidateMiddleware validates that middleware can be started during init
func ValidateMiddleware(ctx context.Context, ui cli.Ui, middlewareConfigs []*configs.Middleware) hcl.Diagnostics {
	var diags hcl.Diagnostics

	for _, config := range middlewareConfigs {
		client, err := NewClient(config.Name, config)
		if err != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Failed to create middleware",
				Detail:   fmt.Sprintf("Failed to create middleware %q: %s", config.Name, err),
				Subject:  &config.DeclRange,
			})
			continue
		}

		// Try to start and initialize
		if err := client.Start(ctx); err != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Failed to start middleware",
				Detail:   fmt.Sprintf("Failed to start middleware %q: %s", config.Name, err),
				Subject:  &config.DeclRange,
			})
		} else {
			// Send ping to validate communication
			if err := client.Ping(ctx); err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Middleware ping failed",
					Detail:   fmt.Sprintf("Failed to ping middleware %q: %s", config.Name, err),
					Subject:  &config.DeclRange,
				})
			}

			// Stop the middleware
			if err := client.Stop(ctx); err != nil {
				// Log but don't fail validation
				log := logging.HCLogger()
				log.Warn("failed to stop middleware during validation", "name", config.Name, "error", err)
			}
		}
		// log that it was good
		ui.Info(fmt.Sprintf("- %s is valid and ready", config.Name))
	}

	return diags
}
