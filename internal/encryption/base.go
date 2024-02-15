package encryption

import (
	"encoding/json"

	"github.com/hashicorp/hcl/v2"
)

// This currently deals with raw bytes so it could be moved into it's own library and not depend explicitly on the OpenTofu codebase.
type StateEncryption interface {
	EncryptState([]byte) ([]byte, hcl.Diagnostics)

	// Validation is performed that the decrypted []byte is some sort of json object
	DecryptState([]byte) ([]byte, hcl.Diagnostics)
}

type baseEncryption struct {
	f        *encryption
	t        *TargetConfig
	enforced bool
	name     string
}

func NewState(f *encryption, t *TargetConfig, name string) StateEncryption {
	return &baseEncryption{f, t, false, name}
}

func NewEnforcableState(f *encryption, t *EnforcableTargetConfig, name string) StateEncryption {
	return &baseEncryption{f, t.AsTargetConfig(), t.Enforced, name}
}

type encstate struct {
	Meta map[string][]byte `json:"meta"`
	Data []byte            `json:"state"`
}

func (s *baseEncryption) EncryptState(data []byte) ([]byte, hcl.Diagnostics) {
	es := encstate{
		Meta: make(map[string][]byte),
	}

	// Mutates es.Meta
	methods, diags := targetToMethods(s.f, s.t, s.enforced, s.name, es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	encd, err := methods[0].Encrypt(data)
	if err != nil {
		// TODO diags
		panic(err)
	}

	es.Data = encd
	jsond, err := json.Marshal(es)
	if err != nil {
		// TODO diags
		panic(err)
	}

	return jsond, diags
}

func (s *baseEncryption) DecryptState(data []byte) ([]byte, hcl.Diagnostics) {
	es := encstate{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		// TODO diags
		panic(err)
	}

	methods, diags := targetToMethods(s.f, s.t, s.enforced, s.name, es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	uncd, err := methods[0].Decrypt(es.Data)
	if err != nil {
		// TODO diags
		panic(err)
	}
	return uncd, diags

}
