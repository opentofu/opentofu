// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"

	"github.com/hashicorp/hcl/v2"
)

const (
	// encryptionVersion is the version of the encryption format
	// used in the json that is stored in the backend
	// This is used to ensure that we can handle future changes to the encryption format
	// and to ensure that we can handle migration between multiple versions of the encryption format over time
	encryptionVersion = "v0"
)

type baseEncryption struct {
	enc      *encryption
	target   *config.TargetConfig
	enforced bool
	name     string
}

func newBaseEncryption(enc *encryption, target *config.TargetConfig, enforced bool, name string) *baseEncryption {
	return &baseEncryption{
		enc:      enc,
		target:   target,
		enforced: enforced,
		name:     name,
	}
}

type baseData struct {
	Meta    map[keyprovider.Addr][]byte `json:"meta"`
	Data    []byte                      `json:"encrypted_data"`
	Version string                      `json:"encryption_version"` // This is both a sigil for a valid encrypted payload and a future compatability field
}

func (b *baseEncryption) encrypt(data []byte) ([]byte, hcl.Diagnostics) {
	if b.target == nil {
		return data, nil
	}

	es := baseData{
		Meta:    make(map[keyprovider.Addr][]byte),
		Version: encryptionVersion,
	}

	// Mutates es.Meta
	methods, diags := b.buildTargetMethods(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	var encryptor method.Method = nil
	if len(methods) != 0 {
		encryptor = methods[0]
	}

	if encryptor == nil {
		// ensure that the method is defined when Enforced is true
		if b.enforced {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Encryption method required",
				Detail:   fmt.Sprintf("%q is enforced, and therefore requires a method to be provided", b.name),
			})
			return nil, diags
		}
		// if no method is defined and Enforced is false, we can just return the data as is, no encryption
		// should be performed
		return data, nil
	}

	encrypted, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption failed for " + b.name,
			Detail:   err.Error(),
		})
	}

	es.Data = encrypted

	// Marshal the baseData struct to json ready to be stored
	marshalledData, err := json.Marshal(es)
	if err != nil {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to encode encrypted data as json",
			Detail:   err.Error(),
		})
	}

	return marshalledData, diags
}

func (b *baseEncryption) decrypt(data []byte, validator func([]byte) error) ([]byte, hcl.Diagnostics) {
	if b.target == nil {
		return data, nil
	}

	es := baseData{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to decode encrypted payload as json",
			Detail:   err.Error(),
		}}
	}

	data, diags := validateVersion(data, validator, es)
	if diags.HasErrors() {
		return nil, diags
	}

	methods, diags := b.buildTargetMethods(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	// Ensure that we have some methods that we can use
	if len(methods) == 0 {
		// If we do not have any methods defined, we can just use the validator to check if the data is valid
		// and return that
		err = validator(data)
		if err != nil {
			return nil, append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Detail:   "Attempted decryption failed, no methods or fallbacks specified and the data is invalid",
				Summary:  err.Error(),
			})
		}
		// No methods/fallbacks specified and data is valid payload
		return data, diags
	}

	// Initialize diagnostics to record any issues encountered during decryption.
	var methodDiags hcl.Diagnostics

	// Attempt to decrypt the data using each method in turn
	for _, method := range methods {
		if method == nil {
			// If no method has been specified for this target, attempt to validate and return the
			// data if it is valid
			err = validator(data)
			if err != nil {
				// We record the validation failure but continue to the next method
				// this allows us to record all failures and not just the first one
				methodDiags = append(methodDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Attempted decryption failed for " + b.name,
					Detail:   err.Error(),
				})
				continue
			}
			// Validation was successful, return the data
			return data, diags
		}
		// Attempt to decrypt the data using the method
		decrypted, err := method.Decrypt(es.Data)
		if err != nil {
			// Record the failure and move on
			methodDiags = append(methodDiags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Attempted decryption failed for " + b.name,
				Detail:   err.Error(),
			})
			continue
		}
		// Successfully decrypted, return the data
		return decrypted, diags
	}

	// If we have reached here, we have attempted to decrypt with all the methods and failed
	// Record the overall failure
	diags = append(diags, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Decryption failed",
		Detail:   "All methods of decryption provided failed for " + b.name,
	})

	return nil, append(diags, methodDiags...)
}

// validateVersion takes in the possibly encrypted data and validates that it is in the expected format
// It returns the data if it is already decrypted, or diagnostics if it is not in the expected format
func validateVersion(data []byte, validator func([]byte) error, es baseData) ([]byte, hcl.Diagnostics) {
	// Valid payloads should have their `Version` field set correctly
	// We can use this as flag to detect if the json marshalling unmarshalled a valid payload
	// or if it was just bad input
	if len(es.Version) == 0 {
		// Check if the data is already decrypted by passing it through the validator
		// If it's already decrypted then we can return the data directly
		err := validator(data)
		if err != nil {
			// we can return an error because someone could've passed in some other form of json data
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to determine data type during decryption",
			}}
		}
		// Data may already be decrypted, return it
		return data, nil
	}

	if es.Version != encryptionVersion {
		// In the future, we should handle migration between versions, however for now we should just error
		// as we only have one version
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid encrypted payload version",
			Detail:   fmt.Sprintf("expected %q, got %q", encryptionVersion, es.Version),
		}}
	}

	return data, nil
}
