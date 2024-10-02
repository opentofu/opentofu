package gitlab

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/internal/logging"
	"golang.org/x/oauth2"
)

func New(enc encryption.StateEncryption) backend.Backend {
	var (
		defaultAddressEnvVars = []string{"TF_GITLAB_ADDRESS", "CI_SERVER_URL"}
		defaultProjectEnvVars = []string{"TF_GITLAB_PROJECT", "CI_PROJECT_ID"}
		defaultTokenEnvVars   = []string{"TF_GITLAB_TOKEN", "CI_JOB_TOKEN"}
	)

	const (
		defaultRetryMax     = 2
		defaultRetryWaitMin = 1
		defaultRetryWaitMax = 30
	)

	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"address": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.MultiEnvDefaultFunc(defaultAddressEnvVars, nil),
				Description: "The Gitlab URL.",
			},
			"project": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.MultiEnvDefaultFunc(defaultProjectEnvVars, nil),
				Description: "The Gitlab project path or ID number.",
			},
			"token": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.MultiEnvDefaultFunc(defaultTokenEnvVars, nil),
				Description: "The Gitlab access token to manage remote state.",
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
				DefaultFunc: schema.EnvDefaultFunc("TF_GITLAB_RETRY_MAX", defaultRetryMax),
				Description: "The number of HTTP request retries.",
			},
			"retry_wait_min": &schema.Schema{
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_GITLAB_RETRY_WAIT_MIN", defaultRetryWaitMin),
				Description: "The minimum time in seconds to wait between HTTP request attempts.",
			},
			"retry_wait_max": &schema.Schema{
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TF_GITLAB_RETRY_WAIT_MAX", defaultRetryWaitMax),
				Description: "The maximum time in seconds to wait between HTTP request attempts.",
			},
		},
	}

	b := &Backend{Backend: s, encryption: enc}
	b.Backend.ConfigureFunc = b.configure
	return b
}

type Backend struct {
	encryption encryption.StateEncryption
	client     *RemoteClient

	address *url.URL
	project string
	token   string

	httpClient *retryablehttp.Client

	*schema.Backend
}

func (b *Backend) configure(ctx context.Context) error {
	data := schema.FromContextBackendConfig(ctx)
	var err error

	var (
		address      = data.Get("address").(string)
		project      = data.Get("project").(string)
		token        = data.Get("token").(string)
		retryMax     = data.Get("retry_max").(int)
		retryWaitMin = data.Get("retry_wait_min").(int)
		retryWaitMax = data.Get("retry_wait_max").(int)

		skipCertVerification = data.Get("skip_cert_verification").(bool)
	)

	if b.address, err = url.Parse(address); err != nil {
		return fmt.Errorf("could not to parse gitlab address %s: %w", address, err)
	} else if b.address.Scheme != "http" && b.address.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}

	if b.project = project; project == "" {
		return fmt.Errorf("project must not be empty")
	}

	if b.token = token; token == "" {
		return fmt.Errorf("token must not be empty")
	}

	b.httpClient = retryablehttp.NewClient()

	// get original transport so we can add authentication
	originalTransport := b.httpClient.HTTPClient.Transport

	// optionally skip tls certificate validation
	if transport, ok := originalTransport.(*http.Transport); ok {
		if skipCertVerification {
			transport.TLSClientConfig.InsecureSkipVerify = true
		}
	}

	// build an http client with retries and authentication
	b.httpClient.HTTPClient.Transport = &oauth2.Transport{
		Base: originalTransport,
		Source: oauth2.StaticTokenSource(&oauth2.Token{
			TokenType:   "Bearer",
			AccessToken: b.token,
		}),
	}

	// set up logging to match the configured log level and flags
	b.httpClient.Logger = log.New(logging.LogOutput(), "", log.Flags())

	// set up retries like the http backend
	b.httpClient.RetryMax = retryMax
	b.httpClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Second
	b.httpClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Second

	// build a remote client for the default state
	b.client = b.remoteClientFor(backend.DefaultStateName)

	return nil
}

func (b *Backend) remoteClientFor(stateName string) *RemoteClient {
	return &RemoteClient{
		HTTPClient: b.httpClient,
		BaseURL:    b.address,
		Project:    b.project,
		StateName:  stateName,
	}
}
