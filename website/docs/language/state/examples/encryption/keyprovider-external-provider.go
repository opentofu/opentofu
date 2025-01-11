package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

// Header is the initial greeting the key provider sends out.
type Header struct {
	// Magic must always be OpenTofu-External-Keyprovider
	Magic string `json:"magic"`
	// Version must be 1.
	Version int `json:"version"`
}

// Metadata describes both the input and the output metadata.
type Metadata struct {
	ExternalData map[string]any `json:"external_data"`
}

// Input describes the input data structure. This is nil on input if no existing
// data needs to be decrypted.
type Input *Metadata

// Output describes the output data written to stdout.
type Output struct {
	Key struct {
		// EncryptionKey must always be provided.
		EncryptionKey []byte `json:"encryption_key,omitempty"`
		// DecryptionKey must be provided when the input metadata is present.
		DecryptionKey []byte `json:"decryption_key,omitempty"`
	} `json:"key"`
	// Meta contains the metadata to store alongside the encrypted data. You can
	// store data here you need to reconstruct the decryption key later.
	Meta Metadata `json:"meta"`
}

func main() {
	// Write logs to stderr
	log.Default().SetOutput(os.Stderr)

	// Write the header:
	header := Header{
		"OpenTofu-External-Key-Provider",
		1,
	}
	marshalledHeader, err := json.Marshal(header)
	if err != nil {
		log.Fatalf("%v", err)
	}
	_, _ = os.Stdout.Write(append(marshalledHeader, []byte("\n")...))

	// Read the input
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}
	var inMeta Input
	if err := json.Unmarshal(input, &inMeta); err != nil {
		log.Fatalf("Failed to parse stdin: %v", err)
	}

	// TODO produce the encryption key
	if inMeta != nil {
		// TODO produce decryption key
	}

	output := Output{
		// TODO: produce output
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		log.Fatalf("Failed to encode output: %v", err)
	}
	_, _ = os.Stdout.Write(outputData)
}
