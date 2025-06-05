// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"encoding/json"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

// JSON-RPC message types
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
	ID      int         `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Hook parameter types
type PrePlanParams struct {
	Provider     string            `json:"provider"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	ResourceMode addrs.ResourceMode `json:"resource_mode"`
	Config       cty.Value         `json:"config"`
	CurrentState cty.Value         `json:"current_state"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

type PostPlanParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	CurrentState  cty.Value         `json:"current_state"`
	PlannedState  cty.Value         `json:"planned_state"`
	Config        cty.Value         `json:"config"`
	PlannedAction string            `json:"planned_action"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

type PreApplyParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	CurrentState  cty.Value         `json:"current_state"`
	PlannedState  cty.Value         `json:"planned_state"`
	Config        cty.Value         `json:"config"`
	PlannedAction string            `json:"planned_action"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

type PostApplyParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	Before        cty.Value         `json:"before"`
	After         cty.Value         `json:"after"`
	Config        cty.Value         `json:"config"`
	AppliedAction string            `json:"applied_action"`
	Failed        bool              `json:"failed"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

type PreRefreshParams struct {
	Provider     string            `json:"provider"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	ResourceMode addrs.ResourceMode `json:"resource_mode"`
	CurrentState cty.Value         `json:"current_state"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

type PostRefreshParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	Before        cty.Value         `json:"before"`
	After         cty.Value         `json:"after"`
	DriftDetected bool              `json:"drift_detected"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}

// HookResult is returned by all hooks
type HookResult struct {
	Status   string                 `json:"status"` // "pass" or "fail"
	Message  string                 `json:"message,omitempty"`
	// Metadata is only persisted to state for post-plan and post-apply hooks
	// Other hooks may return metadata for logging but it won't be saved
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// ModifiedConfig is only used by pre-plan hooks to allow modifying the configuration
	ModifiedConfig map[string]interface{} `json:"modified_config,omitempty"`
}

// Initialize params and result
type initializeParams struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

type initializeResult struct {
	Capabilities []string `json:"capabilities"`
}

// Plan-level hook parameters - receives the full plan JSON
type OnPlanCompletedParams struct {
	// Full plan JSON - contains the complete jsonplan.Plan structure as JSON
	PlanJSON json.RawMessage `json:"plan_json"`
	// Whether the plan was successful
	Success bool `json:"success"`
	// Any errors that occurred during planning
	Errors []string `json:"errors,omitempty"`
	// Accumulated metadata from previous middleware in the chain
	PreviousMiddlewareMetadata map[string]interface{} `json:"previous_middleware_metadata,omitempty"`
}