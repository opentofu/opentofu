// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package plugintofu

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProviderClient implements providers.Interface for MessagePack-based providers
type ProviderClient struct {
	addr        addrs.Provider
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	stderr      io.ReadCloser
	nextID      uint64
	mu          sync.Mutex
	initialized bool
}

// Verify interface compliance at compile time
var _ providers.Interface = (*ProviderClient)(nil)

// NewProviderClient creates a new client for a MessagePack provider
func NewProviderClient(addr addrs.Provider, execPath string) (*ProviderClient, error) {
	cmd := exec.Command(execPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to start provider: %w", err)
	}

	client := &ProviderClient{
		addr:   addr,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	go client.logStderr()

	return client, nil
}

// Close shuts down the provider
func (c *ProviderClient) Close(_ context.Context) error {
	if c.cmd != nil && c.cmd.Process != nil {
		// Send shutdown request
		req := &Request{
			ID:     atomic.AddUint64(&c.nextID, 1),
			Method: "shutdown",
		}
		c.sendRequest(req)

		// Clean up
		c.stdin.Close()
		c.stdout.Close()
		c.stderr.Close()
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}

func (c *ProviderClient) logStderr() {
	buf := make([]byte, 1024)
	for {
		n, err := c.stderr.Read(buf)
		if err != nil {
			break
		}
		// Log to our stderr (this will show up in OpenTofu logs)
		os.Stderr.Write(buf[:n])
	}
}

func (c *ProviderClient) sendRequest(req *Request) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Marshal request
	data, err := msgpack.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send length prefix + data
	length := uint32(len(data))
	if err := binary.Write(c.stdin, binary.BigEndian, length); err != nil {
		return nil, fmt.Errorf("failed to write length: %w", err)
	}

	if _, err := c.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write data: %w", err)
	}

	// Read response length
	var respLength uint32
	if err := binary.Read(c.stdout, binary.BigEndian, &respLength); err != nil {
		return nil, fmt.Errorf("failed to read response length: %w", err)
	}

	// Read response data
	respData := make([]byte, respLength)
	if _, err := io.ReadFull(c.stdout, respData); err != nil {
		return nil, fmt.Errorf("failed to read response data: %w", err)
	}

	// Unmarshal response
	var resp Response
	if err := msgpack.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// Implement providers.Interface methods

func (c *ProviderClient) GetProviderSchema(_ context.Context) providers.GetProviderSchemaResponse {
	if !c.initialized {
		// Initialize first
		req := &Request{
			ID:     atomic.AddUint64(&c.nextID, 1),
			Method: "initialize",
			Params: &InitializeRequest{
				Config: make(map[string]interface{}),
			},
		}

		resp, err := c.sendRequest(req)
		if err != nil {
			return providers.GetProviderSchemaResponse{
				Diagnostics: tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "Provider initialization failed", err.Error()),
				},
			}
		}
		if resp.Error != nil {
			return providers.GetProviderSchemaResponse{
				Diagnostics: tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "Provider initialization failed", resp.Error.Message),
				},
			}
		}
		c.initialized = true
	}

	req := &Request{
		ID:     atomic.AddUint64(&c.nextID, 1),
		Method: "getSchema",
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return providers.GetProviderSchemaResponse{
			Diagnostics: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Error, "Failed to get schema", err.Error()),
			},
		}
	}

	if resp.Error != nil {
		return providers.GetProviderSchemaResponse{
			Diagnostics: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Error, "Provider returned error", resp.Error.Message),
			},
		}
	}

	// For now, return minimal schema - we'll expand this later
	return providers.GetProviderSchemaResponse{
		Functions: make(map[string]providers.FunctionSpec),
	}
}

func (c *ProviderClient) ValidateProviderConfig(_ context.Context, req providers.ValidateProviderConfigRequest) providers.ValidateProviderConfigResponse {
	return providers.ValidateProviderConfigResponse{}
}

func (c *ProviderClient) ValidateResourceConfig(_ context.Context, req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	return providers.ValidateResourceConfigResponse{}
}

func (c *ProviderClient) ValidateDataResourceConfig(_ context.Context, req providers.ValidateDataResourceConfigRequest) providers.ValidateDataResourceConfigResponse {
	return providers.ValidateDataResourceConfigResponse{}
}

func (c *ProviderClient) UpgradeResourceState(_ context.Context, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return providers.UpgradeResourceStateResponse{}
}

func (c *ProviderClient) ConfigureProvider(_ context.Context, req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
	return providers.ConfigureProviderResponse{}
}

func (c *ProviderClient) Stop(ctx context.Context) error {
	return c.Close(ctx)
}

func (c *ProviderClient) ReadResource(_ context.Context, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	return providers.ReadResourceResponse{}
}

func (c *ProviderClient) PlanResourceChange(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	return providers.PlanResourceChangeResponse{}
}

func (c *ProviderClient) ApplyResourceChange(_ context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{}
}

func (c *ProviderClient) ImportResourceState(_ context.Context, req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	return providers.ImportResourceStateResponse{}
}

func (c *ProviderClient) ReadDataSource(_ context.Context, req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	return providers.ReadDataSourceResponse{}
}

func (c *ProviderClient) CallFunction(_ context.Context, funcReq providers.CallFunctionRequest) providers.CallFunctionResponse {
	req := &Request{
		ID:     atomic.AddUint64(&c.nextID, 1),
		Method: "callFunction",
		Params: &CallFunctionRequest{
			Name: funcReq.Name,
			Args: make(map[string]interface{}), // TODO: Convert from []cty.Value to map[string]interface{}
		},
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return providers.CallFunctionResponse{
			Error: fmt.Errorf("failed to call function: %w", err),
		}
	}

	if resp.Error != nil {
		return providers.CallFunctionResponse{
			Error: fmt.Errorf("provider error: %s", resp.Error.Message),
		}
	}

	// TODO: Convert result back to cty.Value
	return providers.CallFunctionResponse{
		Result: cty.NilVal, // TODO: implement conversion
	}
}

func (c *ProviderClient) MoveResourceState(_ context.Context, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	return providers.MoveResourceStateResponse{}
}

func (c *ProviderClient) GetFunctions(_ context.Context) providers.GetFunctionsResponse {
	return providers.GetFunctionsResponse{}
}
