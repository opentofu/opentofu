package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

// Header is the initial line that needs to be written as JSON when the program starts.
type Header struct {
	Magic   string `json:"magic"`
	Version int    `json:"version"`
}

// Input is the input data received from OpenTofu in response to the header as JSON.
type Input struct {
	// Key is the encryption or decryption key, if present.
	Key []byte `json:"key,omitempty"`
	// Payload is the data to encrypt/decrypt.
	Payload []byte `json:"payload"`
}

// Output is the data structure that should be written to the output.
type Output struct {
	// Payload is the payload that has been encrypted/decrypted by the external method.
	Payload []byte `json:"payload"`
}

func main() {
	// Write logs to stderr
	log.Default().SetOutput(os.Stderr)

	// Write header:
	header := Header{
		"OpenTofu-External-Encryption-Method",
		1,
	}
	marshalledHeader, err := json.Marshal(header)
	if err != nil {
		log.Fatalf("%v", err)
	}
	_, _ = os.Stdout.Write(append(marshalledHeader, []byte("\n")...))

	// Read input:
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}
	var inputData Input
	if err = json.Unmarshal(input, &inputData); err != nil {
		log.Fatalf("Failed to parse stdin: %v", err)
	}

	// Create encrypt the input and produce the output here.
	var outputPayload []byte

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
