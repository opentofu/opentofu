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

type Output struct {
	Keys struct {
		EncryptionKey []byte `json:"encryption_key,omitempty"`
		DecryptionKey []byte `json:"decryption_key,omitempty"`
	} `json:"keys"`
	Meta struct {
		ExternalData map[string]any `json:"external_data"`
	} `json:"meta,omitempty"`
}

func main() {
	// Write logs to stderr
	log.Default().SetOutput(os.Stderr)

	header := Header{
		"OpenTofu-External-Key-Provider",
		1,
	}
	marshalledHeader, err := json.Marshal(header)
	if err != nil {
		log.Fatalf("%v", err)
	}
	_, _ = os.Stdout.Write(append(marshalledHeader, []byte("\n")...))

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}
	var inMeta any
	if err = json.Unmarshal(input, &inMeta); err != nil {
		log.Fatalf("Failed to parse stdin: %v", err)
	}

	key := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	if len(os.Args) == 2 && os.Args[1] == "--hello-world" {
		key = []byte("Hello world! 123")
	}

	decryptionKey := key
	if inMeta == nil {
		decryptionKey = nil
	}

	output := Output{
		Keys: struct {
			EncryptionKey []byte `json:"encryption_key,omitempty"`
			DecryptionKey []byte `json:"decryption_key,omitempty"`
		}{
			EncryptionKey: key,
			DecryptionKey: decryptionKey,
		},
		Meta: struct {
			ExternalData map[string]any `json:"external_data"`
		}{ExternalData: map[string]any{}},
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		log.Fatalf("Failed to stringify output: %v", err)
	}
	_, _ = os.Stdout.Write(outputData)
}
