package oras

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
	"sync"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/time/rate"
	orasRegistry "oras.land/oras-go/v2/registry"
	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"
)

const envVarRepository = "TF_BACKEND_ORAS_REPOSITORY"

const (
	envVarRetryMax     = "TF_BACKEND_ORAS_RETRY_MAX"
	envVarRetryWaitMin = "TF_BACKEND_ORAS_RETRY_WAIT_MIN"
	envVarRetryWaitMax = "TF_BACKEND_ORAS_RETRY_WAIT_MAX"
	envVarLockTTL      = "TF_BACKEND_ORAS_LOCK_TTL"
	envVarRateLimit    = "TF_BACKEND_ORAS_RATE_LIMIT"
	envVarRateBurst    = "TF_BACKEND_ORAS_RATE_LIMIT_BURST"
)

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	repository  string
	insecure    bool
	caFile      string
	compression string
	lockTTL     time.Duration
	rateLimit   int
	rateBurst   int
	retryCfg    RetryConfig

	versioningEnabled     bool
	versioningMaxVersions int

	orasCredsPolicy cliconfigORASCredentialsPolicy
	repoClient      *orasRepositoryClient
}

func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"repository": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "OCI repository in the form <registry>/<repository>, without tag or digest. Can also be set via TF_BACKEND_ORAS_REPOSITORY env var.",
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
			"retry_max": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarRetryMax, 2),
				Description: "The number of retries for transient registry requests.",
			},
			"retry_wait_min": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarRetryWaitMin, 1),
				Description: "The minimum time in seconds to wait between transient registry request attempts.",
			},
			"retry_wait_max": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarRetryWaitMax, 30),
				Description: "The maximum time in seconds to wait between transient registry request attempts.",
			},
			"compression": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "none",
				Description: "State compression. Supported values: none, gzip.",
			},
			"lock_ttl": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarLockTTL, 0),
				Description: "Lock TTL in seconds. If set, stale locks older than this will be automatically cleared. 0 disables.",
			},
			"rate_limit": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarRateLimit, 0),
				Description: "Maximum registry requests per second. 0 disables rate limiting.",
			},
			"rate_limit_burst": {
				Type:        schema.TypeInt,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc(envVarRateBurst, 0),
				Description: "Maximum burst size for registry requests when rate limiting is enabled. 0 defaults to 1.",
			},
			"versioning": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     false,
							Description: "Whether to keep historical state versions using versioned tags.",
						},
						"max_versions": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     10,
							Description: "Maximum number of historical state versions to retain.",
						},
					},
				},
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

	b.compression = strings.ToLower(strings.TrimSpace(data.Get("compression").(string)))
	if b.compression == "" {
		b.compression = "none"
	}
	switch b.compression {
	case "none", "gzip":
		// ok
	default:
		return fmt.Errorf("unsupported compression %q (supported: none, gzip)", b.compression)
	}

	lockTTLSeconds := data.Get("lock_ttl").(int)
	if lockTTLSeconds < 0 {
		return fmt.Errorf("lock_ttl must be non-negative")
	}
	b.lockTTL = time.Duration(lockTTLSeconds) * time.Second

	rateLimit := data.Get("rate_limit").(int)
	rateBurst := data.Get("rate_limit_burst").(int)
	if rateLimit < 0 {
		return fmt.Errorf("rate_limit must be non-negative")
	}
	if rateBurst < 0 {
		return fmt.Errorf("rate_limit_burst must be non-negative")
	}
	b.rateLimit = rateLimit
	b.rateBurst = rateBurst

	// Retry behavior (match HTTP backend semantics: retry_max is number of retries).
	retryMax := data.Get("retry_max").(int)
	retryWaitMin := time.Duration(data.Get("retry_wait_min").(int)) * time.Second
	retryWaitMax := time.Duration(data.Get("retry_wait_max").(int)) * time.Second

	retryCfg := DefaultRetryConfig()
	retryCfg.MaxAttempts = retryMax + 1
	retryCfg.InitialBackoff = retryWaitMin
	retryCfg.MaxBackoff = retryWaitMax
	// Keep BackoffMultiplier from DefaultRetryConfig.
	if retryCfg.MaxAttempts < 1 {
		retryCfg.MaxAttempts = 1
	}
	if retryCfg.InitialBackoff <= 0 {
		retryCfg.InitialBackoff = time.Second
	}
	if retryCfg.MaxBackoff > 0 && retryCfg.MaxBackoff < retryCfg.InitialBackoff {
		retryCfg.MaxBackoff = retryCfg.InitialBackoff
	}
	b.retryCfg = retryCfg

	// State versioning (optional)
	if v, ok := data.GetOk("versioning"); ok {
		if spec, ok := v.([]interface{})[0].(map[string]interface{}); ok {
			b.versioningEnabled = spec["enabled"].(bool)
			b.versioningMaxVersions = spec["max_versions"].(int)
		} else {
			return fmt.Errorf("failed to parse versioning")
		}
	}
	if b.versioningEnabled && b.versioningMaxVersions < 0 {
		b.versioningMaxVersions = 0
	}

	cfg, diags := cliconfig.LoadConfig(ctx)
	if diags.HasErrors() {
		return diags.Err()
	}
	policy, err := cfg.OCICredentialsPolicy(ctx)
	if err != nil {
		return err
	}
	b.orasCredsPolicy = realORASCredentialsPolicy{policy: policy}

	b.repoClient, err = newORASRepositoryClient(ctx, b.repository, b.insecure, b.caFile, b.orasCredsPolicy, b.rateLimit, b.rateBurst)
	return err
}

func (b *Backend) getRepository() (*orasRepositoryClient, error) {
	if b.repoClient == nil {
		return nil, fmt.Errorf("backend is not configured")
	}
	return b.repoClient, nil
}

func (b *Backend) StateMgr(ctx context.Context, workspace string) (statemgr.Full, error) {
	repo, err := b.getRepository()
	if err != nil {
		return nil, err
	}
	client := newRemoteClient(repo, workspace)
	client.retryConfig = b.retryCfg
	client.versioningEnabled = b.versioningEnabled
	client.versioningMaxVersions = b.versioningMaxVersions
	client.stateCompression = b.compression
	client.lockTTL = b.lockTTL
	return remote.NewState(client, b.encryption), nil
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	repo, err := b.getRepository()
	if err != nil {
		return nil, err
	}
	wss, err := listWorkspacesFromTags(ctx, repo)
	if err != nil {
		if isNotFound(err) {
			return []string{backend.DefaultStateName}, nil
		}
		return nil, err
	}
	if len(wss) == 0 {
		return []string{backend.DefaultStateName}, nil
	}
	return wss, nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	repo, err := b.getRepository()
	if err != nil {
		return err
	}

	wsTag := workspaceTagFor(name)
	stateRef := stateTagPrefix + wsTag
	lockRef := lockTagPrefix + wsTag
	stateVersionPrefix := stateRef + stateVersionTagSeparator

	if desc, err := repo.inner.Resolve(ctx, stateRef); err == nil {
		_ = repo.inner.Delete(ctx, desc)
	}
	_ = repo.inner.Tags(ctx, "", func(page []string) error {
		for _, tag := range page {
			if !strings.HasPrefix(tag, stateVersionPrefix) {
				continue
			}
			if desc, err := repo.inner.Resolve(ctx, tag); err == nil {
				_ = repo.inner.Delete(ctx, desc)
			}
		}
		return nil
	})
	if desc, err := repo.inner.Resolve(ctx, lockRef); err == nil {
		_ = repo.inner.Delete(ctx, desc)
	}
	return nil
}

// ORAS repository client

type cliconfigORASCredentialsPolicy interface {
	CredentialFunc(ctx context.Context, repository string) (credentialFunc, error)
}

type credentialFunc func(ctx context.Context, hostport string) (orasAuth.Credential, error)

type orasRepositoryClient struct {
	repository string
	inner      orasRepository
	authFn     credentialFunc
}

func (r *orasRepositoryClient) accessTokenForHost(ctx context.Context, host string) (string, error) {
	if r == nil || r.authFn == nil {
		return "", nil
	}
	cred, err := r.authFn(ctx, host)
	if err != nil {
		return "", err
	}
	if cred.AccessToken != "" {
		return cred.AccessToken, nil
	}
	if cred.Password != "" {
		return cred.Password, nil
	}
	return "", nil
}

type orasRepository interface {
	Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)
	Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error
	Delete(ctx context.Context, target ocispec.Descriptor) error
	Exists(ctx context.Context, target ocispec.Descriptor) (bool, error)
	Tags(ctx context.Context, last string, fn func(tags []string) error) error
}

func newORASRepositoryClient(ctx context.Context, repository string, insecure bool, caFile string, policy cliconfigORASCredentialsPolicy, rateLimit int, rateBurst int) (*orasRepositoryClient, error) {
	repo, err := orasRemote.NewRepository(repository)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI repository %q: %w", repository, err)
	}

	httpClient, err := newORASHTTPClient(ctx, insecure, caFile, rateLimit, rateBurst)
	if err != nil {
		return nil, err
	}

	credFunc, err := policy.CredentialFunc(ctx, repository)
	if err != nil {
		return nil, err
	}

	repoClient := &orasRepositoryClient{
		repository: repository,
		inner:      repo,
		authFn:     credFunc,
	}

	repo.Client = &orasAuth.Client{
		Client: httpClient,
		Credential: func(ctx context.Context, host string) (orasAuth.Credential, error) {
			return credFunc(ctx, host)
		},
	}

	return repoClient, nil
}

func newORASHTTPClient(ctx context.Context, insecure bool, caFile string, rateLimit int, rateBurst int) (*http.Client, error) {
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

	var limiter requestLimiter
	if rateLimit > 0 {
		if rateBurst <= 0 {
			rateBurst = 1
		}
		limiter = rate.NewLimiter(rate.Limit(rateLimit), rateBurst)
	}

	var rt http.RoundTripper = &userAgentRoundTripper{
		userAgent: httpclient.OpenTofuUserAgent(version.Version),
		inner:     client.Transport,
	}
	if limiter != nil {
		rt = &rateLimitedRoundTripper{limiter: limiter, inner: rt}
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

type requestLimiter interface {
	Wait(ctx context.Context) error
}

type rateLimitedRoundTripper struct {
	limiter requestLimiter
	inner   http.RoundTripper
}

func (rt *rateLimitedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.limiter != nil {
		if err := rt.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
	}
	return rt.inner.RoundTrip(req)
}

// Credentials

const defaultDockerCredentialHelperCacheTTL = 5 * time.Minute

type dockerCredentialHelperCacheKey struct {
	helperName string
	serverURL  string
}

type dockerCredentialHelperCacheEntry struct {
	result   ociauthconfig.DockerCredentialHelperGetResult
	err      error
	expires  time.Time
	hasValue bool
}

type cachedDockerCredentialHelperEnv struct {
	inner ociauthconfig.CredentialsLookupEnvironment
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[dockerCredentialHelperCacheKey]dockerCredentialHelperCacheEntry
}

var _ ociauthconfig.CredentialsLookupEnvironment = (*cachedDockerCredentialHelperEnv)(nil)

func newCachedDockerCredentialHelperEnv(inner ociauthconfig.CredentialsLookupEnvironment, ttl time.Duration) *cachedDockerCredentialHelperEnv {
	return &cachedDockerCredentialHelperEnv{
		inner: inner,
		ttl:   ttl,
		now:   time.Now,
		cache: make(map[dockerCredentialHelperCacheKey]dockerCredentialHelperCacheEntry),
	}
}

func (e *cachedDockerCredentialHelperEnv) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	if err := ctx.Err(); err != nil {
		return ociauthconfig.DockerCredentialHelperGetResult{}, err
	}
	if e.inner == nil {
		return ociauthconfig.DockerCredentialHelperGetResult{}, fmt.Errorf("no credential helper lookup environment")
	}
	if e.ttl <= 0 {
		return e.inner.QueryDockerCredentialHelper(ctx, helperName, serverURL)
	}

	key := dockerCredentialHelperCacheKey{helperName: helperName, serverURL: serverURL}
	now := e.now()

	e.mu.Lock()
	if entry, ok := e.cache[key]; ok && entry.hasValue && now.Before(entry.expires) {
		e.mu.Unlock()
		return entry.result, entry.err
	}
	e.mu.Unlock()

	result, err := e.inner.QueryDockerCredentialHelper(ctx, helperName, serverURL)

	e.mu.Lock()
	e.cache[key] = dockerCredentialHelperCacheEntry{
		result:   result,
		err:      err,
		expires:  now.Add(e.ttl),
		hasValue: true,
	}
	e.mu.Unlock()

	return result, err
}

type realORASCredentialsPolicy struct {
	policy ociauthconfig.CredentialsConfigs
}

func (p realORASCredentialsPolicy) CredentialFunc(ctx context.Context, repository string) (credentialFunc, error) {
	repo, err := orasRemote.NewRepository(repository)
	if err != nil {
		return nil, err
	}
	registryDomain := repo.Reference.Registry
	repositoryPath := repo.Reference.Repository

	lookupEnv := newCachedDockerCredentialHelperEnv(dockerCredentialHelperEnv{}, defaultDockerCredentialHelperCacheTTL)

	return func(ctx context.Context, _ string) (orasAuth.Credential, error) {
		source, err := p.policy.CredentialsSourceForRepository(ctx, registryDomain, repositoryPath)
		if err != nil {
			return orasAuth.EmptyCredential, err
		}
		creds, err := source.Credentials(ctx, lookupEnv)
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
