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

type basedata struct {
	Meta    map[keyprovider.Addr][]byte `json:"meta"`
	Data    []byte                      `json:"encrypted_data"`
	Version string                      `json:"encryption_version"` // This is both a sigil for a valid encrypted payload and a future compatability field
}

func (s *baseEncryption) encrypt(data []byte) ([]byte, hcl.Diagnostics) {
	if s.target == nil {
		return data, nil
	}

	es := basedata{
		Meta:    make(map[keyprovider.Addr][]byte),
		Version: encryptionVersion,
	}

	// Mutates es.Meta
	methods, diags := s.buildTargetMethods(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	var encryptor method.Method = nil
	if len(methods) != 0 {
		encryptor = methods[0]
	}

	if encryptor == nil {
		// ensure that the method is defined when Enforced is true
		if s.enforced {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Encryption method required",
				Detail:   fmt.Sprintf("%q is enforced, and therefore requires a method to be provided", s.name),
			})
			return nil, diags
		}
		return data, nil
	}

	encd, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption failed for " + s.name,
			Detail:   err.Error(),
		})
	}

	es.Data = encd
	jsond, err := json.Marshal(es)
	if err != nil {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to encode encrypted data as json",
			Detail:   err.Error(),
		})
	}

	return jsond, diags
}

// TODO Find a way to make these errors actionable / clear
func (s *baseEncryption) decrypt(data []byte, validator func([]byte) error) ([]byte, hcl.Diagnostics) {
	if s.target == nil {
		return data, nil
	}

	es := basedata{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid data format for decryption",
			Detail:   err.Error(),
		}}
	}

	if len(es.Version) == 0 {
		// Not a valid payload, might be already decrypted
		err = validator(data)
		if err == nil {
			// Yep, it's already decrypted
			return data, nil
		} else {
			// Nope, just bad input
			return nil, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to determine data structure during decryption",
			}}
		}
	}

	if es.Version != encryptionVersion {
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid encrypted payload version",
			Detail:   fmt.Sprintf("%s != %s", es.Version, encryptionVersion),
		}}
	}

	methods, diags := s.buildTargetMethods(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	if len(methods) == 0 {
		err = validator(data)
		if err == nil {
			// No methods/fallbacks specified and data is valid payload
			return data, diags
		} else {
			// TODO improve this error message
			return nil, append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  err.Error(),
			})
		}
	}

	var methodDiags hcl.Diagnostics
	for _, method := range methods {
		if method == nil {
			// No method specified for this target
			err = validator(data)
			if err == nil {
				return data, diags
			}
			// toDO improve this error message
			methodDiags = append(methodDiags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Attempted decryption failed for " + s.name,
				Detail:   err.Error(),
			})
			continue
		}
		uncd, err := method.Decrypt(es.Data)
		if err == nil {
			// Success
			return uncd, diags
		}
		// Record the failure
		methodDiags = append(methodDiags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Attempted decryption failed for " + s.name,
			Detail:   err.Error(),
		})
	}

	// Record the overall failure
	diags = append(diags, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Decryption failed",
		Detail:   "All methods of decryption provided failed for " + s.name,
	})

	return nil, append(diags, methodDiags...)
}
