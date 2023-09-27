// SPDX-License-Identifier: MPL-2.0

package statefile

import (
	"errors"
	"os"
	"testing"
)

func TestReadErrNoState_emptyFile(t *testing.T) {
	emptyFile, err := os.Open("testdata/read/empty")
	if err != nil {
		t.Fatal(err)
	}
	defer emptyFile.Close()

	_, err = Read(emptyFile)
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}

func TestReadErrNoState_nilFile(t *testing.T) {
	nilFile, err := os.Open("")
	if err == nil {
		t.Fatal("wrongly succeeded in opening non-existent file")
	}

	_, err = Read(nilFile)
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}
