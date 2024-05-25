// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"net"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
)

// New creates a new backend for Consul remote state.
func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"mount": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: "KVv2 mount name in Vault",
			},

			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: "Secret base path to store state in Vault",
			},

			"vault_token": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Token for Vault",
				Default:     "", // To prevent input
			},

			"address": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: "HTTP URL to the Consul Cluster",
				Default:     "", // To prevent input
			},

			// "scheme": &schema.Schema{
			// 	Type:        schema.TypeString,
			// 	Optional:    true,
			// 	Description: "Scheme to communicate to Consul with",
			// 	Default:     "", // To prevent input
			// },

			"gzip": &schema.Schema{
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Compress the state data using gzip",
				Default:     false,
			},

			"lock": &schema.Schema{
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Lock state access",
				Default:     true,
			},

			"ca_file": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A path to a PEM-encoded certificate authority used to verify the remote agent's certificate.",
				DefaultFunc: schema.EnvDefaultFunc("CONSUL_CACERT", ""),
			},

			// "cert_file": &schema.Schema{
			// 	Type:        schema.TypeString,
			// 	Optional:    true,
			// 	Description: "A path to a PEM-encoded certificate provided to the remote agent; requires use of key_file.",
			// 	DefaultFunc: schema.EnvDefaultFunc("CONSUL_CLIENT_CERT", ""),
			// },

			// "key_file": &schema.Schema{
			// 	Type:        schema.TypeString,
			// 	Optional:    true,
			// 	Description: "A path to a PEM-encoded private key, required if cert_file is specified.",
			// 	DefaultFunc: schema.EnvDefaultFunc("CONSUL_CLIENT_KEY", ""),
			// },
		},
	}

	result := &Backend{Backend: s, encryption: enc}
	result.Backend.ConfigureFunc = result.configure
	return result
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	// The fields below are set from configure
	client     *vaultapi.Client
	configData *schema.ResourceData
	lock       bool
	token      string
	mount      string
}

func (b *Backend) configure(ctx context.Context) error {
	// Grab the resource data
	b.configData = schema.FromContextBackendConfig(ctx)

	// Store the lock information
	b.lock = b.configData.Get("lock").(bool)

	b.mount = b.configData.Get("mount").(string)

	data := b.configData

	// Configure the client
	config := vaultapi.DefaultConfig()

	// replace the default Transport Dialer to reduce the KeepAlive
	// config.Transport.DialContext = dialContext

	if v, ok := data.GetOk("vault_token"); ok && v.(string) != "" {
		b.token = v.(string)
	}
	if v, ok := data.GetOk("address"); ok && v.(string) != "" {
		config.Address = v.(string)
	}

	// if v, ok := data.GetOk("datacenter"); ok && v.(string) != "" {
	// 	config.Datacenter = v.(string)
	// }

	// if v, ok := data.GetOk("ca_file"); ok && v.(string) != "" {
	// 	config.TLSConfig.CAFile = v.(string)
	// }
	// if v, ok := data.GetOk("cert_file"); ok && v.(string) != "" {
	// 	config.TLSConfig.CertFile = v.(string)
	// }
	// if v, ok := data.GetOk("key_file"); ok && v.(string) != "" {
	// 	config.TLSConfig.KeyFile = v.(string)
	// }

	client, err := vaultapi.NewClient(config)
	if err != nil {
		return err
	}

	client.SetToken(b.token)

	b.client = client
	return nil
}

// dialContext is the DialContext function for the consul client transport.
// This is stored in a package var to inject a different dialer for tests.
var dialContext = (&net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 17 * time.Second,
}).DialContext
