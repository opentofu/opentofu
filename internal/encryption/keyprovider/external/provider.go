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
)

type keyProvider struct {
	command []string
}

func (k keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta, ok := rawMeta.(*MetadataV1)
	if !ok {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("bug: incorrect metadata type of %T provided", rawMeta),
		}
	}

	input, err := json.Marshal(inMeta)
	if err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("bug: cannot JSON-marshal metadata (%v)", err),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	stderr := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, k.command[0], k.command[1:]...)

	handler := &ioHandler{
		false,
		bytes.NewBuffer(input),
		[]byte{},
		cancel,
		nil,
	}

	cmd.Stdin = handler
	cmd.Stdout = handler
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if handler.err != nil {
			return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
				Message: "external key provider protocol failure",
				Cause:   err,
			}
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() != 0 {
				return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
					Message: fmt.Sprintf("the external command exited with a non-zero exit code (%v)\n\nStderr:\n-------\n%s", err, stderr),
				}
			}
		}
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("the external command exited with an error (%v)\n\nStderr:\n-------\n%s", err, stderr),
		}
	}

	var result *OutputV1
	decoder := json.NewDecoder(bytes.NewReader(handler.output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("the external command returned an invalid JSON response (%v)\n\nStderr:\n-------\n%s", err, stderr),
		}
	}

	return result.Keys, &result.Meta, nil
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
	parts := strings.SplitN(string(i.output), "\n", 2)
	if len(parts) == 1 {
		return n, nil
	}
	var header Header
	// Note: this is intentionally not using strict decoding. Later protocol versions may introduce additional header
	// fields.
	if jsonErr := json.Unmarshal([]byte(parts[0]), &header); jsonErr != nil {
		err := fmt.Errorf("failed to unmarshal header from external binary (%w)", jsonErr)
		i.err = err
		i.cancel()
		return n, err
	}

	if header.Magic != HeaderMagic {
		err := fmt.Errorf("invalid magic received from external key provider: %s", header.Magic)
		i.err = err
		i.cancel()
		return n, err
	}
	if header.Version != 1 {
		err := fmt.Errorf("invalid version number received from external key provider: %d", header.Version)
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
