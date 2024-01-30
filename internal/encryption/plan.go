package encryption

type Plan interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptState([]byte) ([]byte, error)
}
