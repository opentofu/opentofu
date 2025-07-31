// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package openbao

import (
	"fmt"

	openbao "github.com/openbao/openbao/api/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type Config struct {
	Address string `hcl:"address,optional"`
	Token   string `hcl:"token,optional"`

	KeyName           string        `hcl:"key_name"`
	KeyLength         DataKeyLength `hcl:"key_length,optional"`
	TransitEnginePath string        `hcl:"transit_engine_path,optional"`
}

const (
	defaultDataKeyLength     DataKeyLength = 32
	defaultTransitEnginePath string        = "/transit"
)

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.KeyName == "" {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "no key name found",
		}
	}

	if c.KeyLength == 0 {
		c.KeyLength = defaultDataKeyLength
	}

	if err := c.KeyLength.Validate(); err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: err,
		}
	}

	if c.TransitEnginePath == "" {
		c.TransitEnginePath = defaultTransitEnginePath
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
		svc: service{
			c:           client,
			transitPath: c.TransitEnginePath,
		},
		keyName:   c.KeyName,
		keyLength: c.KeyLength,
	}, new(keyMeta), nil
}

type DataKeyLength int

func (l DataKeyLength) Validate() error {
	switch l {
	case 16, 32, 64:
		return nil
	default:
		return fmt.Errorf("data key length should one of 16, 32 or 64 bytes: got %v", l)
	}
}

func (l DataKeyLength) Bits() int {
	return int(l) * 8
}

type clientConstructor func(config *openbao.Config, token string) (client, error)

// newClient variable allows to inject different client implementations.
// In order to keep client interface simple, token setting is in this function as well.
// It's not possible to pass token in config.
var newClient clientConstructor = newOpenBaoClient

func newOpenBaoClient(config *openbao.Config, token string) (client, error) {
	// NewClient reads BAO_TOKEN and some other optional env variables.
	c, err := openbao.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenBao client: %w", err)
	}

	// Token from HCL supersedes BAO_TOKEN.
	if token != "" {
		c.SetToken(token)
	}

	return c.Logical(), nil
}
