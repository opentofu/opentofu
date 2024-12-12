package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

type Output struct {
	Key struct {
		EncryptionKey []byte `json:"encryption_key,omitempty"`
		DecryptionKey []byte `json:"decryption_key,omitempty"`
	} `json:"key"`
	Meta *struct {
		ExternalData map[string]any `json:"external_data"`
	} `json:"meta,omitempty"`
}

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}
	_, _ = os.Stderr.Write(input)
	var inMeta any
	if err := json.Unmarshal(input, &inMeta); err != nil {
		log.Fatalf("Failed to parse stdin: %v", err)
	}

	decryptionKey := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	if inMeta == nil {
		decryptionKey = nil
	}

	output := Output{
		Key: struct {
			EncryptionKey []byte `json:"encryption_key,omitempty"`
			DecryptionKey []byte `json:"decryption_key,omitempty"`
		}{
			EncryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			DecryptionKey: decryptionKey,
		},
		Meta: &struct {
			ExternalData map[string]any `json:"external_data"`
		}{ExternalData: map[string]any{}},
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		log.Fatalf("Failed to stringify output: %v", err)
	}
	_, _ = os.Stdout.Write(outputData)
}
