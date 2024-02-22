package aesgcm_test

import (
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"testing"
)

func TestDescriptor(t *testing.T) {
	if id := aesgcm.New().ID(); id != "aes_gcm" {
		t.Fatalf("Incorrect descriptor ID returned: %s", id)
	}
}
