package encryption

type PassphraseKeyProvider struct {
	Passphrase string `hcl:"passphrase"`
}

func (p PassphraseKeyProvider) KeyData() ([]byte, error) {
	return []byte(p.Passphrase), nil
}
