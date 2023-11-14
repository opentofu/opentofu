package vault

import (
	"context"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
)

// New creates a new backend for Vault remote state.
func New() backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"path": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Path to store state in Vault",
			},

			"access_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Access token for a Vault ACL",
				Default:     "", // To prevent input
			},

			"address": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Address to the Vault Cluster (scheme://host:port)",
				Default:     "", // To prevent input
			},

			"mount": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Default mount path",
				Default:     "secret",
			},

			"gzip": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Compress the state data using gzip",
				Default:     false,
			},

			"lock": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Lock state access",
				Default:     true,
			},

			"ca_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A path to a PEM-encoded certificate authority used to verify the remote agent's certificate.",
				DefaultFunc: schema.EnvDefaultFunc("VAULT_CACERT", ""),
			},

			"cert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A path to a PEM-encoded certificate provided to the remote agent; requires use of key_file.",
				DefaultFunc: schema.EnvDefaultFunc("VAULT_CLIENT_CERT", ""),
			},

			"key_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A path to a PEM-encoded private key, required if cert_file is specified.",
				DefaultFunc: schema.EnvDefaultFunc("VAULT_CLIENT_KEY", ""),
			},
		},
	}

	result := &Backend{Backend: s}
	result.Backend.ConfigureFunc = result.configure
	return result
}

type Backend struct {
	*schema.Backend

	// The fields below are set from configure
	ctx        context.Context
	client     *vault.Client
	configData *schema.ResourceData
	lock       bool
}

func (b *Backend) configure(ctx context.Context) error {
	// Grab the resource data
	b.configData = schema.FromContextBackendConfig(ctx)
	b.ctx = ctx

	// Store the lock information
	b.lock = b.configData.Get("lock").(bool)

	data := b.configData

	// Configure the client
	config := vault.DefaultConfiguration()

	config.RequestTimeout = 30 * time.Second

	if v, ok := data.GetOk("address"); ok && v.(string) != "" {
		config.Address = v.(string)
	}

	if v, ok := data.GetOk("ca_file"); ok && v.(string) != "" {
		config.TLS.ServerCertificate.FromFile = v.(string)
	}
	if v, ok := data.GetOk("cert_file"); ok && v.(string) != "" {
		config.TLS.ClientCertificate.FromFile = v.(string)
	}
	if v, ok := data.GetOk("key_file"); ok && v.(string) != "" {
		config.TLS.ClientCertificateKey.FromFile = v.(string)
	}

	client, err := vault.New(vault.WithConfiguration(config), vault.WithEnvironment())
	if err != nil {
		return err
	}

	if v, ok := data.GetOk("access_token"); ok && v.(string) != "" {
		if err = client.SetToken(v.(string)); err != nil {
			return err
		}
	}

	b.client = client
	return nil
}
