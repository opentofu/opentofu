// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
)

// StateEncryption describes the interface for encrypting state files.
type StateEncryption interface {
	// DecryptState decrypts a potentially encrypted state file and returns a valid JSON-serialized state file.
	//
	// When implementing this function:
	//
	// If the user configured no encryption, also return the input as-is regardless if the state file is valid. If the
	// user configured encryption unserialize the input as JSON and check for the presence of the field specified in the
	// StateEncryptionMarkerField. If the field is not present, return the input as-is and return a warning that an
	// unexpected unencrypted state file was encountered. Otherwise, decrypt the state file and return the decrypted
	// state file as serialized JSON. If the state file cannot be decrypted, return an error in the diagnostics.
	//
	// When using this function:
	//
	// After reading the state file from its source (local file, remote backend, etc.), pass in the state file to this
	// function. Do not attempt to determine if the state file is encrypted as this function will take care of any
	// and all encryption-related matters. After the function returns, use the returned byte array as a normal state
	// file.
	DecryptState([]byte) ([]byte, EncryptionStatus, error)

	// EncryptState encrypts a state file and returns the encrypted form.
	//
	// When implementing this function:
	//
	// The file should take a JSON-serialized state file as an input and encrypt it according to the configuration.
	// The encrypted form should also return a JSON which contains, at least, the key specified in
	// StateEncryptionMarkerField to identify the state file as encrypted. This is necessary because some backends
	// expect a state file to always be a JSON file.
	//
	// If the user configured no encryption, this function should be a no-op and return the input unchanged. If the
	// input is not a valid state file, this function should return an error in the diagnostics return.
	//
	// When using this function:
	//
	// Pass in a valid JSON-serialized state file as an input and store the output. Note that you should not pass the
	// output to any additional functions that require a valid state file as it may not contain the fields typically
	// present in a state file.
	EncryptState([]byte) ([]byte, error)
}

type stateEncryption struct {
	base *baseEncryption
}

func newStateEncryption(ctx context.Context, enc *encryption, target *config.TargetConfig, enforced bool, name string, staticEval *configs.StaticEvaluator) (StateEncryption, hcl.Diagnostics) {
	base, diags := newBaseEncryption(ctx, enc, target, enforced, name, staticEval)
	return &stateEncryption{base}, diags
}

type statedata struct {
	Serial  *int   `json:"serial"`
	Lineage string `json:"lineage"`
}

func (s *stateEncryption) EncryptState(plainState []byte) ([]byte, error) {
	var passthrough statedata
	err := json.Unmarshal(plainState, &passthrough)
	if err != nil {
		return nil, err
	}

	return s.base.encrypt(plainState, func(base basedata) interface{} {
		// Merge together the base encryption data and the passthrough fields
		return struct {
			statedata
			basedata
		}{
			statedata: passthrough,
			basedata:  base,
		}
	})
}

func (s *stateEncryption) DecryptState(encryptedState []byte) ([]byte, EncryptionStatus, error) {
	decryptedState, status, err := s.base.decrypt(context.TODO(), encryptedState, func(data []byte) error {
		tmp := struct {
			FormatVersion string `json:"terraform_version"`
		}{}
		err := json.Unmarshal(data, &tmp)
		if err != nil {
			return err
		}
		if len(tmp.FormatVersion) == 0 {
			// Not a state file
			return fmt.Errorf("Given payload is not a state file")
		}
		// Probably a state file
		return nil
	})

	if err != nil {
		return nil, status, err
	}

	// Make sure that the state passthrough fields match
	var encrypted statedata
	err = json.Unmarshal(encryptedState, &encrypted)
	if err != nil {
		return nil, status, err
	}
	var state statedata
	err = json.Unmarshal(decryptedState, &state)
	if err != nil {
		return nil, status, err
	}

	// TODO make encrypted.Serial non-optional.  This is only for supporting alpha1 states!
	if encrypted.Serial != nil && state.Serial != nil && *state.Serial != *encrypted.Serial {
		return nil, status, fmt.Errorf("invalid state metadata, serial field mismatch %v vs %v", *encrypted.Serial, *state.Serial)
	}

	// TODO make encrypted.Lineage non-optional.  This is only for supporting alpha1 states!
	if encrypted.Lineage != "" && state.Lineage != encrypted.Lineage {
		return nil, status, fmt.Errorf("invalid state metadata, linage field mismatch %v vs %v", encrypted.Lineage, state.Lineage)
	}

	return decryptedState, status, nil
}

func StateEncryptionDisabled() StateEncryption {
	return &stateDisabled{}
}

type stateDisabled struct{}

func (s *stateDisabled) EncryptState(plainState []byte) ([]byte, error) {
	return plainState, nil
}
func (s *stateDisabled) DecryptState(encryptedState []byte) ([]byte, EncryptionStatus, error) {
	return encryptedState, StatusSatisfied, nil
}
