// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// This credential source models an OCI token/key renewal between SDK getter
// calls. It is independent of the snapshot implementation's retry logic.
type changingSigningCredentials struct {
	common.ConfigurationProvider
	keys                     []*rsa.PrivateKey
	keyIDs                   []string
	current                  int
	rotateOnce, rotateAlways bool
	keyReads, idReads        int
	failure                  string
}

func (p *changingSigningCredentials) KeyID() (string, error) {
	p.idReads++
	if p.failure == "id" {
		return "", errors.New("SENSITIVE-TOKEN")
	}
	if p.failure == "panic" {
		panic("SENSITIVE-TOKEN")
	}
	if len(p.keyIDs) > 0 {
		return p.keyIDs[p.current%len(p.keyIDs)], nil
	}
	return fmt.Sprintf("ST$test-token-%d", p.current), nil
}

func (p *changingSigningCredentials) PrivateRSAKey() (*rsa.PrivateKey, error) {
	p.keyReads++
	if p.failure == "key" {
		return nil, errors.New("SENSITIVE-PRIVATE-KEY")
	}
	if p.rotateAlways || p.rotateOnce {
		p.current++
		p.rotateOnce = false
	}
	return p.keys[p.current%len(p.keys)], nil
}

func (p *changingSigningCredentials) Refreshable() bool { return true }

func TestSigningSnapshotRetriesCredentialRenewal(t *testing.T) {
	oldKey, _ := authTestPrivateKey(t)
	newKey, _ := authTestPrivateKey(t)
	source := &changingSigningCredentials{keys: []*rsa.PrivateKey{oldKey, newKey}, rotateOnce: true}
	provider := newSnapshotSigningProvider(source)
	key, err := provider.PrivateRSAKey()
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := provider.KeyID()
	if err != nil || keyID != "ST$test-token-1" || key.N.Cmp(newKey.N) != 0 {
		t.Fatalf("key/token pair inconsistent: %v", err)
	}
	if source.keyReads != 2 || source.idReads != 4 {
		t.Fatalf("rotation did not require one snapshot retry: keys=%d ids=%d", source.keyReads, source.idReads)
	}
	// The following signature obtains a new pair instead of keeping a
	// credential snapshot indefinitely.
	source.current = 2
	key, err = provider.PrivateRSAKey()
	if err != nil {
		t.Fatal(err)
	}
	keyID, err = provider.KeyID()
	if err != nil || keyID != "ST$test-token-2" || key.N.Cmp(oldKey.N) != 0 {
		t.Fatalf("next signature used stale pair: %v", err)
	}
	if !provider.(common.RefreshableConfigurationProvider).Refreshable() {
		t.Fatal("refresh marker lost")
	}
}

func TestSigningSnapshotBoundsUnstableCredentialSource(t *testing.T) {
	key, _ := authTestPrivateKey(t)
	source := &changingSigningCredentials{keys: []*rsa.PrivateKey{key}, rotateAlways: true}
	provider := newSnapshotSigningProvider(source)
	if _, err := provider.PrivateRSAKey(); err == nil || !strings.Contains(err.Error(), "stable") {
		t.Fatalf("unstable source accepted: %v", err)
	}
	if source.keyReads != 3 || source.idReads != 6 {
		t.Fatalf("retry bound failed: keys=%d ids=%d", source.keyReads, source.idReads)
	}
	if provider.(*snapshotSigningProvider).pairedKeyID != "" {
		t.Fatal("failed snapshot retained a key ID")
	}
}

func TestSigningSnapshotDoesNotDiscloseCredentialErrors(t *testing.T) {
	key, _ := authTestPrivateKey(t)
	for _, failure := range []string{"id", "key", "panic"} {
		t.Run(failure, func(t *testing.T) {
			provider := newSnapshotSigningProvider(&changingSigningCredentials{keys: []*rsa.PrivateKey{key}, failure: failure})
			if _, err := provider.PrivateRSAKey(); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
				t.Fatalf("snapshot error disclosed: %v", err)
			}
			if failure != "key" {
				if _, err := provider.KeyID(); err == nil || strings.Contains(err.Error(), "SENSITIVE") {
					t.Fatalf("key ID error disclosed: %v", err)
				}
			}
		})
	}
}

func TestSigningSnapshotWithOfficialSDKRequestSigner(t *testing.T) {
	oldKey, _ := authTestPrivateKey(t)
	newKey, _ := authTestPrivateKey(t)
	source := &changingSigningCredentials{keys: []*rsa.PrivateKey{oldKey, newKey}, rotateOnce: true}
	provider := newSnapshotSigningProvider(source)
	signer := common.RequestSigner(provider, []string{"date", "(request-target)", "host"}, nil)
	request, err := http.NewRequest(http.MethodGet, "https://objectstorage.us-ashburn-1.oraclecloud.com/n/test/b/state/o/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Date", "Tue, 13 Oct 2026 10:00:00 GMT")
	if err := signer.Sign(request); err != nil {
		t.Fatal(err)
	}
	header := request.Header.Get("Authorization")
	if !strings.Contains(header, `keyId="ST$test-token-1"`) {
		t.Fatal("SDK signature used wrong token ID")
	}
	match := regexp.MustCompile(`signature="([^"]+)"`).FindStringSubmatch(header)
	if len(match) != 2 {
		t.Fatal("SDK signature missing")
	}
	signature, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		t.Fatal(err)
	}
	canonical := "date: " + request.Header.Get("Date") + "\n(request-target): get " + request.URL.RequestURI() + "\nhost: " + request.URL.Host
	digest := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(&newKey.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("signature does not match token's renewed key: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&oldKey.PublicKey, crypto.SHA256, digest[:], signature); err == nil {
		t.Fatal("signature unexpectedly used old key")
	}
}

type delayedSigningKeyID struct {
	common.ConfigurationProvider
	arrived chan<- struct{}
	release <-chan struct{}
}

func (p delayedSigningKeyID) KeyID() (string, error) {
	p.arrived <- struct{}{}
	<-p.release
	return p.ConfigurationProvider.KeyID()
}

func TestSigningSnapshotCanceledOperationCannotOverwriteAnotherPair(t *testing.T) {
	oldKey, _ := authTestPrivateKey(t)
	newKey, _ := authTestPrivateKey(t)
	oldID := "ST$" + authTestToken(t, time.Now().Add(time.Hour), map[string]any{"generation": 0})
	newID := "ST$" + authTestToken(t, time.Now().Add(time.Hour), map[string]any{"generation": 1})
	runtimeSource := &changingSigningCredentials{keys: []*rsa.PrivateKey{oldKey, newKey}, keyIDs: []string{oldID, newID}}
	protectedSource := &protectedConfigurationProvider{provider: runtimeSource, mode: "ResourcePrincipal"}
	first := newSnapshotSigningProvider(protectedSource)
	second := newSnapshotSigningProvider(protectedSource)
	arrived, release := make(chan struct{}), make(chan struct{})
	firstSigner := common.RequestSigner(delayedSigningKeyID{ConfigurationProvider: first, arrived: arrived, release: release}, common.DefaultGenericHeaders(), nil)
	secondSigner := common.RequestSigner(second, common.DefaultGenericHeaders(), nil)
	request := func(name string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, "https://objectstorage.us-ashburn-1.oraclecloud.com/n/test/b/state/o/"+name, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Date", "Tue, 13 Oct 2026 10:00:00 GMT")
		return r
	}
	firstRequest, secondRequest := request("first"), request("second")
	completed := make(chan error, 1)
	go func() { completed <- firstSigner.Sign(firstRequest) }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("first signer did not obtain its private key")
	}
	// The first SDK goroutine has made a signature with the old private key,
	// but has not read its cached ID yet. Model its caller returning on
	// cancellation and a second SDK operation starting after token renewal.
	runtimeSource.current = 1
	if err := secondSigner.Sign(secondRequest); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first signer did not finish")
	}
	verify := func(r *http.Request, key *rsa.PrivateKey, id string) {
		header := r.Header.Get("Authorization")
		if !strings.Contains(header, `keyId="`+id+`"`) {
			t.Fatal("overlapping operation overwrote signing token")
		}
		match := regexp.MustCompile(`signature="([^"]+)"`).FindStringSubmatch(header)
		if len(match) != 2 {
			t.Fatal("SDK signature missing")
		}
		signature, err := base64.StdEncoding.DecodeString(match[1])
		if err != nil {
			t.Fatal(err)
		}
		canonical := "date: " + r.Header.Get("Date") + "\n(request-target): get " + r.URL.RequestURI() + "\nhost: " + r.URL.Host
		digest := sha256.Sum256([]byte(canonical))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
			t.Fatalf("overlapping request signature has mismatched token/key: %v", err)
		}
	}
	verify(firstRequest, oldKey, oldID)
	verify(secondRequest, newKey, newID)
}
