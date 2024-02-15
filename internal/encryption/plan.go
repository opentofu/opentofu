package encryption

type PlanEncryption interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptPlan([]byte) ([]byte, error)
}

type planEncryption struct {
}

func NewPlan(f *encryption, t *EnforcableTargetConfig, name string) PlanEncryption {
	return &planEncryption{}
}

func (p planEncryption) EncryptPlan(input []byte) ([]byte, error) {
	// TODO: Implement fallback logic
	return nil, nil //p.method.Encrypt(input)
}

func (p planEncryption) DecryptPlan(input []byte) ([]byte, error) {
	// TODO: Implement fallback logic
	return nil, nil //p.method.Decrypt(input)
}
