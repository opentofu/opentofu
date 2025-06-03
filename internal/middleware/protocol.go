// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/zclconf/go-cty/cty"
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
}

type PostPlanParams struct {
	Provider     string            `json:"provider"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	ResourceMode addrs.ResourceMode `json:"resource_mode"`
	Action       plans.Action      `json:"action"`
	Before       cty.Value         `json:"before"`
	After        cty.Value         `json:"after"`
}

type PreApplyParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	PlannedAction plans.Action      `json:"planned_action"`
	PlannedValues cty.Value         `json:"planned_values"`
}

type PostApplyParams struct {
	Provider     string            `json:"provider"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	ResourceMode addrs.ResourceMode `json:"resource_mode"`
	Action       plans.Action      `json:"action"`
	Result       string            `json:"result"` // "success" or "failure"
	State        cty.Value         `json:"state"`
	Error        string            `json:"error,omitempty"`
}

type PreRefreshParams struct {
	Provider     string            `json:"provider"`
	ResourceType string            `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	ResourceMode addrs.ResourceMode `json:"resource_mode"`
	CurrentState cty.Value         `json:"current_state"`
}

type PostRefreshParams struct {
	Provider      string            `json:"provider"`
	ResourceType  string            `json:"resource_type"`
	ResourceName  string            `json:"resource_name"`
	ResourceMode  addrs.ResourceMode `json:"resource_mode"`
	Before        cty.Value         `json:"before"`
	After         cty.Value         `json:"after"`
	DriftDetected bool              `json:"drift_detected"`
}

// HookResult is returned by all hooks
type HookResult struct {
	Status   string                 `json:"status"` // "pass" or "fail"
	Message  string                 `json:"message,omitempty"`
	// Metadata is only persisted to state for post-plan and post-apply hooks
	// Other hooks may return metadata for logging but it won't be saved
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Initialize params and result
type initializeParams struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

type initializeResult struct {
	Capabilities []string `json:"capabilities"`
}