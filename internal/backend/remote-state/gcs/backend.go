// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package gcs implements remote storage of state on Google Cloud Storage (GCS).
package gcs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/version"
	"golang.org/x/oauth2"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

// Backend implements "backend".Backend for GCS.
// Input(), Validate() and Configure() are implemented by embedding *schema.Backend.
// State(), DeleteState() and States() are implemented explicitly.
type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	storageClient *storage.Client

	bucketName string
	prefix     string

	encryptionKey []byte
	kmsKeyName    string
}

func New(enc encryption.StateEncryption) backend.Backend {
	b := &Backend{encryption: enc}
	b.Backend = &schema.Backend{
		ConfigureFunc: b.configure,
		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the Google Cloud Storage bucket",
			},

			"prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The directory where state files will be saved inside the bucket",
			},

			"credentials": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Google Cloud JSON Account Key",
				Default:     "",
			},

			"access_token": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_OAUTH_ACCESS_TOKEN",
				}, nil),
				Description: "An OAuth2 token used for GCP authentication",
			},

			"impersonate_service_account": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
					"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
				}, nil),
				Description: "The service account to impersonate for all Google API Calls",
			},

			"impersonate_service_account_delegates": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "The delegation chain for the impersonated service account",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},

			"encryption_key": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_ENCRYPTION_KEY",
				}, nil),
				Description:   "A 32 byte base64 encoded 'customer supplied encryption key' used when reading and writing state files in the bucket.",
				ConflictsWith: []string{"kms_encryption_key"},
			},

			"kms_encryption_key": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_KMS_ENCRYPTION_KEY",
				}, nil),
				Description:   "A Cloud KMS key ('customer managed encryption key') used when reading and writing state files in the bucket. Format should be 'projects/{{project}}/locations/{{location}}/keyRings/{{keyRing}}/cryptoKeys/{{name}}'.",
				ConflictsWith: []string{"encryption_key"},
			},

			"storage_custom_endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_BACKEND_STORAGE_CUSTOM_ENDPOINT",
					"GOOGLE_STORAGE_CUSTOM_ENDPOINT",
				}, nil),
			},
		},
	}

	return b
}

func (b *Backend) configure(ctx context.Context) error {
	if b.storageClient != nil {
		return nil
	}

	data := schema.FromContextBackendConfig(ctx)

	b.bucketName = data.Get("bucket").(string)
	b.prefix = strings.TrimLeft(data.Get("prefix").(string), "/")
	if b.prefix != "" && !strings.HasSuffix(b.prefix, "/") {
		b.prefix = b.prefix + "/"
	}

	var opts []option.ClientOption
	var credOptions []option.ClientOption

	// Add credential source
	var creds string
	var tokenSource oauth2.TokenSource

	if v, ok := data.GetOk("access_token"); ok {
		tokenSource = oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: v.(string),
		})
	} else if v, ok := data.GetOk("credentials"); ok {
		creds = v.(string)
	} else if v := os.Getenv("GOOGLE_BACKEND_CREDENTIALS"); v != "" {
		creds = v
	} else {
		creds = os.Getenv("GOOGLE_CREDENTIALS")
	}

	if tokenSource != nil {
		credOptions = append(credOptions, option.WithTokenSource(tokenSource))
	} else if creds != "" {

		// to mirror how the provider works, we accept the file path or the contents
		contents, err := backend.ReadPathOrContents(creds)
		if err != nil {
			return fmt.Errorf("Error loading credentials: %w", err)
		}

		if !json.Valid([]byte(contents)) {
			return fmt.Errorf("the string provided in credentials is neither valid json nor a valid file path")
		}

		credOptions = append(credOptions, option.WithCredentialsJSON([]byte(contents)))
	}

	// Service Account Impersonation
	if v, ok := data.GetOk("impersonate_service_account"); ok {
		ServiceAccount := v.(string)
		var delegates []string

		if v, ok := data.GetOk("impersonate_service_account_delegates"); ok {
			d := v.([]interface{})
			if len(delegates) > 0 {
				delegates = make([]string, 0, len(d))
			}
			for _, delegate := range d {
				delegates = append(delegates, delegate.(string))
			}
		}

		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: ServiceAccount,
			Scopes:          []string{storage.ScopeReadWrite},
			Delegates:       delegates,
		}, credOptions...)

		if err != nil {
			return err
		}

		opts = append(opts, option.WithTokenSource(ts))

	} else {
		opts = append(opts, credOptions...)
	}

	opts = append(opts, option.WithUserAgent(httpclient.OpenTofuUserAgent(version.Version)))

	// Custom endpoint for storage API
	if storageEndpoint, ok := data.GetOk("storage_custom_endpoint"); ok {
		endpoint := option.WithEndpoint(storageEndpoint.(string))
		opts = append(opts, endpoint)
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("storage.NewClient() failed: %w", err)
	}

	b.storageClient = client

	// Customer-supplied encryption
	key := data.Get("encryption_key").(string)
	if key != "" {
		kc, err := backend.ReadPathOrContents(key)
		if err != nil {
			return fmt.Errorf("Error loading encryption key: %w", err)
		}

		// The GCS client expects a customer supplied encryption key to be
		// passed in as a 32 byte long byte slice. The byte slice is base64
		// encoded before being passed to the API. We take a base64 encoded key
		// to remain consistent with the GCS docs.
		// https://cloud.google.com/storage/docs/encryption#customer-supplied
		// https://github.com/GoogleCloudPlatform/google-cloud-go/blob/def681/storage/storage.go#L1181
		k, err := base64.StdEncoding.DecodeString(kc)
		if err != nil {
			return fmt.Errorf("Error decoding encryption key: %w", err)
		}
		b.encryptionKey = k
	}

	// Customer-managed encryption
	kmsName := data.Get("kms_encryption_key").(string)
	if kmsName != "" {
		b.kmsKeyName = kmsName
	}

	return nil
}
