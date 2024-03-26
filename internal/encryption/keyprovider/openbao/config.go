package openbao

import (
	"context"

	openbao "github.com/openbao/openbao/api"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// TODO: compliance tests

// TODO: docs

// TODO: script to run bao for tests

// TODO: add datakey_bit_size

type Config struct {
	// TODO: check more optional fields to add
	// TODO: check if there are different auth options
	Address string `hcl:"address,optional"`
	Token   string `hcl:"token,optional"`
	KeyName string `hcl:"key_name"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {

	// TODO: read from env / check what is read by openbao package

	config := openbao.DefaultConfig()

	config.Address = c.Address

	// TODO: validation of input

	client, err := openbao.NewClient(config)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Cause: err} // TODO: check errors usage
	}

	// TODO: check for another way of setting a token
	client.SetToken(c.Token)

	// TODO: raw client injection

	return &keyProvider{
		svc:     service{rawClient{client}},
		keyName: c.KeyName,
		ctx:     context.Background(),
	}, new(keyMeta), nil
}
