package encryption

import "github.com/opentofu/opentofu/internal/encryption/method"

type PlanEncryption interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptPlan([]byte) ([]byte, error)
}

type planEncryption struct {
	methods []method.Method
}

func NewPlan(methods []method.Method) PlanEncryption {
	return &planEncryption{methods}
}

func (p planEncryption) EncryptPlan(input []byte) ([]byte, error) {
	return p.methods[0].Encrypt(input)
}

func (p planEncryption) DecryptPlan(input []byte) ([]byte, error) {
	return p.methods[0].Decrypt(input)
}
