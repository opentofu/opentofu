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
	// All hook methods return a map of results keyed by middleware name

	// PrePlan is called before planning a resource or data source
	PrePlan(ctx context.Context, params PrePlanParams) (map[string]*HookResult, error)

	// PostPlan is called after planning a resource or data source
	PostPlan(ctx context.Context, params PostPlanParams) (map[string]*HookResult, error)

	// PreApply is called before applying changes to a resource or data source
	PreApply(ctx context.Context, params PreApplyParams) (map[string]*HookResult, error)

	// PostApply is called after applying changes to a resource or data source
	PostApply(ctx context.Context, params PostApplyParams) (map[string]*HookResult, error)

	// PreRefresh is called before refreshing a resource or data source
	PreRefresh(ctx context.Context, params PreRefreshParams) (map[string]*HookResult, error)

	// PostRefresh is called after refreshing a resource or data source
	PostRefresh(ctx context.Context, params PostRefreshParams) (map[string]*HookResult, error)

	// Plan-level hook that receives full plan JSON

	// OnPlanCompleted is called when planning is complete (success or failure)
	OnPlanCompleted(ctx context.Context, params OnPlanCompletedParams) (map[string]*HookResult, error)
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
		log.Debug("creating middleware client", "name", config.Name, "command", config.Command)
		client, err := NewClient(config.Name, config)
		if err != nil {
			log.Error("failed to create middleware client", "name", config.Name, "error", err)
			// Clean up any already started clients
			m.stopAllLocked(ctx)
			return fmt.Errorf("failed to create middleware client %q: %w", config.Name, err)
		}
		log.Debug("middleware client created", "name", config.Name)

		log.Debug("starting middleware client", "name", config.Name)
		if err := client.Start(ctx); err != nil {
			log.Error("failed to start middleware client", "name", config.Name, "error", err)
			// Clean up any already started clients
			m.stopAllLocked(ctx)
			return fmt.Errorf("failed to start middleware %q: %w", config.Name, err)
		}
		log.Debug("middleware client started", "name", config.Name)

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
func (m *manager) PrePlan(ctx context.Context, params PrePlanParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PrePlan(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-plan failed: %w", m.configs[i].Name, err)
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result

		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}

		// Check if middleware failed - pre-hooks stop on failure
		if result.Status == "fail" {
			// Return immediately on failure with results so far
			return results, nil
		}
	}

	// All middleware passed
	return results, nil
}

// PostPlan calls post-plan hook on all middleware in order
func (m *manager) PostPlan(ctx context.Context, params PostPlanParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PostPlan(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-plan failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result
		
		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	return results, nil
}

// PreApply calls pre-apply hook on all middleware in order
func (m *manager) PreApply(ctx context.Context, params PreApplyParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PreApply(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-apply failed: %w", m.configs[i].Name, err)
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result

		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}

		// Check if middleware failed - pre-hooks stop on failure
		if result.Status == "fail" {
			// Return immediately on failure with results so far
			return results, nil
		}
	}

	// All middleware passed
	return results, nil
}

// PostApply calls post-apply hook on all middleware in order
func (m *manager) PostApply(ctx context.Context, params PostApplyParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PostApply(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-apply failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result
		
		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	return results, nil
}

// PreRefresh calls pre-refresh hook on all middleware in order
func (m *manager) PreRefresh(ctx context.Context, params PreRefreshParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PreRefresh(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("middleware %q pre-refresh failed: %w", m.configs[i].Name, err)
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result

		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}

		// Check if middleware failed - pre-hooks stop on failure
		if result.Status == "fail" {
			// Return immediately on failure with results so far
			return results, nil
		}
	}

	// All middleware passed
	return results, nil
}

// PostRefresh calls post-refresh hook on all middleware in order
func (m *manager) PostRefresh(ctx context.Context, params PostRefreshParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	// For post-* hooks, we run all middleware regardless of failures
	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.PostRefresh(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware post-refresh failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result
		
		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	return results, nil
}

// OnPlanCompleted calls plan-completed hook on all middleware in order
func (m *manager) OnPlanCompleted(ctx context.Context, params OnPlanCompletedParams) (map[string]*HookResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return nil, fmt.Errorf("middleware manager not started")
	}

	results := make(map[string]*HookResult)
	accumulatedMetadata := make(map[string]interface{})

	for i, client := range m.clients {
		// Add accumulated metadata to params for chaining
		params.PreviousMiddlewareMetadata = accumulatedMetadata
		
		result, err := client.OnPlanCompleted(ctx, params)
		if err != nil {
			// Log error but continue with other middleware
			log := logging.HCLogger()
			log.Error("middleware plan-completed failed", "name", m.configs[i].Name, "error", err)
			continue
		}

		// Store result keyed by middleware name
		results[m.configs[i].Name] = result
		
		// Accumulate metadata for next middleware
		if result.Metadata != nil {
			accumulatedMetadata[m.configs[i].Name] = result.Metadata
		}
	}

	return results, nil
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
		log := logging.HCLogger()
		log.Debug("validating middleware - starting", "name", config.Name)
		if err := client.Start(ctx); err != nil {
			log.Error("validation failed - could not start middleware", "name", config.Name, "error", err)
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Failed to start middleware",
				Detail:   fmt.Sprintf("Failed to start middleware %q: %s", config.Name, err),
				Subject:  &config.DeclRange,
			})
		} else {
			log.Debug("validating middleware - started, sending ping", "name", config.Name)
			// Send ping to validate communication
			if err := client.Ping(ctx); err != nil {
				log.Error("validation failed - ping failed", "name", config.Name, "error", err)
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Middleware ping failed",
					Detail:   fmt.Sprintf("Failed to ping middleware %q: %s", config.Name, err),
					Subject:  &config.DeclRange,
				})
			} else {
				log.Debug("validating middleware - ping successful", "name", config.Name)
			}

			log.Debug("validating middleware - stopping", "name", config.Name)
			// Stop the middleware
			if err := client.Stop(ctx); err != nil {
				// Log but don't fail validation
				log.Warn("failed to stop middleware during validation", "name", config.Name, "error", err)
			} else {
				log.Debug("validating middleware - stopped", "name", config.Name)
			}
		}
		// log that it was good
		ui.Info(fmt.Sprintf("- %s is valid and ready", config.Name))
	}

	return diags
}
