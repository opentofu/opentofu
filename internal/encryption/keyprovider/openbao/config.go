package openbao

import (
	"context"
	"errors"
	"fmt"

	openbao "github.com/openbao/openbao/api"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type Config struct {
	Address string `hcl:"address,optional"`
	Token   string `hcl:"token,optional"`

	KeyName   string `hcl:"key_name"`
	KeyLength int    `hcl:"key_length,optional"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.KeyName == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "no key name found",
		}
	}

	if c.KeyLength == 0 {
		c.KeyLength = defaultKeyLength
	}

	if err := validateKeyLength(c.KeyLength); err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: err,
		}
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
		svc:       service{client},
		keyName:   c.KeyName,
		keyLength: c.KeyLength,
		ctx:       context.Background(),
	}, new(keyMeta), nil
}

type clientConstructor func(config *openbao.Config, token string) (client, error)

var errNoOpenBaoTokenFound = errors.New("no openbao token found")

// newClient variable allows to inject different client implementations.
// In order to keep client interface simple, token setting is in this function as well.
// It's not possible to pass token in config.
var newClient clientConstructor = newOpenBaoClient

func newOpenBaoClient(config *openbao.Config, token string) (client, error) {
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

const defaultKeyLength = 32

func validateKeyLength(keyLength int) error {
	if keyLength != 16 && keyLength != 32 && keyLength != 64 {
		return fmt.Errorf("invalid key length: %d, supported options are 16, 32 or 64 bytes", keyLength)
	}

	return nil
}
