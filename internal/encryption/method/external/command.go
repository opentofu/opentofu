// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

type command struct {
	keys           *keyprovider.Output
	encryptCommand []string
	decryptCommand []string
}

func (c command) Encrypt(data []byte) ([]byte, error) {
	var key []byte
	if c.keys != nil {
		key = c.keys.EncryptionKey
	}
	input := InputV1{
		Key:     key,
		Payload: data,
	}
	result, err := c.run(c.encryptCommand, input)
	if err != nil {
		return nil, &method.ErrEncryptionFailed{
			Cause: err,
		}
	}
	return result, nil
}

func (c command) Decrypt(data []byte) ([]byte, error) {
	var key []byte
	if c.keys != nil {
		key = c.keys.DecryptionKey
		if len(c.keys.EncryptionKey) > 0 && len(key) == 0 {
			return nil, &method.ErrDecryptionKeyUnavailable{}
		}
	}
	if len(data) == 0 {
		return nil, &method.ErrDecryptionFailed{Cause: &method.ErrCryptoFailure{
			Message: "Cannot decrypt empty data.",
		}}
	}
	input := InputV1{
		Key:     key,
		Payload: data,
	}
	result, err := c.run(c.decryptCommand, input)
	if err != nil {
		return nil, &method.ErrDecryptionFailed{
			Cause: err,
		}
	}
	return result, nil
}

func (c command) run(command []string, input any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	inputData, err := json.Marshal(input)
	if err != nil {
		return nil, &method.ErrCryptoFailure{
			Message: "failed to marshal input",
			Cause:   err,
		}
	}

	stderr := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec //Launching external commands here is the entire point.

	handler := &ioHandler{
		false,
		bytes.NewBuffer(inputData),
		[]byte{},
		cancel,
		nil,
	}

	cmd.Stdin = handler
	cmd.Stdout = handler
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if handler.err != nil {
			return nil, &method.ErrCryptoFailure{
				Message:          "external encryption method failure",
				Cause:            handler.err,
				SupplementalData: fmt.Sprintf("Stderr:\n-------\n%s\n", stderr.String()),
			}
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() != 0 {
				return nil, &method.ErrCryptoFailure{
					Message:          "external encryption method exited with non-zero exit code",
					Cause:            err,
					SupplementalData: fmt.Sprintf("Stderr:\n-------\n%s\n", stderr.String()),
				}
			}
		}
		return nil, &method.ErrCryptoFailure{
			Message:          "external encryption method exited with an error",
			Cause:            err,
			SupplementalData: fmt.Sprintf("Stderr:\n-------\n%s\n", stderr.String()),
		}
	}

	var result *OutputV1
	decoder := json.NewDecoder(bytes.NewReader(handler.output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return nil, &method.ErrCryptoFailure{
			Message:          "external encryption method returned an invalid JSON",
			Cause:            err,
			SupplementalData: fmt.Sprintf("Stderr:\n-------\n%s\n", stderr.String()),
		}
	}

	return result.Payload, nil
}

type ioHandler struct {
	headerFinished bool
	input          *bytes.Buffer
	output         []byte
	cancel         func()
	err            error
}

func (i *ioHandler) Write(p []byte) (int, error) {
	i.output = append(i.output, p...)
	n := len(p)
	if i.headerFinished {
		// Header is finished, just collect the output.
		return n, nil
	}
	// Check if the full header is present.
	parts := strings.SplitN(string(i.output), "\n", 2) //nolint:mnd //This rule is dumb.
	if len(parts) == 1 {
		return n, nil
	}
	var header Header
	// Note: this is intentionally not using strict decoding. Later protocol versions may introduce additional header
	// fields.
	if jsonErr := json.Unmarshal([]byte(parts[0]), &header); jsonErr != nil {
		err := fmt.Errorf("failed to unmarshal header from external method (%w)", jsonErr)
		i.err = err
		i.cancel()
		return n, err
	}

	if header.Magic != Magic {
		err := fmt.Errorf("invalid magic received from external method: %s", header.Magic)
		i.err = err
		i.cancel()
		return n, err
	}
	if header.Version != 1 {
		err := fmt.Errorf("invalid version number received from external method: %d", header.Version)
		i.err = err
		i.cancel()
		return n, err
	}
	i.headerFinished = true
	i.output = []byte(parts[1])
	return n, nil
}

func (i *ioHandler) Read(p []byte) (int, error) {
	return i.input.Read(p)
}
