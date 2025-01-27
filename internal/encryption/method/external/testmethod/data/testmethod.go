// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

type Header struct {
	Magic   string `json:"magic"`
	Version int    `json:"version"`
}

type Input struct {
	Key     []byte `json:"key,omitempty"`
	Payload []byte `json:"payload"`
}

type Output struct {
	// Payload is the payload that has been encrypted/decrypted by the external method.
	Payload []byte `json:"payload"`
}

// main implements a simple XOR-encryption. This is meant as an example and not suitable for any production use.
func main() {
	// Write logs to stderr
	log.Default().SetOutput(os.Stderr)

	// Write header
	header := Header{
		"OpenTofu-External-Encryption-Method",
		1,
	}
	marshalledHeader, err := json.Marshal(header)
	if err != nil {
		log.Fatalf("%v", err)
	}
	_, _ = os.Stdout.Write(append(marshalledHeader, []byte("\n")...))

	// Read input
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}
	var inputData Input
	if err = json.Unmarshal(input, &inputData); err != nil {
		log.Fatalf("Failed to parse stdin: %v", err)
	}

	// Create output as an XOR of the key and input
	outputPayload := make([]byte, len(inputData.Payload))
	for i, b := range inputData.Payload {
		outputPayload[i] = inputData.Key[i%len(inputData.Key)] ^ b
	}

	// Write output
	output := Output{
		Payload: outputPayload,
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		log.Fatalf("Failed to stringify output: %v", err)
	}
	_, _ = os.Stdout.Write(outputData)
}
