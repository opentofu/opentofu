// Package static contains a key provider that emits a static key.
package static

type staticKeyProvider struct {
	key []byte
}

func (p staticKeyProvider) Provide() ([]byte, error) {
	return p.key, nil
}
