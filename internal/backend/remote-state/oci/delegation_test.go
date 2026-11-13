// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

func clearDelegationEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"use_obo_token", "obo_token", "obo_token_path"} {
		for _, key := range []string{"TF_VAR_" + name, "OCI_" + name, name, "OCI_" + strings.ToUpper(name)} {
			t.Setenv(key, "")
		}
	}
}

func delegationTestClient(t *testing.T) (objectstorage.ObjectStorageClient, common.ConfigurationProvider, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	provider := common.NewRawConfigurationProvider("tenancy", "user", "us-ashburn-1", "fingerprint", keyPEM, nil)
	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		t.Fatal("cannot initialize mock SDK signing identity")
	}
	client.Host = "https://mock-oci.invalid"
	return client, provider, key
}

func TestDelegationLegacyEnvironmentPrecedence(t *testing.T) {
	for _, name := range []string{"use_obo_token", "obo_token", "obo_token_path"} {
		t.Run(name, func(t *testing.T) {
			clearDelegationEnvironment(t)
			keys := []string{"TF_VAR_" + name, "OCI_" + name, name, "OCI_" + strings.ToUpper(name)}
			for i, key := range keys {
				t.Setenv(key, string(rune('a'+i)))
			}
			for i, key := range keys {
				if got := delegationSetting(name, "fallback"); got != string(rune('a'+i)) {
					t.Fatal("legacy delegation setting precedence changed")
				}
				t.Setenv(key, "")
			}
			if delegationSetting(name, "fallback") != "fallback" {
				t.Fatal("delegation default was lost")
			}
		})
	}
}

func TestDelegationDisabledAndInvalidBoolean(t *testing.T) {
	clearDelegationEnvironment(t)
	if err := configureDelegation(nil, nil); err != nil {
		t.Fatal("delegation should default to disabled")
	}
	t.Setenv("use_obo_token", "false")
	t.Setenv("obo_token_path", "/missing/token")
	if err := configureDelegation(nil, nil); err != nil {
		t.Fatal("disabled delegation inspected credentials")
	}
	t.Setenv("TF_VAR_use_obo_token", "TOKEN_SECRET")
	if err := configureDelegation(nil, nil); err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
		t.Fatal("invalid boolean was accepted or leaked")
	}
}

func TestDelegationTokenValidationAndFilePrecedence(t *testing.T) {
	clearDelegationEnvironment(t)
	t.Setenv("obo_token", "inline-token")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("  file-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCI_obo_token_path", path)
	if token, err := readDelegationToken(); err != nil || token != "file-token" {
		t.Fatal("token file did not take precedence or whitespace was not trimmed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := readDelegationToken(); err == nil || strings.Contains(err.Error(), path) {
		t.Fatal("unreadable file silently fell back or disclosed its path")
	}
	t.Setenv("OCI_obo_token_path", "")
	for _, token := range []string{"", " ", "TOKEN_SECRET\r\nInjected: header"} {
		t.Setenv("obo_token", token)
		if _, err := readDelegationToken(); err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
			t.Fatal("unsafe or missing token was accepted or disclosed")
		}
	}
	t.Setenv("obo_token", "opaque.token.supported")
	if token, err := readDelegationToken(); err != nil || token != "opaque.token.supported" {
		t.Fatal("opaque delegation tokens lost provider compatibility")
	}
	if err := os.WriteFile(path, []byte("TOKEN_SECRET\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCI_obo_token_path", path)
	if _, err := readDelegationToken(); err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
		t.Fatal("invalid file header characters were accepted or disclosed")
	}
}

func TestDelegationRecognizableJWTExpiry(t *testing.T) {
	clearDelegationEnvironment(t)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	for _, tc := range []struct{ name, payload string }{
		{"expired", `{"exp":1,"secret":"TOKEN_SECRET"}`},
		{"missing-exp", `{"secret":"TOKEN_SECRET"}`},
		{"invalid-exp", `{"exp":"TOKEN_SECRET"}`},
		{"invalid-payload", `TOKEN_SECRET`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("obo_token", header+"."+base64.RawURLEncoding.EncodeToString([]byte(tc.payload))+".signature")
			_, err := readDelegationToken()
			if err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
				t.Fatal("invalid JWT was accepted or leaked")
			}
		})
	}
	// An unexpired recognizable JWT is accepted, while OCI remains responsible
	// for verifying its signature, subject, scope, and delegation permissions.
	payload := []byte(`{"exp":4102444800}`)
	t.Setenv("obo_token", header+"."+base64.RawURLEncoding.EncodeToString(payload)+".signature")
	if _, err := readDelegationToken(); err != nil {
		t.Fatal("unexpired JWT was rejected")
	}
}

func TestDelegationInitializationFailsClosed(t *testing.T) {
	clearDelegationEnvironment(t)
	t.Setenv("use_obo_token", "true")
	client, provider, _ := delegationTestClient(t)
	if err := configureDelegation(&client, provider); err == nil {
		t.Fatal("enabled delegation without token was accepted")
	}
	t.Setenv("obo_token_path", filepath.Join(t.TempDir(), "missing"))
	if err := configureDelegation(&client, provider); err == nil {
		t.Fatal("unreadable delegation token file was accepted")
	}
}

// All transport calls are in-memory mocks. Verify the real OCI SDK's RSA
// signature, including the delegation header on its custom PutObject signer.
func TestDelegationOfficialSDKCRUDSignaturesAndRotation(t *testing.T) {
	clearDelegationEnvironment(t)
	t.Setenv("use_obo_token", "true")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("first-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("obo_token_path", path)
	client, provider, key := delegationTestClient(t)
	previousCalls := 0
	client.Interceptor = func(req *http.Request) error {
		previousCalls++
		req.Header.Set("existing-interceptor", "preserved")
		return nil
	}
	if err := configureDelegation(&client, provider); err != nil {
		t.Fatal(err)
	}
	store := newMockObjectStore()
	expected := "first-token"
	var methods []string
	client.HTTPClient = &http.Client{Transport: mockSDKTransport{http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get(delegationHeader) != expected || req.Header.Get("existing-interceptor") != "preserved" {
			t.Fatal("rotating delegation token or preceding interceptor was not applied")
		}
		verifyDelegationSignature(t, req, key)
		methods = append(methods, req.Method)
		name := strings.TrimPrefix(req.URL.Path, "/n/namespace/b/bucket/o/")
		switch req.Method {
		case http.MethodGet:
			resp, err := store.GetObject(req.Context(), objectstorage.GetObjectRequest{ObjectName: common.String(name)})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			defer resp.Content.Close()
			w.Header().Set("ETag", *resp.ETag)
			w.Header().Set("Content-MD5", *resp.ContentMd5)
			io.Copy(w, resp.Content)
		case http.MethodPut:
			policy := common.NoRetryPolicy()
			resp, err := store.PutObject(req.Context(), objectstorage.PutObjectRequest{ObjectName: common.String(name), PutObjectBody: req.Body,
				ContentLength: common.Int64(req.ContentLength), ContentMD5: common.String(req.Header.Get("Content-MD5")),
				IfMatch: optionalString(req.Header.Get("If-Match")), IfNoneMatch: optionalString(req.Header.Get("If-None-Match")),
				OpcMeta: map[string]string{writeTokenMetadata: req.Header.Get("opc-meta-" + writeTokenMetadata)}, RequestMetadata: common.RequestMetadata{RetryPolicy: &policy}})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			w.Header().Set("ETag", *resp.ETag)
		case http.MethodDelete:
			_, err := store.DeleteObject(req.Context(), objectstorage.DeleteObjectRequest{ObjectName: common.String(name), IfMatch: optionalString(req.Header.Get("If-Match"))})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			w.WriteHeader(204)
		default:
			t.Fatal("unexpected SDK operation")
		}
	})}}
	c := testRemoteClient(&client)
	if _, err := c.Get(t.Context()); err != nil {
		t.Fatal(err)
	}
	expected = "second-token"
	if err := os.WriteFile(path, []byte(expected), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(t.Context(), []byte("state")); err != nil {
		t.Fatal(err)
	}
	expected = "third-token"
	if err := os.WriteFile(path, []byte(expected), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "GET,PUT,DELETE" || previousCalls != 3 {
		t.Fatal("expected delegated SDK CRUD operations")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(t.Context()); err == nil || len(methods) != 3 {
		t.Fatal("missing rotated token did not fail closed before transport")
	}
}

func verifyDelegationSignature(t *testing.T, request *http.Request, key *rsa.PrivateKey) {
	t.Helper()
	authorization := request.Header.Get("Authorization")
	_, headers, ok := strings.Cut(authorization, `headers="`)
	if !ok {
		t.Fatal("SDK request has no signed-header declaration")
	}
	headers, _, ok = strings.Cut(headers, `"`)
	if !ok || !strings.Contains(" "+headers+" ", " "+delegationHeader+" ") {
		t.Fatal("SDK omitted the delegation header from its signature")
	}
	_, signature, ok := strings.Cut(authorization, `signature="`)
	if !ok {
		t.Fatal("SDK request has no signature")
	}
	signature, _, ok = strings.Cut(signature, `"`)
	if !ok {
		t.Fatal("SDK request signature is malformed")
	}
	var signed []string
	for _, header := range strings.Fields(headers) {
		value := request.Header.Get(header)
		if header == "host" {
			value = request.URL.Host
		}
		if header == "(request-target)" {
			value = strings.ToLower(request.Method) + " " + request.URL.RequestURI()
		}
		signed = append(signed, header+": "+value)
	}
	digest := sha256.Sum256([]byte(strings.Join(signed, "\n")))
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], decoded) != nil {
		t.Fatal("SDK delegation header signature did not verify")
	}
}
