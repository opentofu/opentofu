// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyProvider struct {
	command []string
}

func (k keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keysOutput keyprovider.Output, encryptionMeta keyprovider.KeyMeta, err error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta, ok := rawMeta.(*Metadata)
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

	ctx := context.TODO()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, k.command[0], k.command[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() != 0 {
				return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
					Message: fmt.Sprintf("the external command exited with a non-zero exit code (%v)\n\nStdout:\n-------\n%s\n-------\nStderr:\n-------\n%s", err, stdout, stderr),
				}
			}
		}
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("the external command exited with an error (%v)\n\nStdout:\n-------\n%s\n-------\nStderr:\n-------\n%s", err, stdout, stderr),
		}
	}

	var result *ExternalCommandOutput
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("the external command returned an invalid JSON response (%v)\n\nStdout:\n-------\n%s\n-------\nStderr:\n-------\n%s", err, stdout, stderr),
		}
	}

	return result.Key, result.Meta, nil
}
