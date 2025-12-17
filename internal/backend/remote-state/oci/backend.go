package oci

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	orasRegistry "oras.land/oras-go/v2/registry"
	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"
)

const envVarRepository = "TF_BACKEND_OCI_REPOSITORY"

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	repository string
	insecure   bool
	caFile     string

	ociCredsPolicy cliconfigOCICredentialsPolicy
	repoClient     *ociRepositoryClient
}

func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"repository": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "OCI repository in the form <registry>/<repository>, without tag or digest. Can also be set via TF_BACKEND_OCI_REPOSITORY env var.",
				DefaultFunc: schema.EnvDefaultFunc(envVarRepository, ""),
			},
			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Skip TLS certificate verification when communicating with the OCI registry",
			},
			"ca_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Path to a PEM-encoded CA certificate bundle to trust when communicating with the OCI registry",
			},
		},
	}

	b := &Backend{Backend: s, encryption: enc}
	b.Backend.ConfigureFunc = b.configure
	return b
}

func (b *Backend) configure(ctx context.Context) error {
	data := schema.FromContextBackendConfig(ctx)

	repository := data.Get("repository").(string)
	if repository == "" {
		return fmt.Errorf("repository must not be empty (set via config or %s)", envVarRepository)
	}

	ref, err := orasRegistry.ParseReference(repository)
	if err != nil {
		return err
	}
	if ref.Reference != "" {
		return fmt.Errorf("repository must not include a tag or digest")
	}

	b.repository = repository
	b.insecure = data.Get("insecure").(bool)
	b.caFile = data.Get("ca_file").(string)

	cfg, diags := cliconfig.LoadConfig(ctx)
	if diags.HasErrors() {
		return diags.Err()
	}
	policy, err := cfg.OCICredentialsPolicy(ctx)
	if err != nil {
		return err
	}
	b.ociCredsPolicy = realOCICredentialsPolicy{policy: policy}

	b.repoClient, err = newOCIRepositoryClient(ctx, b.repository, b.insecure, b.caFile, b.ociCredsPolicy)
	return err
}

func (b *Backend) getRepository() (*ociRepositoryClient, error) {
	if b.repoClient == nil {
		return nil, fmt.Errorf("backend is not configured")
	}
	return b.repoClient, nil
}

// OCI repository client

type cliconfigOCICredentialsPolicy interface {
	CredentialFunc(ctx context.Context, repository string) (credentialFunc, error)
}

type credentialFunc func(ctx context.Context, hostport string) (orasAuth.Credential, error)

type ociRepositoryClient struct {
	inner ociRepository
}

type ociRepository interface {
	Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)
	Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error
	Delete(ctx context.Context, target ocispec.Descriptor) error
	Exists(ctx context.Context, target ocispec.Descriptor) (bool, error)
	Tags(ctx context.Context, last string, fn func(tags []string) error) error
}

func newOCIRepositoryClient(ctx context.Context, repository string, insecure bool, caFile string, policy cliconfigOCICredentialsPolicy) (*ociRepositoryClient, error) {
	repo, err := orasRemote.NewRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI repository %q: %w", repository, err)
	}

	httpClient, err := newOCIHTTPClient(ctx, insecure, caFile)
	if err != nil {
		return nil, err
	}

	credFunc, err := policy.CredentialFunc(ctx, repository)
	if err != nil {
		return nil, err
	}

	repo.Client = &orasAuth.Client{
		Client: httpClient,
		Credential: func(ctx context.Context, _ string) (orasAuth.Credential, error) {
			return credFunc(ctx, repo.Reference.Registry)
		},
	}

	return &ociRepositoryClient{inner: repo}, nil
}

func newOCIHTTPClient(ctx context.Context, insecure bool, caFile string) (*http.Client, error) {
	client := cleanhttp.DefaultPooledClient()

	if t, ok := client.Transport.(*http.Transport); ok {
		t = t.Clone()
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.InsecureSkipVerify = insecure
		if caFile != "" {
			pem, err := os.ReadFile(caFile)
			if err != nil {
				return nil, fmt.Errorf("reading ca_file %q: %w", caFile, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("ca_file %q: no valid certificates", caFile)
			}
			t.TLSClientConfig.RootCAs = pool
		}
		client.Transport = t
	}

	var rt http.RoundTripper = &userAgentRoundTripper{
		userAgent: httpclient.OpenTofuUserAgent(version.Version),
		inner:     client.Transport,
	}
	if span := tracing.SpanFromContext(ctx); span != nil && span.IsRecording() {
		rt = otelhttp.NewTransport(rt)
	}
	client.Transport = rt

	return client, nil
}

type userAgentRoundTripper struct {
	userAgent string
	inner     http.RoundTripper
}

func (rt *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", rt.userAgent)
	}
	return rt.inner.RoundTrip(req)
}

// Credentials

type realOCICredentialsPolicy struct {
	policy ociauthconfig.CredentialsConfigs
}

func (p realOCICredentialsPolicy) CredentialFunc(ctx context.Context, repository string) (credentialFunc, error) {
	repo, err := orasRemote.NewRepository(repository)
	if err != nil {
		return nil, err
	}
	registryDomain := repo.Reference.Registry
	repositoryPath := repo.Reference.Repository

	return func(ctx context.Context, _ string) (orasAuth.Credential, error) {
		source, err := p.policy.CredentialsSourceForRepository(ctx, registryDomain, repositoryPath)
		if err != nil {
			return orasAuth.EmptyCredential, err
		}
		creds, err := source.Credentials(ctx, dockerCredentialHelperEnv{})
		if err != nil {
			if ociauthconfig.IsCredentialsNotFoundError(err) {
				return orasAuth.EmptyCredential, nil
			}
			return orasAuth.EmptyCredential, err
		}
		return creds.ToORASCredential(), nil
	}, nil
}

type dockerCredentialHelperEnv struct{}

var _ ociauthconfig.CredentialsLookupEnvironment = dockerCredentialHelperEnv{}

func (dockerCredentialHelperEnv) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	exe := "docker-credential-" + helperName

	cmd := exec.CommandContext(ctx, exe, "get")
	cmd.Stdin = strings.NewReader(serverURL)
	stdout, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return ociauthconfig.DockerCredentialHelperGetResult{}, ociauthconfig.NewCredentialsNotFoundError(err)
		}
		return ociauthconfig.DockerCredentialHelperGetResult{}, err
	}

	var result ociauthconfig.DockerCredentialHelperGetResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return ociauthconfig.DockerCredentialHelperGetResult{}, fmt.Errorf("parsing credential helper response: %w", err)
	}
	if result.ServerURL == "" {
		result.ServerURL = serverURL
	}
	return result, nil
}
