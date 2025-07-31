// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"address": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_ADDRESS", nil),
				Description: "The address of the REST endpoint",
			},
			"update_method": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_UPDATE_METHOD", "POST"),
				Description: "HTTP method to use when updating state",
			},
			"lock_address": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_LOCK_ADDRESS", nil),
				Description: "The address of the lock REST endpoint",
			},
			"unlock_address": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_UNLOCK_ADDRESS", nil),
				Description: "The address of the unlock REST endpoint",
			},
			"lock_method": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_LOCK_METHOD", "LOCK"),
				Description: "The HTTP method to use when locking",
			},
			"unlock_method": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_UNLOCK_METHOD", "UNLOCK"),
				Description: "The HTTP method to use when unlocking",
			},
			"username": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_USERNAME", nil),
				Description: "The username for HTTP basic authentication",
			},
			"password": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_PASSWORD", nil),
				Description: "The password for HTTP basic authentication",
			},
			"skip_cert_verification": &schema.Schema{
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether to skip TLS verification.",
			},
			"retry_max": &schema.Schema{
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_RETRY_MAX", 2),
				Description: "The number of HTTP request retries.",
			},
			"retry_wait_min": &schema.Schema{
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_RETRY_WAIT_MIN", 1),
				Description: "The minimum time in seconds to wait between HTTP request attempts.",
			},
			"retry_wait_max": &schema.Schema{
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_RETRY_WAIT_MAX", 30),
				Description: "The maximum time in seconds to wait between HTTP request attempts.",
			},
			"client_ca_certificate_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_CLIENT_CA_CERTIFICATE_PEM", ""),
				Description: "A PEM-encoded CA certificate chain used by the client to verify server certificates during TLS authentication.",
			},
			"client_certificate_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_CLIENT_CERTIFICATE_PEM", ""),
				Description: "A PEM-encoded certificate used by the server to verify the client during mutual TLS (mTLS) authentication.",
			},
			"client_private_key_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_HTTP_CLIENT_PRIVATE_KEY_PEM", ""),
				Description: "A PEM-encoded private key, required if client_certificate_pem is specified.",
			},
			"headers": &schema.Schema{
				Type:     schema.TypeMap,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				ValidateFunc: func(cv interface{}, ck string) ([]string, []error) {
					nameRegex := regexp.MustCompile("[^a-zA-Z0-9-_]")
					valueRegex := regexp.MustCompile("[^[:ascii:]]")

					headers := cv.(map[string]interface{})
					err := make([]error, 0, len(headers))
					for name, value := range headers {
						if len(name) == 0 || nameRegex.MatchString(name) {
							err = append(err, fmt.Errorf(
								"%s \"%s\" name must not be empty and only contain A-Za-z0-9-_ characters", ck, name))
						}

						v := value.(string)
						if len(strings.TrimSpace(v)) == 0 || valueRegex.MatchString(v) {
							err = append(err, fmt.Errorf(
								"%s \"%s\" value must not be empty and only contain ascii characters", ck, name))
						}
					}
					return nil, err
				},
				Description: "A map of headers, when set will be included with HTTP requests sent to the HTTP backend",
			},
		},
	}

	b := &Backend{Backend: s, encryption: enc}
	b.Backend.ConfigureFunc = b.configure
	return b
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	client *httpClient
}

// configureTLS configures TLS when needed; if there are no conditions requiring TLS, no change is made.
func (b *Backend) configureTLS(client *retryablehttp.Client, data *schema.ResourceData) error {
	// If there are no conditions needing to configure TLS, leave the client untouched
	skipCertVerification := data.Get("skip_cert_verification").(bool)
	clientCACertificatePem := data.Get("client_ca_certificate_pem").(string)
	clientCertificatePem := data.Get("client_certificate_pem").(string)
	clientPrivateKeyPem := data.Get("client_private_key_pem").(string)
	if !skipCertVerification && clientCACertificatePem == "" && clientCertificatePem == "" && clientPrivateKeyPem == "" {
		return nil
	}
	if clientCertificatePem != "" && clientPrivateKeyPem == "" {
		return fmt.Errorf("client_certificate_pem is set but client_private_key_pem is not")
	}
	if clientPrivateKeyPem != "" && clientCertificatePem == "" {
		return fmt.Errorf("client_private_key_pem is set but client_certificate_pem is not")
	}

	// TLS configuration is needed; create an object and configure it
	var tlsConfig tls.Config
	client.HTTPClient.Transport.(*http.Transport).TLSClientConfig = &tlsConfig

	if skipCertVerification {
		// ignores TLS verification
		tlsConfig.InsecureSkipVerify = true
	}
	if clientCACertificatePem != "" {
		// trust servers based on a CA
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM([]byte(clientCACertificatePem)) {
			return errors.New("failed to append certs")
		}
	}
	if clientCertificatePem != "" && clientPrivateKeyPem != "" {
		// attach a client certificate to the TLS handshake (aka mTLS)
		certificate, err := tls.X509KeyPair([]byte(clientCertificatePem), []byte(clientPrivateKeyPem))
		if err != nil {
			return fmt.Errorf("cannot load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	return nil
}

func (b *Backend) configure(ctx context.Context) error {
	data := schema.FromContextBackendConfig(ctx)

	address := data.Get("address").(string)
	updateURL, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("failed to parse address URL: %w", err)
	}
	if updateURL.Scheme != "http" && updateURL.Scheme != "https" {
		return fmt.Errorf("address must be HTTP or HTTPS")
	}

	updateMethod := data.Get("update_method").(string)

	var lockURL *url.URL
	if v, ok := data.GetOk("lock_address"); ok && v.(string) != "" {
		var err error
		lockURL, err = url.Parse(v.(string))
		if err != nil {
			return fmt.Errorf("failed to parse lockAddress URL: %w", err)
		}
		if lockURL.Scheme != "http" && lockURL.Scheme != "https" {
			return fmt.Errorf("lockAddress must be HTTP or HTTPS")
		}
	}

	lockMethod := data.Get("lock_method").(string)

	var unlockURL *url.URL
	if v, ok := data.GetOk("unlock_address"); ok && v.(string) != "" {
		var err error
		unlockURL, err = url.Parse(v.(string))
		if err != nil {
			return fmt.Errorf("failed to parse unlockAddress URL: %w", err)
		}
		if unlockURL.Scheme != "http" && unlockURL.Scheme != "https" {
			return fmt.Errorf("unlockAddress must be HTTP or HTTPS")
		}
	}

	unlockMethod := data.Get("unlock_method").(string)

	username := data.Get("username").(string)
	password := data.Get("password").(string)

	var headers map[string]string
	if dv, ok := data.GetOk("headers"); ok {
		dh := dv.(map[string]interface{})
		headers = make(map[string]string, len(dh))

		for k, v := range dh {
			value, ok := v.(string)
			if !ok {
				return fmt.Errorf("header value for %s must be a string", k)
			}
			switch strings.ToLower(k) {
			case "authorization":
				if username != "" {
					return fmt.Errorf("headers \"%s\" cannot be set when providing username", k)
				}
				headers[k] = value
			case "content-type", "content-md5":
				return fmt.Errorf("headers \"%s\" is reserved", k)
			default:
				headers[k] = value
			}
		}
	}

	rClient := retryablehttp.NewClient()
	rClient.RetryMax = data.Get("retry_max").(int)
	rClient.RetryWaitMin = time.Duration(data.Get("retry_wait_min").(int)) * time.Second
	rClient.RetryWaitMax = time.Duration(data.Get("retry_wait_max").(int)) * time.Second
	rClient.Logger = log.New(logging.LogOutput(), "", log.Flags())
	if err = b.configureTLS(rClient, data); err != nil {
		return err
	}

	b.client = &httpClient{
		URL:          updateURL,
		UpdateMethod: updateMethod,

		LockURL:      lockURL,
		LockMethod:   lockMethod,
		UnlockURL:    unlockURL,
		UnlockMethod: unlockMethod,

		Headers:  headers,
		Username: username,
		Password: password,

		// accessible only for testing use
		Client: rClient,
	}
	return nil
}

func (b *Backend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	if name != backend.DefaultStateName {
		return nil, backend.ErrWorkspacesNotSupported
	}

	return remote.NewState(b.client, b.encryption), nil
}

func (b *Backend) Workspaces(context.Context) ([]string, error) {
	return nil, backend.ErrWorkspacesNotSupported
}

func (b *Backend) DeleteWorkspace(context.Context, string, bool) error {
	return backend.ErrWorkspacesNotSupported
}
