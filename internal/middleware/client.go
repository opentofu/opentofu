// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/logging"
)

// Client represents a single middleware process
type Client struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	
	encoder *json.Encoder
	
	nextID  int32
	mu      sync.Mutex
	
	// For logging stderr
	stderrScanner *bufio.Scanner
	done          chan struct{}
}

// NewClient creates a new middleware client
func NewClient(name string, config *configs.Middleware) (*Client, error) {
	// Evaluate the command expression using the static evaluator
	cmdStr, diags := config.EvaluateCommand()
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate command: %s", diags.Error())
	}
	
	// Evaluate args using the static evaluator
	args, argDiags := config.EvaluateArgs()
	if argDiags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate args: %s", argDiags.Error())
	}
	
	// Create the command
	cmd := exec.Command(cmdStr, args...)
	
	// Set environment variables
	if len(config.Env) > 0 {
		env := cmd.Environ()
		for k, v := range config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}
	
	return &Client{
		name: name,
		cmd:  cmd,
		done: make(chan struct{}),
	}, nil
}

// Start starts the middleware process
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Set up pipes
	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	c.stderr, err = c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	
	// Set up JSON encoder
	c.encoder = json.NewEncoder(c.stdin)
	
	// Start stderr logger
	c.stderrScanner = bufio.NewScanner(c.stderr)
	go c.logStderr()
	
	// Start the process
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start middleware %q: %w", c.name, err)
	}
	
	// Send initialize message
	var result initializeResult
	if err := c.call(ctx, "initialize", initializeParams{
		Version: "1.0",
		Name:    c.name,
	}, &result); err != nil {
		c.Stop(context.Background())
		return fmt.Errorf("failed to initialize middleware %q: %w", c.name, err)
	}
	
	log := logging.HCLogger()
	log.Info("middleware initialized", "name", c.name, "capabilities", result.Capabilities)
	
	return nil
}

// Stop stops the middleware process
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	
	// Try to send shutdown message
	_ = c.callNoResult(ctx, "shutdown", nil)
	
	// Close stdin to signal we're done
	if c.stdin != nil {
		c.stdin.Close()
	}
	
	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()
	
	select {
	case <-ctx.Done():
		// Force kill if context cancelled
		c.cmd.Process.Kill()
		return ctx.Err()
	case err := <-done:
		close(c.done)
		return err
	}
}

// call makes a JSON-RPC call and waits for the response
func (c *Client) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	id := int(atomic.AddInt32(&c.nextID, 1))
	
	// Create request
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	
	// Send request
	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	
	// Create a new decoder for this specific response to avoid state issues
	decoder := json.NewDecoder(c.stdout)
	
	// Read response
	var resp jsonRPCResponse
	if err := decoder.Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Check ID matches
	if resp.ID != id {
		return fmt.Errorf("response ID mismatch: expected %d, got %d", id, resp.ID)
	}
	
	// Check for error
	if resp.Error != nil {
		return fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	
	// Decode result
	if result != nil && resp.Result != nil {
		// resp.Result is likely a map[string]interface{} or json.RawMessage
		// We need to marshal it back to JSON then unmarshal to the target type
		resultJSON, err := json.Marshal(resp.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		if err := json.Unmarshal(resultJSON, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}
	
	return nil
}

// callNoResult makes a JSON-RPC call without expecting a result
func (c *Client) callNoResult(ctx context.Context, method string, params interface{}) error {
	var result interface{}
	return c.call(ctx, method, params, &result)
}

// logStderr logs stderr output from the middleware process
func (c *Client) logStderr() {
	log := logging.HCLogger()
	for c.stderrScanner.Scan() {
		log.Debug("middleware stderr", "name", c.name, "output", c.stderrScanner.Text())
	}
	if err := c.stderrScanner.Err(); err != nil {
		log.Error("middleware stderr scanner error", "name", c.name, "error", err)
	}
}

// Hook methods that convert between cty.Value and JSON

func (c *Client) PrePlan(ctx context.Context, params PrePlanParams) (*HookResult, error) {
	// Convert cty.Values to JSON
	var configJSON, currentJSON json.RawMessage
	
	if !params.Config.IsNull() {
		// For config, we need to handle unknown values by converting them to null
		configWithoutUnknowns := cty.UnknownAsNull(params.Config)
		c, err := ctyjson.Marshal(configWithoutUnknowns, configWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		configJSON = c
	}
	
	if !params.CurrentState.IsNull() {
		// Current state should not have unknowns, but handle them just in case
		stateWithoutUnknowns := cty.UnknownAsNull(params.CurrentState)
		s, err := ctyjson.Marshal(stateWithoutUnknowns, stateWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current state: %w", err)
		}
		currentJSON = s
	}
	
	rpcParams := map[string]interface{}{
		"provider":      params.Provider,
		"resource_type": params.ResourceType,
		"resource_name": params.ResourceName,
		"resource_mode": string(params.ResourceMode),
		"config":        configJSON,
		"current_state": currentJSON,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	var result HookResult
	if err := c.call(ctx, "pre-plan", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (c *Client) PostPlan(ctx context.Context, params PostPlanParams) (*HookResult, error) {
	// Convert cty.Values to JSON
	var currentJSON, plannedJSON, configJSON json.RawMessage
	
	if !params.CurrentState.IsNull() {
		b, err := ctyjson.Marshal(params.CurrentState, params.CurrentState.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current state: %w", err)
		}
		currentJSON = b
	}
	
	if !params.PlannedState.IsNull() {
		// Planned state might have unknowns
		plannedWithoutUnknowns := cty.UnknownAsNull(params.PlannedState)
		a, err := ctyjson.Marshal(plannedWithoutUnknowns, plannedWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal planned state: %w", err)
		}
		plannedJSON = a
	}
	
	if !params.Config.IsNull() {
		// For config, we need to handle unknown values by converting them to null
		configWithoutUnknowns := cty.UnknownAsNull(params.Config)
		c, err := ctyjson.Marshal(configWithoutUnknowns, configWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		configJSON = c
	}
	
	rpcParams := map[string]interface{}{
		"provider":       params.Provider,
		"resource_type":  params.ResourceType,
		"resource_name":  params.ResourceName,
		"resource_mode":  string(params.ResourceMode),
		"current_state":  currentJSON,
		"planned_state":  plannedJSON,
		"config":         configJSON,
		"planned_action": params.PlannedAction,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	var result HookResult
	if err := c.call(ctx, "post-plan", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (c *Client) PreApply(ctx context.Context, params PreApplyParams) (*HookResult, error) {
	// Convert cty.Values to JSON
	var currentJSON, plannedJSON, configJSON json.RawMessage
	
	if !params.CurrentState.IsNull() {
		b, err := ctyjson.Marshal(params.CurrentState, params.CurrentState.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current state: %w", err)
		}
		currentJSON = b
	}
	
	if !params.PlannedState.IsNull() {
		// Planned state might have unknowns
		plannedWithoutUnknowns := cty.UnknownAsNull(params.PlannedState)
		a, err := ctyjson.Marshal(plannedWithoutUnknowns, plannedWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal planned state: %w", err)
		}
		plannedJSON = a
	}
	
	if !params.Config.IsNull() {
		// For config, we need to handle unknown values by converting them to null
		configWithoutUnknowns := cty.UnknownAsNull(params.Config)
		c, err := ctyjson.Marshal(configWithoutUnknowns, configWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		configJSON = c
	}
	
	rpcParams := map[string]interface{}{
		"provider":       params.Provider,
		"resource_type":  params.ResourceType,
		"resource_name":  params.ResourceName,
		"resource_mode":  string(params.ResourceMode),
		"current_state":  currentJSON,
		"planned_state":  plannedJSON,
		"config":         configJSON,
		"planned_action": params.PlannedAction,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	var result HookResult
	if err := c.call(ctx, "pre-apply", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (c *Client) PostApply(ctx context.Context, params PostApplyParams) (*HookResult, error) {
	// Convert cty.Values to JSON
	var beforeJSON, afterJSON, configJSON json.RawMessage
	
	if !params.Before.IsNull() {
		b, err := ctyjson.SimpleJSONValue{Value: params.Before}.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal before state: %w", err)
		}
		beforeJSON = b
	}
	
	if !params.After.IsNull() {
		a, err := ctyjson.SimpleJSONValue{Value: params.After}.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal after state: %w", err)
		}
		afterJSON = a
	}
	
	if !params.Config.IsNull() {
		// For config, we need to handle unknown values by converting them to null
		configWithoutUnknowns := cty.UnknownAsNull(params.Config)
		c, err := ctyjson.Marshal(configWithoutUnknowns, configWithoutUnknowns.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		configJSON = c
	}
	
	rpcParams := map[string]interface{}{
		"provider":       params.Provider,
		"resource_type":  params.ResourceType,
		"resource_name":  params.ResourceName,
		"resource_mode":  string(params.ResourceMode),
		"before":         beforeJSON,
		"after":          afterJSON,
		"config":         configJSON,
		"applied_action": params.AppliedAction,
		"failed":         params.Failed,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	
	var result HookResult
	if err := c.call(ctx, "post-apply", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (c *Client) PreRefresh(ctx context.Context, params PreRefreshParams) (*HookResult, error) {
	// Convert cty.Value to JSON
	var stateJSON json.RawMessage
	
	if !params.CurrentState.IsNull() {
		s, err := ctyjson.SimpleJSONValue{Value: params.CurrentState}.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current state: %w", err)
		}
		stateJSON = s
	}
	
	rpcParams := map[string]interface{}{
		"provider":      params.Provider,
		"resource_type": params.ResourceType,
		"resource_name": params.ResourceName,
		"resource_mode": string(params.ResourceMode),
		"current_state": stateJSON,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	var result HookResult
	if err := c.call(ctx, "pre-refresh", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

func (c *Client) PostRefresh(ctx context.Context, params PostRefreshParams) (*HookResult, error) {
	// Convert cty.Values to JSON
	var beforeJSON, afterJSON json.RawMessage
	
	if !params.Before.IsNull() {
		b, err := ctyjson.SimpleJSONValue{Value: params.Before}.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal before: %w", err)
		}
		beforeJSON = b
	}
	
	if !params.After.IsNull() {
		a, err := ctyjson.SimpleJSONValue{Value: params.After}.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal after: %w", err)
		}
		afterJSON = a
	}
	
	rpcParams := map[string]interface{}{
		"provider":       params.Provider,
		"resource_type":  params.ResourceType,
		"resource_name":  params.ResourceName,
		"resource_mode":  string(params.ResourceMode),
		"before":         beforeJSON,
		"after":          afterJSON,
		"drift_detected": params.DriftDetected,
		"previous_middleware_metadata": params.PreviousMiddlewareMetadata,
	}
	
	var result HookResult
	if err := c.call(ctx, "post-refresh", rpcParams, &result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

// Ping sends a ping request to the middleware and expects a pong response
func (c *Client) Ping(ctx context.Context) error {
	var result map[string]interface{}
	if err := c.call(ctx, "ping", map[string]interface{}{}, &result); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check for pong response
	msg, ok := result["message"].(string)
	if !ok || msg != "pong" {
		return fmt.Errorf("unexpected ping response: %v", result)
	}

	return nil
}