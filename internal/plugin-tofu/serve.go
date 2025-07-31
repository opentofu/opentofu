// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package plugintofu

import (
	"encoding/binary"
	"io"
	"log"
	"os"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type Provider interface {
	Initialize(req *InitializeRequest) (*InitializeResponse, error)
	GetSchema() (*GetSchemaResponse, error)
	CallFunction(req *CallFunctionRequest) (*CallFunctionResponse, error)
	Shutdown() error
}

// ProviderServer manages the provider lifecycle and message handling
type ProviderServer struct {
	provider    Provider
	initialized bool
	logger      *log.Logger
}

// Serve starts a stdio-based MessagePack-RPC server for a provider
func Serve(provider Provider) {
	// Set up stderr logging as per RFC
	logger := log.New(os.Stderr, "[provider-tofu] ", log.LstdFlags)

	server := &ProviderServer{
		provider: provider,
		logger:   logger,
	}

	logger.Println("Provider starting...")

	for {
		req, err := server.readMessage()
		if err != nil {
			if err == io.EOF {
				logger.Println("Provider shutting down (EOF)")
				break
			}
			logger.Printf("Error reading message: %v", err)
			continue
		}

		server.handleRequest(req)
	}
}

func (s *ProviderServer) readMessage() (*Request, error) {
	// Read length prefix (4 bytes, big endian)
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(os.Stdin, lengthBytes); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBytes)

	// Read message data
	data := make([]byte, length)
	if _, err := io.ReadFull(os.Stdin, data); err != nil {
		return nil, err
	}

	var req Request
	if err := msgpack.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

func (s *ProviderServer) handleRequest(req *Request) {
	s.logger.Printf("Handling method: %s (id: %d)", req.Method, req.ID)

	switch req.Method {
	case "initialize":
		if s.initialized {
			s.writeError(req.ID, 1000, "Provider already initialized")
			return
		}

		var params InitializeRequest
		if err := s.unmarshalParams(req.Params, &params); err != nil {
			s.writeError(req.ID, 1001, "Invalid initialize params: "+err.Error())
			return
		}

		result, err := s.provider.Initialize(&params)
		if err != nil {
			s.writeError(req.ID, 1002, "Initialize failed: "+err.Error())
			return
		}

		s.initialized = true
		s.writeResult(req.ID, result)

	case "ping":
		if !s.initialized {
			s.writeError(req.ID, 1003, "Provider not initialized")
			return
		}

		result := &PingResponse{
			Timestamp: time.Now().Unix(),
		}
		s.writeResult(req.ID, result)

	case "getSchema":
		if !s.initialized {
			s.writeError(req.ID, 1003, "Provider not initialized")
			return
		}

		result, err := s.provider.GetSchema()
		if err != nil {
			s.writeError(req.ID, 1004, "GetSchema failed: "+err.Error())
			return
		}
		s.writeResult(req.ID, result)

	case "callFunction":
		if !s.initialized {
			s.writeError(req.ID, 1003, "Provider not initialized")
			return
		}

		var params CallFunctionRequest
		if err := s.unmarshalParams(req.Params, &params); err != nil {
			s.writeError(req.ID, 1005, "Invalid function params: "+err.Error())
			return
		}

		result, err := s.provider.CallFunction(&params)
		if err != nil {
			s.writeError(req.ID, 1006, "Function call failed: "+err.Error())
			return
		}
		s.writeResult(req.ID, result)

	case "shutdown":
		if err := s.provider.Shutdown(); err != nil {
			s.writeError(req.ID, 1007, "Shutdown failed: "+err.Error())
			return
		}

		s.writeResult(req.ID, &ShutdownResponse{})
		s.logger.Println("Provider shutdown complete")
		os.Exit(0)

	default:
		s.writeError(req.ID, 1008, "Method not found: "+req.Method)
	}
}

func (s *ProviderServer) unmarshalParams(params interface{}, target interface{}) error {
	if params == nil {
		return nil
	}

	data, err := msgpack.Marshal(params)
	if err != nil {
		return err
	}

	return msgpack.Unmarshal(data, target)
}

func (s *ProviderServer) writeResult(id uint64, result interface{}) {
	resp := Response{
		ID:     id,
		Result: result,
	}
	s.writeMessage(&resp)
}

func (s *ProviderServer) writeError(id uint64, code int, message string) {
	resp := Response{
		ID: id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	s.writeMessage(&resp)
}

func (s *ProviderServer) writeMessage(msg interface{}) {
	data, err := msgpack.Marshal(msg)
	if err != nil {
		s.logger.Printf("Error marshaling response: %v", err)
		return
	}

	// Write length prefix (4 bytes, big endian)
	length := uint32(len(data))
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, length)

	os.Stdout.Write(lengthBytes)
	os.Stdout.Write(data)
	os.Stdout.Sync()
}
