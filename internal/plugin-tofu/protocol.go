// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package plugintofu

type Request struct {
	ID     uint64      `msgpack:"id"`
	Method string      `msgpack:"method"`
	Params interface{} `msgpack:"params,omitempty"`
}

type Response struct {
	ID     uint64      `msgpack:"id"`
	Result interface{} `msgpack:"result,omitempty"`
	Error  *RPCError   `msgpack:"error,omitempty"`
}

type RPCError struct {
	Code    int         `msgpack:"code"`
	Message string      `msgpack:"message"`
	Data    interface{} `msgpack:"data,omitempty"`
}

// Messages
// TODO: Move these out
type InitializeRequest struct {
	Config map[string]interface{} `msgpack:"config,omitempty"`
}

type InitializeResponse struct {
	ProtocolVersion string   `msgpack:"protocol_version"`
	Capabilities    []string `msgpack:"capabilities"`
}

type ShutdownRequest struct{}
type ShutdownResponse struct{}

type PingRequest struct{}
type PingResponse struct {
	Timestamp int64 `msgpack:"timestamp"`
}

type GetSchemaRequest struct{}

type GetSchemaResponse struct {
	Functions map[string]FunctionSchema `msgpack:"functions"`
}

type FunctionSchema struct {
	Parameters []Parameter `msgpack:"parameters"`
	Return     Parameter   `msgpack:"return"`
	Summary    string      `msgpack:"summary,omitempty"`
}

type Parameter struct {
	Name        string `msgpack:"name"`
	Type        string `msgpack:"type"`
	Description string `msgpack:"description,omitempty"`
	Optional    bool   `msgpack:"optional,omitempty"`
}

type CallFunctionRequest struct {
	Name string                 `msgpack:"name"`
	Args map[string]interface{} `msgpack:"args"`
}

type CallFunctionResponse struct {
	Result interface{} `msgpack:"result"`
}
