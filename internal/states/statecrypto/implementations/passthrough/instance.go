package passthrough

import "github.com/placeholderplaceholderplaceholder/opentf/internal/states/statecrypto/cryptoconfig"

func New(_ cryptoconfig.StateCryptoConfig) (*PassthroughStateWrapper, error) {
	return &PassthroughStateWrapper{}, nil
}
