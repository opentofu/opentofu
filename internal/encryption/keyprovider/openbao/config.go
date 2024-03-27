package openbao

import (
	"context"
	"errors"
	"fmt"

	openbao "github.com/openbao/openbao/api"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// TODO: compliance tests

// TODO: script to run bao for tests

type Config struct {
	Address string `hcl:"address,optional"`
	Token   string `hcl:"token,optional"`

	KeyName        string `hcl:"key_name"`
	DataKeyBitSize int    `hcl:"data_key_bit_size,optional"`
}

const defaultDataKeyBitSize = 256

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.DataKeyBitSize == 0 {
		c.DataKeyBitSize = defaultDataKeyBitSize
	}

	// DefaultConfig reads BAO_ADDR and some other optional env variables.
	config := openbao.DefaultConfig()
	if config.Error != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: config.Error,
		}
	}

	// Address from HCL supersedes BAO_ADDR.
	if c.Address != "" {
		config.Address = c.Address
	}

	client, err := newClient(config, c.Token)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: err,
		}
	}

	return &keyProvider{
		svc:            service{client},
		keyName:        c.KeyName,
		dataKeyBitSize: c.DataKeyBitSize,
		ctx:            context.Background(),
	}, new(keyMeta), nil
}

type clientConstructor func(config *openbao.Config, token string) (client, error)

var errNoOpenBaoTokenFound = errors.New("no openbao token found")

// newClient variable allows to inject different client implementations.
var newClient clientConstructor = func(config *openbao.Config, token string) (client, error) {
	// NewClient reads BAO_TOKEN and some other optional env variables.
	c, err := openbao.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("creating openbao client: %w", err)
	}

	// Token from HCL supersedes BAO_TOKEN.
	if token != "" {
		c.SetToken(token)
	} else if c.Token() == "" {
		return nil, errNoOpenBaoTokenFound
	}

	return c.Logical(), nil
}
