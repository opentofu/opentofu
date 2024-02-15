package encryption

import (
	"encoding/json"

	"github.com/hashicorp/hcl/v2"
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
	es := basedata{
		Meta: make(map[string][]byte),
	}

	// Mutates es.Meta
	methods, diags := s.buildTargetMethods(es.Meta)
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

func (s *baseEncryption) decrypt(data []byte) ([]byte, hcl.Diagnostics) {
	es := basedata{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		// TODO diags
		panic(err)
	}

	methods, diags := s.buildTargetMethods(es.Meta)
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
