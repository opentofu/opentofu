// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

func authTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{
		"auth", "tenancy_ocid", "user_ocid", "fingerprint", "private_key", "private_key_path",
		"private_key_password", "region", "config_file_profile", "certificates_directory",
		"test_certificates_location", "workload_identity_token_path", "token_exchange_domain_url",
		"token_exchange_auth", "token_exchange_client_id", "token_exchange_client_secret",
		"token_exchange_requested_token_type", "token_exchange_subject_token_type",
		"token_exchange_resource_type", "token_exchange_rpst_exp", "token_exchange_public_key",
	} {
		t.Setenv("TF_VAR_"+name, "")
		t.Setenv("OCI_"+strings.ToUpper(name), "")
	}
	t.Setenv("OCI_CONFIG_FILE", "")
	t.Setenv("TF_VAR_config_file", "")
	for _, name := range []string{"boat_tenancy_ocid", "test_certificates_location"} {
		t.Setenv("TF_VAR_"+name, "")
		t.Setenv("OCI_"+name, "")
		t.Setenv(name, "")
	}
}

func authTestPrivateKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func authTestToken(t *testing.T, expires time.Time, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{"exp": expires.Unix(), "sub": "test-subject", "tenant": "test-tenancy", "res_tenant": "test-tenancy"}
	for key, value := range extra {
		claims[key] = value
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func authTestWrite(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAuthEnvironmentPrecedenceAndDefaults(t *testing.T) {
	authTestEnvironment(t)
	t.Setenv("TF_VAR_auth", "SecurityToken")
	t.Setenv("OCI_AUTH", "ResourcePrincipal")
	t.Setenv("TF_VAR_region", "us-phoenix-1")
	t.Setenv("OCI_REGION", "us-ashburn-1")
	c := authConfigWithDefaults(authConfig{Mode: "ApiKey"})
	if c.Mode != "ApiKey" || c.Region != "us-phoenix-1" {
		t.Fatalf("explicit/TF_VAR precedence incorrect: mode=%s region=%s", c.Mode, c.Region)
	}
	c = authConfigWithDefaults(authConfig{})
	if c.Mode != "SecurityToken" || c.ConfigFileProfile != "DEFAULT" || c.TokenExchangeAuth != "OAuthClientCredentials" || c.TokenExchangeSubjectTokenType != "jwt" {
		t.Fatalf("defaults incorrect")
	}
	t.Setenv("TF_VAR_region", "")
	if c := authConfigWithDefaults(authConfig{}); c.Region != "us-ashburn-1" {
		t.Fatal("OCI region fallback was not used")
	}
}

func TestAuthLegacyCredentialEnvironmentParity(t *testing.T) {
	authTestEnvironment(t)
	t.Setenv("boat_tenancy_ocid", "raw-tenancy")
	t.Setenv("OCI_boat_tenancy_ocid", "oci-tenancy")
	t.Setenv("TF_VAR_boat_tenancy_ocid", "tf-tenancy")
	c := authConfigWithDefaults(authConfig{TenancyOCID: "explicit-tenancy"})
	if c.TenancyOCID != "tf-tenancy" {
		t.Fatal("provider BOAT tenancy override precedence lost")
	}
	t.Setenv("OCI_test_certificates_location", "oci-certs")
	t.Setenv("test_certificates_location", "raw-certs")
	if got := authConfigWithDefaults(authConfig{Mode: "InstancePrincipalWithCerts"}).CertificatesDirectory; got != "oci-certs" {
		t.Fatalf("legacy cert ENV got %s", got)
	}
	if got := authConfigWithDefaults(authConfig{CertificatesDirectory: "explicit-certs"}).CertificatesDirectory; got != "explicit-certs" {
		t.Fatal("legacy cert ENV overrode backend input")
	}
}

func TestAuthAPIKeyConfigurationPrecedence(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	keyPath := filepath.Join(t.TempDir(), "key.pem")
	authTestWrite(t, keyPath, key)
	config := filepath.Join(t.TempDir(), "config")
	authTestWrite(t, config, fmt.Sprintf("[DEFAULT]\ntenancy=default-tenancy\nuser=default-user\nfingerprint=default-fingerprint\nregion=us-ashburn-1\nkey_file=%s\n[NAMED]\nuser=named-user\n", keyPath))
	t.Setenv("OCI_TENANCY_OCID", "env-tenancy")
	t.Setenv("TF_VAR_fingerprint", "tf-fingerprint")
	t.Setenv("OCI_FINGERPRINT", "oci-fingerprint")
	provider, err := buildConfigurationProvider(authConfig{ConfigFile: config, ConfigFileProfile: "NAMED", TenancyOCID: "explicit-tenancy"})
	if err != nil {
		t.Fatal(err)
	}
	for name, check := range map[string]struct {
		get  func() (string, error)
		want string
	}{
		"tenancy": {provider.TenancyOCID, "explicit-tenancy"}, "user": {provider.UserOCID, "named-user"},
		"fingerprint": {provider.KeyFingerprint, "tf-fingerprint"}, "region": {provider.Region, "us-ashburn-1"},
	} {
		got, err := check.get()
		if err != nil || got != check.want {
			t.Fatalf("%s: got %q error %v", name, got, err)
		}
	}
	if _, err := provider.PrivateRSAKey(); err != nil {
		t.Fatal(err)
	}
	if provider.(common.RefreshableConfigurationProvider).Refreshable() {
		t.Fatal("API key incorrectly marked refreshable")
	}
	if _, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider); err != nil {
		t.Fatalf("SDK client could not initialize: %v", err)
	}
}

func TestAuthAPIKeyInlineBeforePathAndMalformedFails(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	base := authConfig{TenancyOCID: "tenancy", UserOCID: "user", Fingerprint: "fp", Region: "us-ashburn-1", PrivateKey: strings.ReplaceAll(key, "\n", `\n`), PrivateKeyPath: "nonexistent"}
	if _, err := buildConfigurationProvider(base); err != nil {
		t.Fatal(err)
	}
	base.PrivateKey = "SENSITIVE-INVALID-KEY"
	if _, err := buildConfigurationProvider(base); err == nil || strings.Contains(err.Error(), base.PrivateKey) || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected safe private-key error, got %v", err)
	}
	base.PrivateKey, base.PrivateKeyPath = "", ""
	if _, err := buildConfigurationProvider(base); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestAuthAPIKeyEncryptedPrivateKey(t *testing.T) {
	authTestEnvironment(t)
	key, _ := authTestPrivateKey(t)
	password := "test-encryption-password"
	block, err := x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), []byte(password), x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	material := string(pem.EncodeToMemory(block))
	keyFile := filepath.Join(t.TempDir(), "encrypted.pem")
	authTestWrite(t, keyFile, material)
	c := authConfig{TenancyOCID: "tenancy", UserOCID: "user", Fingerprint: "fp", Region: "us-ashburn-1", PrivateKeyPath: keyFile}
	t.Setenv("OCI_PRIVATE_KEY_PASSWORD", password)
	if _, err := buildConfigurationProvider(c); err != nil {
		t.Fatal(err)
	}
	c.PrivateKeyPassword = "WRONG-SECRET-PASSWORD"
	if _, err := buildConfigurationProvider(c); err == nil || strings.Contains(err.Error(), c.PrivateKeyPassword) {
		t.Fatalf("wrong password accepted or disclosed: %v", err)
	}
}

func TestAuthAPIKeyMissingAndProfiles(t *testing.T) {
	authTestEnvironment(t)
	_, err := buildConfigurationProvider(authConfig{})
	if err == nil || !strings.Contains(err.Error(), "tenancy_ocid") || !strings.Contains(err.Error(), "region") {
		t.Fatalf("missing fields error: %v", err)
	}
	_, err = buildConfigurationProvider(authConfig{ConfigFile: filepath.Join(t.TempDir(), "missing"), ConfigFileProfile: "NAMED"})
	if err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatal("missing named profile accepted")
	}
}

func TestAuthConfigurationFileSelection(t *testing.T) {
	authTestEnvironment(t)
	home, _ := os.UserHomeDir()
	primary := filepath.Join(home, ".oci", "config")
	fallback := filepath.Join(t.TempDir(), "fallback")
	t.Setenv("OCI_CONFIG_FILE", fallback)
	paths, _ := configurationFilePaths(authConfigWithDefaults(authConfig{}))
	if paths[0] != fallback || len(paths) != 2 {
		t.Fatalf("fallback paths: %v", paths)
	}
	authTestWrite(t, primary, "[DEFAULT]\n")
	paths, _ = configurationFilePaths(authConfigWithDefaults(authConfig{}))
	if paths[0] != primary {
		t.Fatalf("existing home config must win OCI_CONFIG_FILE: %v", paths)
	}
	paths, _ = configurationFilePaths(authConfigWithDefaults(authConfig{ConfigFile: fallback}))
	if len(paths) != 1 || paths[0] != fallback {
		t.Fatal("explicit config_file must win")
	}
	paths, _ = configurationFilePaths(authConfigWithDefaults(authConfig{ConfigFileProfile: "NAMED"}))
	if len(paths) != 1 {
		t.Fatal("named profile must not fall back to legacy config")
	}
}

func TestAuthSecurityTokenRotationAndExpiration(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	dir := t.TempDir()
	keyPath, tokenPath, configPath := filepath.Join(dir, "key"), filepath.Join(dir, "token"), filepath.Join(dir, "config")
	authTestWrite(t, keyPath, key)
	first := authTestToken(t, time.Now().Add(time.Hour), nil)
	authTestWrite(t, tokenPath, first+"\n")
	authTestWrite(t, configPath, fmt.Sprintf("[SESSION]\ntenancy=test-tenancy\nfingerprint=fp\nregion=us-phoenix-1\nkey_file=%s\nsecurity_token_file=%s\n", keyPath, tokenPath))
	c := authConfig{Mode: "SecurityToken", ConfigFile: configPath, ConfigFileProfile: "SESSION"}
	provider, err := buildConfigurationProvider(c)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := provider.Region(); got != "us-phoenix-1" {
		t.Fatalf("profile region got %s", got)
	}
	if got, _ := provider.KeyID(); got != "ST$"+first {
		t.Fatal("token was not trimmed")
	}
	second := authTestToken(t, time.Now().Add(2*time.Hour), nil)
	authTestWrite(t, tokenPath, second)
	if got, _ := provider.KeyID(); got != "ST$"+second {
		t.Fatal("session token rotation was not read")
	}
	if !provider.(common.RefreshableConfigurationProvider).Refreshable() {
		t.Fatal("session refresh marker missing")
	}
	if _, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider); err != nil {
		t.Fatal(err)
	}
	authTestWrite(t, tokenPath, authTestToken(t, time.Now().Add(-time.Minute), nil))
	if _, err := provider.KeyID(); err == nil {
		t.Fatal("expired token accepted during subsequent signing")
	}
	if _, err := buildConfigurationProvider(c); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired token accepted at init: %v", err)
	}
	authTestWrite(t, tokenPath, "SECRET-INVALID-TOKEN")
	if _, err := buildConfigurationProvider(c); err == nil || strings.Contains(err.Error(), "SECRET-INVALID-TOKEN") {
		t.Fatal("invalid token accepted or disclosed")
	}
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	if _, err := buildConfigurationProvider(c); err == nil {
		t.Fatal("missing token accepted")
	}
}

func TestAuthSecurityTokenPrivateKeyPrecedenceAndFallback(t *testing.T) {
	authTestEnvironment(t)
	profileKey, profileMaterial := authTestPrivateKey(t)
	rawKey, rawMaterial := authTestPrivateKey(t)
	defaultKey, defaultMaterial := authTestPrivateKey(t)
	dir := t.TempDir()
	profilePath, rawPath, defaultPath := filepath.Join(dir, "profile-key"), filepath.Join(dir, "raw-key"), filepath.Join(dir, "default-key")
	tokenPath, configPath := filepath.Join(dir, "token"), filepath.Join(dir, "config")
	authTestWrite(t, profilePath, profileMaterial)
	authTestWrite(t, rawPath, rawMaterial)
	authTestWrite(t, defaultPath, defaultMaterial)
	authTestWrite(t, tokenPath, authTestToken(t, time.Now().Add(time.Hour), nil))
	writeProfile := func(selectedKey string) {
		authTestWrite(t, configPath, fmt.Sprintf("[DEFAULT]\nkey_file=%s\n[SESSION]\ntenancy=test-tenancy\nfingerprint=fp\nregion=us-ashburn-1\nsecurity_token_file=%s\n%s", defaultPath, tokenPath, selectedKey))
	}
	writeProfile("key_file=" + profilePath + "\n")
	c := authConfig{Mode: "SecurityToken", ConfigFile: configPath, ConfigFileProfile: "SESSION", PrivateKey: rawMaterial}
	provider, err := buildConfigurationProvider(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.PrivateRSAKey()
	if err != nil || got.N.Cmp(profileKey.N) != 0 {
		t.Fatalf("session profile did not win raw key: %v", err)
	}
	// The SDK caches config files by pathname, as it does in the provider.
	// Use a second config file to exercise a different session profile.
	configPath = filepath.Join(dir, "fallback-config")
	writeProfile("")
	c.ConfigFile = configPath
	c.PrivateKey, c.PrivateKeyPath = "", rawPath
	provider, err = buildConfigurationProvider(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err = provider.PrivateRSAKey()
	if err != nil || got.N.Cmp(rawKey.N) != 0 {
		t.Fatalf("raw key path fallback not used: %v", err)
	}
	c.PrivateKeyPath = ""
	provider, err = buildConfigurationProvider(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err = provider.PrivateRSAKey()
	if err != nil || got.N.Cmp(defaultKey.N) != 0 {
		t.Fatalf("DEFAULT key fallback not used: %v", err)
	}
	// A separate file avoids the SDK's expected configuration/key cache.
	c.ConfigFile = filepath.Join(dir, "missing-key-config")
	authTestWrite(t, c.ConfigFile, fmt.Sprintf("[SESSION]\ntenancy=test-tenancy\nfingerprint=fp\nregion=us-ashburn-1\nsecurity_token_file=%s\n", tokenPath))
	if _, err = buildConfigurationProvider(c); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("missing session key accepted: %v", err)
	}
}

func authTestCertificates(t *testing.T, expiration time.Time) string {
	t.Helper()
	dir := t.TempDir()
	key, material := authTestPrivateKey(t)
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test", OrganizationalUnit: []string{"opc-tenant:test-tenancy"}}, NotBefore: time.Now().Add(-time.Hour), NotAfter: expiration, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true}
	encoded, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemCertificate := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}))
	authTestWrite(t, filepath.Join(dir, "ip_cert.pem"), pemCertificate)
	authTestWrite(t, filepath.Join(dir, "intermediate.pem"), pemCertificate)
	authTestWrite(t, filepath.Join(dir, "ip_key.pem"), material)
	return dir
}

func TestAuthAllPrincipalModesUseSDKFactories(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	stub := common.NewRawConfigurationProvider("tenancy", "user", "us-ashburn-1", "fp", key, nil)
	dir := authTestCertificates(t, time.Now().Add(time.Hour))
	tokenFile := filepath.Join(t.TempDir(), "token")
	authTestWrite(t, tokenFile, authTestToken(t, time.Now().Add(time.Hour), nil))
	for _, mode := range []string{"InstancePrincipal", "InstancePrincipalWithCerts", "ResourcePrincipal", "OKEWorkloadIdentity", "WorkloadIdentityFederation"} {
		t.Run(mode, func(t *testing.T) {
			called := ""
			factories := authFactories{
				instancePrincipal: func(region string) (common.ConfigurationProvider, error) {
					if region != "us-phoenix-1" {
						t.Fatal("region lost")
					}
					called = "InstancePrincipal"
					return stub, nil
				},
				instanceCertificates: func(region string, c certificateCredentials) (common.ConfigurationProvider, error) {
					if len(c.leaf) == 0 || len(c.privateKey) == 0 || len(c.intermediate) != 1 {
						t.Fatal("certificate material lost")
					}
					called = "InstancePrincipalWithCerts"
					return stub, nil
				},
				resourcePrincipal: func(region string) (common.ConfigurationProvider, error) {
					called = "ResourcePrincipal"
					return stub, nil
				},
				okeWorkload: func() (common.ConfigurationProvider, error) { called = "OKEWorkloadIdentity"; return stub, nil },
				tokenExchange: func(issuer auth.TokenIssuer, b auth.TokenExchangeBuilder) (common.ConfigurationProvider, error) {
					if _, err := issuer.GetToken(); err != nil {
						t.Fatal(err)
					}
					if b.ClientId != "client" || b.ClientSecret != "secret" || b.SubjectTokenType != "jwt" {
						t.Fatal("token exchange inputs lost")
					}
					called = "WorkloadIdentityFederation"
					return stub, nil
				},
			}
			c := authConfig{Mode: mode, Region: "us-phoenix-1", CertificatesDirectory: dir, WorkloadIdentityTokenPath: tokenFile, TokenExchangeDomainURL: "https://example.identity.oraclecloud.com", TokenExchangeClientID: "client", TokenExchangeClientSecret: "secret", TokenExchangeRequestedTokenType: "urn:oci:token-type:oci-rpst", TokenExchangeResourceType: "resource"}
			provider, err := buildConfigurationProviderWithFactories(c, factories)
			if err != nil {
				t.Fatal(err)
			}
			if called != mode {
				t.Fatalf("got factory %s", called)
			}
			if got, _ := provider.Region(); got != "us-phoenix-1" {
				t.Fatal("region override lost")
			}
			if !provider.(common.RefreshableConfigurationProvider).Refreshable() {
				t.Fatal("refresh marker lost")
			}
		})
	}
}

func TestAuthCertificateValidationAndSDKConstruction(t *testing.T) {
	authTestEnvironment(t)
	valid := authTestCertificates(t, time.Now().Add(time.Hour))
	provider, err := buildConfigurationProvider(authConfig{Mode: "InstancePrincipalWithCerts", Region: "us-ashburn-1", CertificatesDirectory: valid})
	if err != nil {
		t.Fatal(err)
	}
	// Construction parses supplied certificates; it does not contact OCI until
	// a security token is requested. This is not live OCI authentication.
	if tenancy, err := provider.TenancyOCID(); err != nil || tenancy != "test-tenancy" {
		t.Fatalf("tenancy=%s error=%v", tenancy, err)
	}
	expired := authTestCertificates(t, time.Now().Add(-time.Minute))
	if _, err := buildConfigurationProvider(authConfig{Mode: "InstancePrincipalWithCerts", Region: "us-ashburn-1", CertificatesDirectory: expired}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired certificate: %v", err)
	}
	_, other := authTestPrivateKey(t)
	authTestWrite(t, filepath.Join(valid, "ip_key.pem"), other)
	if _, err := readCertificateCredentials(valid); err == nil || !strings.Contains(err.Error(), "match") {
		t.Fatal("mismatched certificate/key accepted")
	}
	if _, err := readCertificateCredentials(t.TempDir()); err == nil {
		t.Fatal("missing certificate accepted")
	}
}

func TestAuthFederationValidation(t *testing.T) {
	authTestEnvironment(t)
	base := authConfigWithDefaults(authConfig{Mode: "WorkloadIdentityFederation", Region: "us-ashburn-1", WorkloadIdentityTokenPath: "token", TokenExchangeDomainURL: "https://example.identity.oraclecloud.com", TokenExchangeRequestedTokenType: "urn:oci:token-type:oci-rpst", TokenExchangeResourceType: "resource", TokenExchangeClientID: "client", TokenExchangeClientSecret: "secret"})
	for _, test := range []struct {
		name   string
		change func(*authConfig)
		want   string
	}{
		{"missing-region", func(c *authConfig) { c.Region = "" }, "region"},
		{"missing-token", func(c *authConfig) { c.WorkloadIdentityTokenPath = "" }, "workload_identity_token_path"},
		{"missing-client", func(c *authConfig) { c.TokenExchangeClientSecret = "" }, "token_exchange_client_secret"},
		{"rpst-resource", func(c *authConfig) { c.TokenExchangeResourceType = "" }, "resource_type"},
		{"invalid-auth", func(c *authConfig) { c.TokenExchangeAuth = "bad" }, "token_exchange_auth"},
		{"client-conflict", func(c *authConfig) { c.TokenExchangeAuth = "InstancePrincipal" }, "must be unset"},
		{"insecure-domain", func(c *authConfig) { c.TokenExchangeDomainURL = "http://example.com" }, "HTTPS"},
		{"secret-domain", func(c *authConfig) { c.TokenExchangeDomainURL = "https://user:TOP-SECRET@example.com" }, "HTTPS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := base
			test.change(&c)
			err := validateFederationConfig(c)
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "TOP-SECRET") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestAuthFederationInstancePrincipalExchange(t *testing.T) {
	authTestEnvironment(t)
	file := filepath.Join(t.TempDir(), "token")
	authTestWrite(t, file, authTestToken(t, time.Now().Add(time.Hour), nil))
	_, key := authTestPrivateKey(t)
	stub := common.NewRawConfigurationProvider("tenancy", "user", "us-ashburn-1", "fp", key, nil)
	called := false
	f := authFactories{
		instancePrincipal: func(string) (common.ConfigurationProvider, error) { return stub, nil },
		tokenExchange: func(_ auth.TokenIssuer, b auth.TokenExchangeBuilder) (common.ConfigurationProvider, error) {
			called = true
			if b.InstancePrincipalProvider == nil || b.ClientId != "" || b.ClientSecret != "" {
				t.Fatal("incorrect instance exchange")
			}
			return stub, nil
		},
	}
	c := authConfig{Mode: "WorkloadIdentityFederation", Region: "us-ashburn-1", WorkloadIdentityTokenPath: file, TokenExchangeDomainURL: "https://example.com", TokenExchangeAuth: "InstancePrincipal", TokenExchangeRequestedTokenType: "urn:oci:token-type:oci-rpst", TokenExchangeResourceType: "resource"}
	if _, err := buildConfigurationProviderWithFactories(c, f); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("SDK token exchange not selected")
	}
}

func TestAuthProjectedTokenRotationAndInvalidCredentials(t *testing.T) {
	authTestEnvironment(t)
	file := filepath.Join(t.TempDir(), "token")
	issuer := projectedTokenFile{filename: file}
	if _, err := issuer.GetToken(); err == nil {
		t.Fatal("missing projected token accepted")
	}
	first := authTestToken(t, time.Now().Add(time.Hour), nil)
	authTestWrite(t, file, " \n"+first+"\n")
	if got, err := issuer.GetToken(); err != nil || got != first {
		t.Fatal("token not trimmed")
	}
	second := authTestToken(t, time.Now().Add(2*time.Hour), nil)
	authTestWrite(t, file, second)
	if got, err := issuer.GetToken(); err != nil || got != second {
		t.Fatal("rotated projected token not read")
	}
	for _, token := range []string{"", "SENSITIVE-TOKEN", authTestToken(t, time.Now().Add(-time.Minute), nil), "e30.eyJleHAiOiJpbnZhbGlkIn0.signature"} {
		authTestWrite(t, file, token)
		if _, err := issuer.GetToken(); err == nil || strings.Contains(err.Error(), "SENSITIVE-TOKEN") {
			t.Fatalf("unsafe error %v", err)
		}
	}
	// Provider parity: other subject-token formats are sent unchanged to the
	// OCI identity domain, which validates their format and expiration.
	issuer.subjectTokenType = "urn:ietf:params:oauth:token-type:access_token"
	authTestWrite(t, file, "opaque-subject-token")
	if got, err := issuer.GetToken(); err != nil || got != "opaque-subject-token" {
		t.Fatal("non-JWT token type rejected")
	}
}

// This server validates the real official SDK's exchange protocol without
// contacting OCI. It does not validate OCI issuer trust, IAM, or live refresh.
func TestAuthFederationSDKExchangeWithMockServer(t *testing.T) {
	authTestEnvironment(t)
	file := filepath.Join(t.TempDir(), "token")
	projected := authTestToken(t, time.Now().Add(time.Hour), nil)
	authTestWrite(t, file, projected)
	returned := authTestToken(t, time.Now().Add(time.Minute), nil)
	refreshed := authTestToken(t, time.Now().Add(time.Hour), nil)
	rotatedSubject := authTestToken(t, time.Now().Add(2*time.Hour), nil)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.URL.Path != "/oauth2/v1/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected token request %s %s", r.Method, r.URL.Path)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client" || secret != "secret" {
			t.Error("missing client credentials")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		expectedSubject := projected
		responseToken := returned
		if call > 1 {
			expectedSubject = rotatedSubject
			responseToken = refreshed
		}
		if r.Form.Get("subject_token") != expectedSubject || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" || r.Form.Get("public_key") == "" {
			t.Error("incorrect exchange payload")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"token":%q}`, responseToken)
	}))
	defer server.Close()
	f := sdkAuthFactories()
	f.tokenExchange = func(issuer auth.TokenIssuer, b auth.TokenExchangeBuilder) (common.ConfigurationProvider, error) {
		b.DomainUrl = server.URL
		return auth.TokenExchangeConfigurationProviderFromIssuer(issuer, b)
	}
	c := authConfig{Mode: "WorkloadIdentityFederation", Region: "us-ashburn-1", WorkloadIdentityTokenPath: file, TokenExchangeDomainURL: "https://example.com", TokenExchangeClientID: "client", TokenExchangeClientSecret: "secret", TokenExchangeRequestedTokenType: "urn:oci:token-type:oci-rpst", TokenExchangeResourceType: "resource"}
	provider, err := buildConfigurationProviderWithFactories(c, f)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := provider.KeyID(); err != nil || got != "ST$"+returned {
		t.Fatalf("key ID exchange failed: %v", err)
	}
	authTestWrite(t, file, rotatedSubject)
	if _, err := provider.PrivateRSAKey(); err != nil {
		t.Fatal(err)
	}
	if got, err := provider.KeyID(); err != nil || got != "ST$"+refreshed {
		t.Fatalf("refreshed token: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("unexpected exchange count %d", got)
	}
}

func TestAuthResourcePrincipalSDKFilesAndExpiredCredentials(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	dir := t.TempDir()
	keyFile, tokenFile := filepath.Join(dir, "key"), filepath.Join(dir, "token")
	authTestWrite(t, keyFile, key)
	first := authTestToken(t, time.Now().Add(time.Minute), nil)
	authTestWrite(t, tokenFile, first)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION", "2.2")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM", keyFile)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_RPST", tokenFile)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_REGION", "us-ashburn-1")
	// A present empty passphrase ENV counts as a configured passphrase in the
	// SDK, so preserve the existing absence instead of manufacturing it.
	if _, ok := os.LookupEnv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE"); ok {
		t.Setenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE", "")
		if err := os.Unsetenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE"); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := buildConfigurationProvider(authConfig{Mode: "ResourcePrincipal", Region: "us-phoenix-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := provider.KeyID(); err != nil || got != "ST$"+first {
		t.Fatalf("resource token read: %v", err)
	}
	second := authTestToken(t, time.Now().Add(time.Hour), nil)
	_, newKey := authTestPrivateKey(t)
	authTestWrite(t, keyFile, newKey)
	authTestWrite(t, tokenFile, second)
	if got, err := provider.KeyID(); err != nil || got != "ST$"+second {
		t.Fatalf("resource token refresh: %v", err)
	}
	if got, _ := provider.Region(); got != "us-phoenix-1" {
		t.Fatal("resource service region override lost")
	}
	if _, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider); err != nil {
		t.Fatal(err)
	}
	// New provider has no cached token, so validates the file's expiration.
	authTestWrite(t, tokenFile, authTestToken(t, time.Now().Add(-time.Minute), nil))
	expired, err := buildConfigurationProvider(authConfig{Mode: "ResourcePrincipal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expired.KeyID(); err == nil {
		t.Fatal("expired resource principal token accepted")
	}
	// Malformed tokens can be reflected by SDK errors; the boundary removes
	// that content before any backend diagnostic is created.
	authTestWrite(t, tokenFile, "SENSITIVE-MALFORMED-RESOURCE-TOKEN")
	malformed, err := buildConfigurationProvider(authConfig{Mode: "ResourcePrincipal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := malformed.KeyID(); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
		t.Fatalf("malformed token error: %v", err)
	}
}

func TestAuthResourcePrincipalV3SDKConstructionWithRegionOverride(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION", "3.0")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION_FOR_LEAF_RESOURCE", "2.2")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_FOR_LEAF_RESOURCE", key)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_RPST_FOR_LEAF_RESOURCE", authTestToken(t, time.Now().Add(time.Hour), nil))
	t.Setenv("OCI_RESOURCE_PRINCIPAL_REGION_FOR_LEAF_RESOURCE", "us-ashburn-1")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_RPT_URL_FOR_PARENT_RESOURCE", "https://example.com/resourcePrincipalToken")
	t.Setenv("OCI_RESOURCE_PRINCIPAL_RPST_ENDPOINT_FOR_PARENT_RESOURCE", "https://example.com/resourcePrincipalSessionToken")
	if _, ok := os.LookupEnv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE_FOR_LEAF_RESOURCE"); ok {
		t.Setenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE_FOR_LEAF_RESOURCE", "")
		if err := os.Unsetenv("OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE_FOR_LEAF_RESOURCE"); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := buildConfigurationProvider(authConfig{Mode: "ResourcePrincipal", Region: "us-phoenix-1"})
	if err != nil {
		t.Fatal(err)
	}
	if region, err := provider.Region(); err != nil || region != "us-phoenix-1" {
		t.Fatalf("version3 service region: %v", err)
	}
	if tenancy, err := provider.TenancyOCID(); err != nil || tenancy != "test-tenancy" {
		t.Fatalf("version3 leaf tenancy: %v", err)
	}
	// Only SDK construction and leaf-claim reading are tested. Requesting
	// the parent security token would contact an external runtime service.
}

func TestAuthHostPrincipalMissingRuntimeCredentials(t *testing.T) {
	authTestEnvironment(t)
	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION", "")
	for _, mode := range []string{"ResourcePrincipal", "OKEWorkloadIdentity"} {
		if _, err := buildConfigurationProvider(authConfig{Mode: mode}); err == nil || !strings.Contains(err.Error(), mode) {
			t.Fatalf("missing %s runtime credentials: %v", mode, err)
		}
	}
	// The instance metadata service is external to this test; inject its
	// credential-unavailable outcome rather than contacting a real instance.
	f := authFactories{instancePrincipal: func(string) (common.ConfigurationProvider, error) {
		return nil, errors.New("metadata credentials unavailable")
	}}
	if _, err := buildConfigurationProviderWithFactories(authConfig{Mode: "InstancePrincipal", Region: "us-ashburn-1"}, f); err == nil {
		t.Fatal("missing instance principal credentials accepted")
	}
}

func TestAuthEveryPrincipalModeCredentialFailuresAreSanitized(t *testing.T) {
	authTestEnvironment(t)
	dir := authTestCertificates(t, time.Now().Add(time.Hour))
	tokenPath := filepath.Join(t.TempDir(), "token")
	authTestWrite(t, tokenPath, authTestToken(t, time.Now().Add(time.Hour), nil))
	for _, test := range []struct{ mode, exchangeAuth string }{
		{"InstancePrincipal", ""}, {"InstancePrincipalWithCerts", ""},
		{"ResourcePrincipal", ""}, {"OKEWorkloadIdentity", ""},
		{"WorkloadIdentityFederation", "OAuthClientCredentials"}, {"WorkloadIdentityFederation", "InstancePrincipal"},
	} {
		t.Run(test.mode+test.exchangeAuth, func(t *testing.T) {
			for _, reason := range []string{"missing", "expired"} {
				failure := errors.New(reason + " credential SENSITIVE-PRIVATE-TOKEN")
				f := authFactories{
					instancePrincipal:    func(string) (common.ConfigurationProvider, error) { return nil, failure },
					instanceCertificates: func(string, certificateCredentials) (common.ConfigurationProvider, error) { return nil, failure },
					resourcePrincipal:    func(string) (common.ConfigurationProvider, error) { return nil, failure },
					okeWorkload:          func() (common.ConfigurationProvider, error) { return nil, failure },
					tokenExchange: func(auth.TokenIssuer, auth.TokenExchangeBuilder) (common.ConfigurationProvider, error) {
						return nil, failure
					},
				}
				c := authConfig{Mode: test.mode, Region: "us-ashburn-1", CertificatesDirectory: dir, WorkloadIdentityTokenPath: tokenPath, TokenExchangeDomainURL: "https://example.com", TokenExchangeAuth: test.exchangeAuth, TokenExchangeRequestedTokenType: "urn:oci:token-type:oci-rpst", TokenExchangeResourceType: "resource"}
				if test.exchangeAuth != "InstancePrincipal" {
					c.TokenExchangeClientID = "client"
					c.TokenExchangeClientSecret = "secret"
				}
				_, err := buildConfigurationProviderWithFactories(c, f)
				if err == nil || strings.Contains(err.Error(), "SENSITIVE") || !strings.Contains(err.Error(), test.mode) {
					t.Fatalf("%s failure unsafe: %v", reason, err)
				}
			}
		})
	}
}

type authFailingProvider struct{ common.ConfigurationProvider }

func (p authFailingProvider) KeyID() (string, error) {
	return "", errors.New("SENSITIVE-TOKEN SENSITIVE-PRIVATE-KEY")
}
func (p authFailingProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { panic("SENSITIVE-PRIVATE-KEY") }

func TestAuthSDKErrorsAndPanicsAreRedacted(t *testing.T) {
	p := &protectedConfigurationProvider{provider: authFailingProvider{}, mode: "ResourcePrincipal"}
	if _, err := p.KeyID(); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
		t.Fatalf("SDK error disclosed: %v", err)
	}
	if _, err := p.PrivateRSAKey(); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
		t.Fatalf("SDK panic disclosed: %v", err)
	}
	f := authFactories{resourcePrincipal: func(string) (common.ConfigurationProvider, error) { return nil, errors.New("SENSITIVE-TOKEN") }}
	if _, err := buildConfigurationProviderWithFactories(authConfig{Mode: "ResourcePrincipal"}, f); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
		t.Fatalf("factory error disclosed: %v", err)
	}
	f.resourcePrincipal = func(string) (common.ConfigurationProvider, error) { panic("SENSITIVE-RUNTIME-TOKEN") }
	if _, err := buildConfigurationProviderWithFactories(authConfig{Mode: "ResourcePrincipal"}, f); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
		t.Fatalf("factory panic disclosed: %v", err)
	}
}

func TestAuthConcurrentCredentialReads(t *testing.T) {
	authTestEnvironment(t)
	_, key := authTestPrivateKey(t)
	provider, err := buildConfigurationProvider(authConfig{TenancyOCID: "tenancy", UserOCID: "user", Fingerprint: "fp", Region: "us-ashburn-1", PrivateKey: key})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			for range 30 {
				if _, err := provider.KeyID(); err != nil {
					t.Error(err)
				}
				if _, err := provider.PrivateRSAKey(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	group.Wait()
}

func TestAuthUnknownModeAndInstanceRegionValidation(t *testing.T) {
	authTestEnvironment(t)
	if _, err := buildConfigurationProvider(authConfig{Mode: "SECRET-NOT-A-MODE"}); err == nil || strings.Contains(err.Error(), "SECRET-NOT-A-MODE") {
		t.Fatal("invalid mode accepted or reflected")
	}
	for _, mode := range []string{"InstancePrincipal", "InstancePrincipalWithCerts"} {
		if _, err := buildConfigurationProvider(authConfig{Mode: mode}); err == nil || !strings.Contains(err.Error(), "region") {
			t.Fatalf("missing region for %s: %v", mode, err)
		}
	}
}
