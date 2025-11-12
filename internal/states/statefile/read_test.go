// SPDX-License-Identifier: MPL-2.0

package statefile

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/enctest"
)

func TestReadErrNoState_emptyFile(t *testing.T) {
	emptyFile, err := os.Open("testdata/read/empty")
	if err != nil {
		t.Fatal(err)
	}
	defer emptyFile.Close()

	_, err = Read(emptyFile, encryption.StateEncryptionDisabled())
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}

func TestReadErrNoState_nilFile(t *testing.T) {
	nilFile, err := os.Open("")
	if err == nil {
		t.Fatal("wrongly succeeded in opening non-existent file")
	}

	_, err = Read(nilFile, encryption.StateEncryptionDisabled())
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}
func TestReadEmptyWithEncryption(t *testing.T) {
	payload := bytes.NewBufferString("")

	_, err := Read(payload, enctest.EncryptionRequired(t).State())
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}
func TestReadEmptyJsonWithEncryption(t *testing.T) {
	payload := bytes.NewBufferString("{}")

	_, err := Read(payload, enctest.EncryptionRequired(t).State())

	if err == nil || err.Error() != "unable to determine data structure during decryption: Given payload is not a state file" {
		t.Fatalf("expected encryption error, got %v", err)
	}
}
