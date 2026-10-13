// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"bufio"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

// Authentication is resolved independently of provider configuration. The modes
// were checked against oracle/terraform-provider-oci at
// ad4dcb798cc2a64eb77285b894f7d6edd73e176b; this implementation uses SDK APIs and
// does not include provider implementation code.
var supportedAuthModes = []string{
	"ApiKey", "InstancePrincipal", "InstancePrincipalWithCerts", "SecurityToken",
	"ResourcePrincipal", "OKEWorkloadIdentity", "WorkloadIdentityFederation",
}

type authConfig struct {
	Mode                            string
	TenancyOCID                     string
	UserOCID                        string
	Fingerprint                     string
	PrivateKey                      string
	PrivateKeyPath                  string
	PrivateKeyPassword              string
	Region                          string
	ConfigFile                      string
	ConfigFileProfile               string
	CertificatesDirectory           string
	WorkloadIdentityTokenPath       string
	TokenExchangeDomainURL          string
	TokenExchangeAuth               string
	TokenExchangeClientID           string
	TokenExchangeClientSecret       string
	TokenExchangeRequestedTokenType string
	TokenExchangeSubjectTokenType   string
	TokenExchangeResourceType       string
	TokenExchangeRPSTExp            string
	TokenExchangePublicKey          string
}

func authConfigWithDefaults(c authConfig) authConfig {
	for _, item := range []struct {
		name  string
		value *string
	}{
		{"auth", &c.Mode}, {"tenancy_ocid", &c.TenancyOCID}, {"user_ocid", &c.UserOCID},
		{"fingerprint", &c.Fingerprint}, {"private_key", &c.PrivateKey},
		{"private_key_path", &c.PrivateKeyPath}, {"private_key_password", &c.PrivateKeyPassword},
		{"region", &c.Region}, {"config_file_profile", &c.ConfigFileProfile},
		{"certificates_directory", &c.CertificatesDirectory},
		{"workload_identity_token_path", &c.WorkloadIdentityTokenPath},
		{"token_exchange_domain_url", &c.TokenExchangeDomainURL},
		{"token_exchange_auth", &c.TokenExchangeAuth},
		{"token_exchange_client_id", &c.TokenExchangeClientID},
		{"token_exchange_client_secret", &c.TokenExchangeClientSecret},
		{"token_exchange_requested_token_type", &c.TokenExchangeRequestedTokenType},
		{"token_exchange_subject_token_type", &c.TokenExchangeSubjectTokenType},
		{"token_exchange_resource_type", &c.TokenExchangeResourceType},
		{"token_exchange_rpst_exp", &c.TokenExchangeRPSTExp},
		{"token_exchange_public_key", &c.TokenExchangePublicKey},
	} {
		if *item.value == "" {
			*item.value = authEnvironmentValue(item.name)
		}
	}
	if c.Mode == "" {
		c.Mode = "ApiKey"
	}
	if strings.EqualFold(c.Mode, "ApiKey") {
		if tenancy := legacyAuthEnvironmentValue("boat_tenancy_ocid"); tenancy != "" {
			c.TenancyOCID = tenancy
		}
	}
	if c.ConfigFileProfile == "" {
		c.ConfigFileProfile = "DEFAULT"
	}
	if c.TokenExchangeAuth == "" {
		c.TokenExchangeAuth = "OAuthClientCredentials"
	}
	if c.TokenExchangeSubjectTokenType == "" {
		c.TokenExchangeSubjectTokenType = "jwt"
	}
	if c.ConfigFile == "" {
		// OCI_CONFIG_FILE has SDK fallback semantics, rather than overriding an
		// existing ~/.oci/config. Explicit config_file is a backend extension.
		c.ConfigFile = os.Getenv("TF_VAR_config_file")
	}
	if c.CertificatesDirectory == "" {
		c.CertificatesDirectory = legacyAuthEnvironmentValue("test_certificates_location")
		if c.CertificatesDirectory == "" {
			c.CertificatesDirectory = os.Getenv("OCI_TEST_CERTIFICATES_LOCATION")
		}
	}
	return c
}

func authEnvironmentValue(name string) string {
	if value := os.Getenv("TF_VAR_" + name); value != "" {
		return value
	}
	return os.Getenv("OCI_" + strings.ToUpper(name))
}

// A few provider implementation switches predate the public uppercase OCI
// schema variables. Preserve their lowercase and unprefixed spellings.
func legacyAuthEnvironmentValue(name string) string {
	for _, key := range []string{"TF_VAR_" + name, "OCI_" + name, name} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

type certificateCredentials struct {
	leaf, privateKey, passphrase []byte
	intermediate                 [][]byte
}

// Factories allow deterministic tests for host-specific SDK authentication.
// They do not replace the SDK in production.
type authFactories struct {
	instancePrincipal    func(string) (common.ConfigurationProvider, error)
	instanceCertificates func(string, certificateCredentials) (common.ConfigurationProvider, error)
	resourcePrincipal    func(string) (common.ConfigurationProvider, error)
	okeWorkload          func() (common.ConfigurationProvider, error)
	tokenExchange        func(auth.TokenIssuer, auth.TokenExchangeBuilder) (common.ConfigurationProvider, error)
}

func sdkAuthFactories() authFactories {
	return authFactories{
		instancePrincipal: func(region string) (common.ConfigurationProvider, error) {
			return auth.InstancePrincipalConfigurationProviderForRegion(common.StringToRegion(region))
		},
		instanceCertificates: func(region string, c certificateCredentials) (common.ConfigurationProvider, error) {
			return auth.InstancePrincipalConfigurationWithCerts(common.StringToRegion(region), c.leaf, c.passphrase, c.privateKey, c.intermediate)
		},
		resourcePrincipal: func(region string) (common.ConfigurationProvider, error) {
			// The SDK's ForRegion factory omits version 3.0. Use its full
			// runtime factory for that version and apply the service region
			// override at our configuration-provider boundary.
			if region == "" || os.Getenv(auth.ResourcePrincipalVersionEnvVar) == auth.ResourcePrincipalVersion3_0 {
				return auth.ResourcePrincipalConfigurationProvider()
			}
			return auth.ResourcePrincipalConfigurationProviderForRegion(common.StringToRegion(region))
		},
		okeWorkload:   func() (common.ConfigurationProvider, error) { return auth.OkeWorkloadIdentityConfigurationProvider() },
		tokenExchange: auth.TokenExchangeConfigurationProviderFromIssuer,
	}
}

func buildConfigurationProvider(c authConfig) (common.ConfigurationProvider, error) {
	return buildConfigurationProviderWithFactories(c, sdkAuthFactories())
}

func buildConfigurationProviderWithFactories(c authConfig, f authFactories) (result common.ConfigurationProvider, resultErr error) {
	c = authConfigWithDefaults(c)
	mode := ""
	for _, candidate := range supportedAuthModes {
		if strings.EqualFold(c.Mode, candidate) {
			mode = candidate
			break
		}
	}
	if mode == "" {
		return nil, fmt.Errorf("auth must be one of: %s", strings.Join(supportedAuthModes, ", "))
	}
	c.Mode = mode
	// SDK constructors can panic on malformed runtime input (for example,
	// incomplete version 3.0 parent endpoint settings). Keep that failure at
	// the same redacted boundary as other credential initialization errors.
	defer func() {
		if recover() != nil {
			result = nil
			resultErr = authCredentialError(mode)
		}
	}()
	var provider common.ConfigurationProvider
	var err error
	switch mode {
	case "ApiKey":
		provider, err = apiKeyProvider(c)
		if err != nil {
			return nil, err
		}
	case "SecurityToken":
		provider, err = sessionTokenProvider(c)
		if err != nil {
			return nil, err
		}
	case "InstancePrincipal", "InstancePrincipalWithCerts":
		if c.Region == "" {
			return nil, errors.New("region is required for instance principal authentication; set region or OCI_REGION")
		}
		if mode == "InstancePrincipal" {
			provider, err = f.instancePrincipal(c.Region)
		} else {
			var credentials certificateCredentials
			credentials, err = readCertificateCredentials(c.CertificatesDirectory)
			if err != nil {
				return nil, err
			}
			provider, err = f.instanceCertificates(c.Region, credentials)
		}
	case "ResourcePrincipal":
		provider, err = f.resourcePrincipal(c.Region)
	case "OKEWorkloadIdentity":
		provider, err = f.okeWorkload()
	case "WorkloadIdentityFederation":
		if err = validateFederationConfig(c); err != nil {
			return nil, err
		}
		issuer := projectedTokenFile{filename: expandAuthPath(c.WorkloadIdentityTokenPath), subjectTokenType: c.TokenExchangeSubjectTokenType}
		if _, err = issuer.GetToken(); err != nil {
			return nil, err
		}
		builder := auth.TokenExchangeBuilder{
			DomainUrl: c.TokenExchangeDomainURL, Region: c.Region,
			RequestedTokenType: c.TokenExchangeRequestedTokenType, SubjectTokenType: c.TokenExchangeSubjectTokenType,
			ResType: c.TokenExchangeResourceType, RpstExp: c.TokenExchangeRPSTExp, PublicKey: c.TokenExchangePublicKey,
		}
		if strings.EqualFold(c.TokenExchangeAuth, "InstancePrincipal") {
			builder.InstancePrincipalProvider, err = f.instancePrincipal(c.Region)
			if err != nil {
				return nil, errors.New("WorkloadIdentityFederation could not initialize instance principal token exchange authentication; check the OCI instance identity and network access")
			}
		} else {
			builder.ClientId, builder.ClientSecret = c.TokenExchangeClientID, c.TokenExchangeClientSecret
		}
		provider, err = f.tokenExchange(issuer, builder)
	}
	if err != nil {
		return nil, authCredentialError(mode)
	}
	if provider == nil {
		return nil, fmt.Errorf("%s authentication returned no configuration provider", mode)
	}
	return &protectedConfigurationProvider{provider: provider, mode: mode, region: c.Region}, nil
}

func expandAuthPath(filename string) string {
	if filename == "~" || strings.HasPrefix(filename, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if filename == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(filename, "~/"))
		}
	}
	return filename
}

func configurationFilePaths(c authConfig) ([]string, error) {
	if c.ConfigFile != "" {
		return []string{expandAuthPath(c.ConfigFile)}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("cannot find home directory; set config_file explicitly")
	}
	primary := filepath.Join(home, ".oci", "config")
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		if fallback := os.Getenv("OCI_CONFIG_FILE"); fallback != "" {
			primary = expandAuthPath(fallback)
		}
	}
	if c.ConfigFileProfile != "DEFAULT" {
		return []string{primary}, nil
	}
	return []string{primary, filepath.Join(home, ".obmcs", "config")}, nil
}

func profileExists(filename, profile string) bool {
	file, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "["+profile+"]" {
			return true
		}
	}
	return false
}

func apiKeyProvider(c authConfig) (common.ConfigurationProvider, error) {
	paths, err := configurationFilePaths(c)
	if err != nil {
		return nil, err
	}
	var files []common.ConfigurationProvider
	for _, filename := range paths {
		if c.ConfigFileProfile != "DEFAULT" && !profileExists(filename, c.ConfigFileProfile) {
			return nil, errors.New("config_file_profile was not found; check the OCI config file and profile name")
		}
		if c.ConfigFile != "" {
			if _, err := os.Stat(filename); err != nil {
				return nil, errors.New("config_file is not readable; check its path and file permissions")
			}
		}
		provider, _ := common.ConfigurationProviderFromFileWithProfile(filename, c.ConfigFileProfile, c.PrivateKeyPassword)
		files = append(files, provider)
		if c.ConfigFileProfile != "DEFAULT" {
			fallback, _ := common.ConfigurationProviderFromFileWithProfile(filename, "DEFAULT", c.PrivateKeyPassword)
			files = append(files, fallback)
		}
	}
	resolve := func(value string, getter func(common.ConfigurationProvider) (string, error)) string {
		if value != "" {
			return value
		}
		for _, file := range files {
			if value, err := getter(file); err == nil && value != "" {
				return value
			}
		}
		return ""
	}
	c.TenancyOCID = resolve(c.TenancyOCID, common.ConfigurationProvider.TenancyOCID)
	c.UserOCID = resolve(c.UserOCID, common.ConfigurationProvider.UserOCID)
	c.Fingerprint = resolve(c.Fingerprint, common.ConfigurationProvider.KeyFingerprint)
	c.Region = resolve(c.Region, common.ConfigurationProvider.Region)
	var missing []string
	for _, field := range []struct{ name, value string }{
		{"tenancy_ocid", c.TenancyOCID}, {"user_ocid", c.UserOCID}, {"fingerprint", c.Fingerprint}, {"region", c.Region},
	} {
		if field.value == "" {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("ApiKey authentication requires %s; set backend attributes, OCI environment variables, or a valid OCI profile", strings.Join(missing, ", "))
	}
	var key *rsa.PrivateKey
	switch {
	case c.PrivateKey != "":
		key, err = common.PrivateKeyFromBytesWithPassword([]byte(strings.ReplaceAll(c.PrivateKey, `\n`, "\n")), []byte(c.PrivateKeyPassword))
	case c.PrivateKeyPath != "":
		var material []byte
		material, err = os.ReadFile(expandAuthPath(c.PrivateKeyPath))
		if err == nil {
			key, err = common.PrivateKeyFromBytesWithPassword(material, []byte(c.PrivateKeyPassword))
		}
	default:
		for _, file := range files {
			key, err = file.PrivateRSAKey()
			if err == nil {
				break
			}
		}
	}
	if key == nil || err != nil {
		return nil, errors.New("ApiKey private key could not be loaded; check private_key, private_key_path or profile key_file and its passphrase")
	}
	return &apiKeyConfiguration{key: key, tenancy: c.TenancyOCID, user: c.UserOCID, fingerprint: c.Fingerprint, region: c.Region}, nil
}

type apiKeyConfiguration struct {
	key                                *rsa.PrivateKey
	tenancy, user, fingerprint, region string
}

func (p *apiKeyConfiguration) PrivateRSAKey() (*rsa.PrivateKey, error) { return p.key, nil }
func (p *apiKeyConfiguration) KeyID() (string, error) {
	return p.tenancy + "/" + p.user + "/" + p.fingerprint, nil
}
func (p *apiKeyConfiguration) TenancyOCID() (string, error)    { return p.tenancy, nil }
func (p *apiKeyConfiguration) UserOCID() (string, error)       { return p.user, nil }
func (p *apiKeyConfiguration) KeyFingerprint() (string, error) { return p.fingerprint, nil }
func (p *apiKeyConfiguration) Region() (string, error)         { return p.region, nil }
func (p *apiKeyConfiguration) AuthType() (common.AuthConfig, error) {
	return common.AuthConfig{AuthType: common.UnknownAuthenticationType}, nil
}

func sessionTokenProvider(c authConfig) (common.ConfigurationProvider, error) {
	paths, err := configurationFilePaths(c)
	if err != nil {
		return nil, err
	}
	if !profileExists(paths[0], c.ConfigFileProfile) {
		return nil, errors.New("SecurityToken config_file_profile was not found; create or renew the OCI CLI session and check its profile")
	}
	provider, err := common.ConfigurationProviderForSessionTokenWithProfile(paths[0], c.ConfigFileProfile, c.PrivateKeyPassword)
	if err != nil {
		return nil, errors.New("SecurityToken profile could not be loaded; check the OCI config file and profile")
	}
	keyID, err := provider.KeyID()
	if err != nil {
		return nil, errors.New("SecurityToken could not be read; check profile security_token_file, tenancy and fingerprint")
	}
	if err := validateExpiringToken(strings.TrimSpace(strings.TrimPrefix(keyID, "ST$")), "SecurityToken"); err != nil {
		return nil, err
	}
	withFallback := &sessionKeyFallbackProvider{ConfigurationProvider: provider, config: c}
	if c.ConfigFileProfile != "DEFAULT" {
		withFallback.defaultProfile, _ = common.ConfigurationProviderFromFileWithProfile(paths[0], "DEFAULT", c.PrivateKeyPassword)
	}
	if _, err := withFallback.PrivateRSAKey(); err != nil {
		return nil, errors.New("SecurityToken private key could not be loaded; check selected profile key_file, private_key or private_key_path, DEFAULT profile key_file, and the passphrase")
	}
	return withFallback, nil
}

// OCI's SecurityToken mode selects the session profile key first. A raw key
// is only a fallback, followed by the SDK's DEFAULT profile inheritance. The
// selected session token and its tenancy/fingerprint remain authoritative.
type sessionKeyFallbackProvider struct {
	common.ConfigurationProvider
	config         authConfig
	defaultProfile common.ConfigurationProvider
}

func (p *sessionKeyFallbackProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	if key, err := p.ConfigurationProvider.PrivateRSAKey(); err == nil {
		return key, nil
	}
	if p.config.PrivateKey != "" {
		if key, err := common.PrivateKeyFromBytesWithPassword([]byte(strings.ReplaceAll(p.config.PrivateKey, `\n`, "\n")), []byte(p.config.PrivateKeyPassword)); err == nil {
			return key, nil
		}
	} else if p.config.PrivateKeyPath != "" {
		if material, err := os.ReadFile(expandAuthPath(p.config.PrivateKeyPath)); err == nil {
			if key, err := common.PrivateKeyFromBytesWithPassword(material, []byte(p.config.PrivateKeyPassword)); err == nil {
				return key, nil
			}
		}
	}
	if p.defaultProfile != nil {
		if key, err := p.defaultProfile.PrivateRSAKey(); err == nil {
			return key, nil
		}
	}
	return nil, authCredentialError("SecurityToken")
}

func validateFederationConfig(c authConfig) error {
	var missing []string
	for _, item := range []struct{ name, value string }{
		{"region", c.Region}, {"workload_identity_token_path", c.WorkloadIdentityTokenPath},
		{"token_exchange_domain_url", c.TokenExchangeDomainURL},
		{"token_exchange_requested_token_type", c.TokenExchangeRequestedTokenType},
	} {
		if strings.TrimSpace(item.value) == "" {
			missing = append(missing, item.name)
		}
	}
	switch {
	case strings.EqualFold(c.TokenExchangeAuth, "OAuthClientCredentials"):
		if c.TokenExchangeClientID == "" {
			missing = append(missing, "token_exchange_client_id")
		}
		if c.TokenExchangeClientSecret == "" {
			missing = append(missing, "token_exchange_client_secret")
		}
	case strings.EqualFold(c.TokenExchangeAuth, "InstancePrincipal"):
		if c.TokenExchangeClientID != "" || c.TokenExchangeClientSecret != "" {
			return errors.New("token_exchange_client_id and token_exchange_client_secret must be unset for InstancePrincipal token exchange authentication")
		}
	default:
		return errors.New("token_exchange_auth must be OAuthClientCredentials or InstancePrincipal")
	}
	if len(missing) > 0 {
		return fmt.Errorf("WorkloadIdentityFederation requires %s", strings.Join(missing, ", "))
	}
	domain, err := url.Parse(c.TokenExchangeDomainURL)
	if err != nil || domain.Scheme != "https" || domain.Hostname() == "" || domain.User != nil || domain.RawQuery != "" || domain.Fragment != "" {
		return errors.New("token_exchange_domain_url must be an HTTPS identity domain URL without credentials, a query, or a fragment")
	}
	if c.TokenExchangeRequestedTokenType == "urn:oci:token-type:oci-rpst" && c.TokenExchangeResourceType == "" {
		return errors.New("token_exchange_resource_type is required when requesting an OCI RPST")
	}
	return nil
}

// Token files are reread, so Kubernetes projected-token rotation is respected.
type projectedTokenFile struct{ filename, subjectTokenType string }

func (f projectedTokenFile) GetToken() (string, error) {
	material, err := os.ReadFile(f.filename)
	if err != nil {
		return "", errors.New("workload identity token file is not readable; check workload_identity_token_path and file permissions")
	}
	token := strings.TrimSpace(string(material))
	if token == "" {
		return "", errors.New("workload identity token file is empty")
	}
	if f.subjectTokenType == "" || strings.EqualFold(f.subjectTokenType, "jwt") || f.subjectTokenType == "urn:ietf:params:oauth:token-type:jwt" {
		if err := validateExpiringToken(token, "workload identity"); err != nil {
			return "", err
		}
	}
	return token, nil
}

// Expiration inspection is input validation, not signature verification. OCI
// validates the exchanged or signed token. No token material enters errors.
func validateExpiringToken(token, description string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%s token is not a JWT; renew the credentials or check the token file", description)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return fmt.Errorf("%s token has an invalid JWT payload", description)
	}
	var claims struct {
		Expiration json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("%s token has invalid expiration metadata", description)
	}
	expires, err := claims.Expiration.Int64()
	if err != nil {
		return fmt.Errorf("%s token is missing a valid expiration; renew the credentials", description)
	}
	if expires <= time.Now().Unix() {
		return fmt.Errorf("%s token has expired; renew the credentials before running tofu init", description)
	}
	return nil
}

func readCertificateCredentials(directory string) (certificateCredentials, error) {
	if directory == "" {
		directory = "."
	}
	directory = expandAuthPath(directory)
	c := certificateCredentials{}
	var err error
	for _, item := range []struct {
		name   string
		target *[]byte
	}{
		{"ip_cert.pem", &c.leaf}, {"ip_key.pem", &c.privateKey},
	} {
		*item.target, err = os.ReadFile(filepath.Join(directory, item.name))
		if err != nil {
			return c, fmt.Errorf("InstancePrincipalWithCerts cannot read %s; check certificates_directory and file permissions", item.name)
		}
	}
	intermediate, err := os.ReadFile(filepath.Join(directory, "intermediate.pem"))
	if err != nil {
		return c, errors.New("InstancePrincipalWithCerts cannot read intermediate.pem; check certificates_directory and file permissions")
	}
	c.intermediate = [][]byte{intermediate}
	c.passphrase, err = os.ReadFile(filepath.Join(directory, "leaf_passphrase"))
	if err != nil && !os.IsNotExist(err) {
		return c, errors.New("InstancePrincipalWithCerts cannot read optional leaf_passphrase")
	}
	parse := func(material []byte) (*x509.Certificate, error) {
		block, _ := pem.Decode(material)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, errors.New("certificate is not PEM encoded")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("certificate is invalid")
		}
		now := time.Now()
		if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return nil, errors.New("certificate is expired or not yet valid")
		}
		return certificate, nil
	}
	leaf, err := parse(c.leaf)
	if err != nil {
		return c, fmt.Errorf("InstancePrincipalWithCerts ip_cert.pem: %s", err)
	}
	if _, err = parse(intermediate); err != nil {
		return c, fmt.Errorf("InstancePrincipalWithCerts intermediate.pem: %s", err)
	}
	hasTenancy := false
	for _, name := range leaf.Subject.Names {
		if value, ok := name.Value.(string); ok && strings.HasPrefix(value, "opc-tenant:") && len(value) > len("opc-tenant:") {
			hasTenancy = true
		}
	}
	if !hasTenancy {
		return c, errors.New("InstancePrincipalWithCerts ip_cert.pem must identify its OCI tenancy")
	}
	key, err := common.PrivateKeyFromBytesWithPassword(c.privateKey, c.passphrase)
	if err != nil {
		return c, errors.New("InstancePrincipalWithCerts ip_key.pem cannot be decoded; check the key and leaf_passphrase")
	}
	public, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok || public.N.Cmp(key.N) != 0 || public.E != key.E {
		return c, errors.New("InstancePrincipalWithCerts ip_key.pem does not match ip_cert.pem")
	}
	return c, nil
}

// This boundary prevents SDK errors (some of which embed token material) from
// reaching backend diagnostics. It also preserves refreshability and protects
// SDK providers whose internal lazy caches are not safe for simultaneous reads.
type protectedConfigurationProvider struct {
	provider     common.ConfigurationProvider
	mode, region string
	mu           sync.Mutex
}

func authCredentialError(mode string) error {
	help := "check credentials, identity permissions, and OCI authentication endpoint access"
	switch mode {
	case "InstancePrincipal":
		help = "check instance identity, metadata service access, dynamic group IAM policies, and federation endpoint access"
	case "InstancePrincipalWithCerts":
		help = "check instance certificate validity, its OCI tenancy, private key, dynamic group IAM policies, and federation endpoint access"
	case "ResourcePrincipal":
		help = "check OCI_RESOURCE_PRINCIPAL_VERSION and the runtime's token, private key, region, file permissions, and IAM policies"
	case "OKEWorkloadIdentity":
		help = "check OCI_RESOURCE_PRINCIPAL_VERSION, OCI_RESOURCE_PRINCIPAL_REGION, KUBERNETES_SERVICE_HOST, the service account token and CA, and workload IAM policies"
	case "SecurityToken":
		help = "renew the OCI CLI session and check the profile's security_token_file, key_file, passphrase, and file permissions"
	case "WorkloadIdentityFederation":
		help = "check the projected token's validity, token exchange credentials and IAM policies, and identity domain endpoint access"
	}
	return fmt.Errorf("%s authentication failed; %s", mode, help)
}
func (p *protectedConfigurationProvider) safeError() error { return authCredentialError(p.mode) }
func (p *protectedConfigurationProvider) stringValue(get func() (string, error)) (value string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() {
		if recover() != nil {
			value = ""
			err = p.safeError()
		}
	}()
	value, err = get()
	if err != nil {
		return "", p.safeError()
	}
	return value, nil
}
func (p *protectedConfigurationProvider) TenancyOCID() (string, error) {
	return p.stringValue(p.provider.TenancyOCID)
}
func (p *protectedConfigurationProvider) UserOCID() (string, error) {
	return p.stringValue(p.provider.UserOCID)
}
func (p *protectedConfigurationProvider) KeyFingerprint() (string, error) {
	return p.stringValue(p.provider.KeyFingerprint)
}
func (p *protectedConfigurationProvider) Region() (string, error) {
	if p.region != "" {
		return p.region, nil
	}
	return p.stringValue(p.provider.Region)
}
func (p *protectedConfigurationProvider) KeyID() (string, error) {
	return p.stringValue(func() (string, error) {
		value, err := p.provider.KeyID()
		if err == nil && (p.mode == "SecurityToken" || strings.HasPrefix(value, "ST$")) {
			token := strings.TrimSpace(strings.TrimPrefix(value, "ST$"))
			if err = validateExpiringToken(token, p.mode); err == nil {
				value = "ST$" + token
			}
		}
		return value, err
	})
}
func (p *protectedConfigurationProvider) PrivateRSAKey() (key *rsa.PrivateKey, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() {
		if recover() != nil {
			key = nil
			err = p.safeError()
		}
	}()
	key, err = p.provider.PrivateRSAKey()
	if err != nil {
		return nil, p.safeError()
	}
	return key, nil
}
func (p *protectedConfigurationProvider) AuthType() (common.AuthConfig, error) {
	// The selected backend mode is authoritative; a profile's
	// authentication_type must not switch API-key auth to instance auth.
	return common.AuthConfig{AuthType: common.UnknownAuthenticationType, IsFromConfigFile: false}, nil
}
func (p *protectedConfigurationProvider) Refreshable() bool { return p.mode != "ApiKey" }
