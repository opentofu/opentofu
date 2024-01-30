package encryption

type Plan interface {
	EncryptPlan([]byte) ([]byte, error)
	DecryptState([]byte) ([]byte, error)
}

func NewPlan(methods []Method) Plan {
	// TODO
	return nil
}
