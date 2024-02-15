package encryption

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

type baseEncryption struct {
	enc      *encryption
	target   *TargetConfig
	enforced bool
	name     string
}

func newBaseEncryption(enc *encryption, target *TargetConfig, enforced bool, name string) *baseEncryption {
	return &baseEncryption{
		enc:      enc,
		target:   target,
		enforced: enforced,
		name:     name,
	}
}

type basedata struct {
	Meta map[string][]byte `json:"meta"`
	Data []byte            `json:"state"`
}

func (s *baseEncryption) encrypt(data []byte) ([]byte, hcl.Diagnostics) {
	if s.target == nil {
		return data, nil
	}

	es := basedata{
		Meta: make(map[string][]byte),
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

func (s *baseEncryption) decrypt(data []byte) ([]byte, hcl.Diagnostics) {
	if s.target == nil {
		return data, nil
	}

	es := basedata{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to decode encrypted data as json",
			Detail:   err.Error(),
		}}
	}

	methods, diags := s.buildTargetMethods(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	var methodDiags hcl.Diagnostics
	for _, method := range methods {
		if method == nil {
			// TODO detection of valid vs invalid decrypted payload
			continue
		}
		uncd, err := methods[0].Decrypt(es.Data)
		if err == nil {
			// Success
			return uncd, diags
		}
		// Record the failure
		methodDiags = append(methodDiags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Decryption failed for " + s.name,
			Detail:   err.Error(),
		})
	}
	return nil, append(diags, methodDiags...)
}
