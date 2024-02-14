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

type stateEncryption struct {
	f    *factory
	t    *TargetConfig
	name string
}

func NewState(f *factory, t *TargetConfig, name string) StateEncryption {
	return &stateEncryption{f, t, name}
}

type encstate struct {
	Meta map[string][]byte `json:"meta"`
	Data []byte            `json:"state"`
}

func (s *stateEncryption) EncryptState(data []byte) ([]byte, hcl.Diagnostics) {
	es := encstate{
		Meta: make(map[string][]byte),
	}

	// Mutates es.Meta
	enc, diags := s.f.inst(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	primary, _, tdiags := enc.setupTarget(s.t, s.name)
	diags = append(diags, tdiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	encd, err := primary.Encrypt(data)
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

func (s *stateEncryption) DecryptState(data []byte) ([]byte, hcl.Diagnostics) {
	es := encstate{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		// TODO diags
		panic(err)
	}

	enc, diags := s.f.inst(es.Meta)
	if diags.HasErrors() {
		return nil, diags
	}

	primary, _, tdiags := enc.setupTarget(s.t, s.name)
	diags = append(diags, tdiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	uncd, err := primary.Decrypt(es.Data)
	if err != nil {
		// TODO diags
		panic(err)
	}
	return uncd, diags

}
