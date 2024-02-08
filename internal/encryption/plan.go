package encryption

import "github.com/opentofu/opentofu/internal/encryption/method"

type PlanEncryption interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptPlan([]byte) ([]byte, error)
}

type planEncryption struct {
	method   method.Method
	fallback method.Method
}

func NewPlan(method method.Method, fallback method.Method) PlanEncryption {
	return &planEncryption{method, fallback}
}

func (p planEncryption) EncryptPlan(input []byte) ([]byte, error) {
	// TODO: Implement fallback logic
	return p.method.Encrypt(input)
}

func (p planEncryption) DecryptPlan(input []byte) ([]byte, error) {
	// TODO: Implement fallback logic
	return p.method.Decrypt(input)
}
