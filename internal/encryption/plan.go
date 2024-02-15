package encryption

import "github.com/hashicorp/hcl/v2"

// This currently deals with raw bytes so it could be moved into it's own library and not depend explicitly on the OpenTofu codestate.
type PlanEncryption interface {
	EncryptPlan([]byte) ([]byte, hcl.Diagnostics)
	DecryptPlan([]byte) ([]byte, hcl.Diagnostics)
}

type planEncryption struct {
	base *baseEncryption
}

func NewPlan(enc *encryption, target *EnforcableTargetConfig, name string) PlanEncryption {
	return &planEncryption{
		base: newBaseEncryption(enc, target.AsTargetConfig(), target.Enforced, name),
	}
}

func (p planEncryption) EncryptPlan(data []byte) ([]byte, hcl.Diagnostics) {
	return p.base.encrypt(data)
}

func (p planEncryption) DecryptPlan(data []byte) ([]byte, hcl.Diagnostics) {
	return p.base.decrypt(data)
}
