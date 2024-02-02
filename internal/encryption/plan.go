package encryption

import "github.com/opentofu/opentofu/internal/encryption/method"

type PlanEncryption interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptPlan([]byte) ([]byte, error)
}

func NewPlan(methods []method.Method) PlanEncryption {
	// TODO
	return nil
}
