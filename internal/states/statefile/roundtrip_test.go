// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statefile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/enctest"
	"github.com/opentofu/opentofu/internal/states"
)

// This is a bit cursed, but we need to remove .Equals() in order to do a deep comparison.
// Benefits include: full comparsion of all fields, better diff output, ability to strip '\r' on windows.
type noEqualsState states.State
type noEqualsResource states.Resource
type noEqualsResourceInstance states.ResourceInstance
type noEqualsResourceInstanceObjectSrc states.ResourceInstanceObjectSrc

var cmpTransformers = []cmp.Option{
	cmp.Transformer("StateEquals", func(in *states.State) *noEqualsState {
		if in == nil {
			return nil
		}
		nes := noEqualsState(*in)
		return &nes
	}),
	cmp.Transformer("ResourceEquals", func(in *states.Resource) *noEqualsResource {
		if in == nil {
			return nil
		}
		nes := noEqualsResource(*in)
		return &nes
	}),
	cmp.Transformer("ResourceInstanceEquals", func(in *states.ResourceInstance) *noEqualsResourceInstance {
		if in == nil {
			return nil
		}
		nes := noEqualsResourceInstance(*in)
		return &nes
	}),
	cmp.Transformer("ResourceInstanceObjectSrcEquals", func(in *states.ResourceInstanceObjectSrc) *noEqualsResourceInstanceObjectSrc {
		if in == nil {
			return nil
		}
		nes := noEqualsResourceInstanceObjectSrc(*in)

		if runtime.GOOS == "windows" {
			// Unify AttrsJSON line endings by stripping \r
			nes.AttrsJSON = slices.DeleteFunc(slices.Clone(nes.AttrsJSON), func(b byte) bool { return b == '\r' })

		}

		return &nes
	}),
	cmp.Transformer("CTYVALUE", func(in cty.Value) string {
		return in.GoString()
	}),
	cmpopts.IgnoreFields(File{}, "TerraformVersion"),
}

func TestRoundtrip(t *testing.T) {
	const dir = "testdata/roundtrip"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range entries {
		const inSuffix = ".in.tfstate"
		const outSuffix = ".out.tfstate"

		if info.IsDir() {
			continue
		}
		inName := info.Name()
		if !strings.HasSuffix(inName, inSuffix) {
			continue
		}
		name := inName[:len(inName)-len(inSuffix)]
		outName := name + outSuffix

		t.Run(name, func(t *testing.T) {
			oSrcWant, err := os.ReadFile(filepath.Join(dir, outName))
			if err != nil {
				t.Fatal(err)
			}
			oWant, diags := readStateV4(oSrcWant)
			if diags.HasErrors() {
				t.Fatal(diags.Err())
			}

			ir, err := os.Open(filepath.Join(dir, inName))
			if err != nil {
				t.Fatal(err)
			}
			defer ir.Close()

			f, err := Read(ir, encryption.StateEncryptionDisabled())
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			var buf bytes.Buffer
			err = WriteIndent(f, &buf, encryption.StateEncryptionDisabled())
			if err != nil {
				t.Fatal(err)
			}
			oSrcWritten := buf.Bytes()

			oGot, diags := readStateV4(oSrcWritten)
			if diags.HasErrors() {
				t.Fatal(diags.Err())
			}

			if diff := cmp.Diff(oWant, oGot, cmpTransformers...); diff != "" {
				t.Error("wrong result:\n" + diff)
			}
		})
	}
}

func TestRoundtripEncryption(t *testing.T) {
	const path = "testdata/roundtrip/v4-modules.out.tfstate"

	enc := enctest.EncryptionWithFallback(t).State()

	unencryptedInput, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unencryptedInput.Close()

	// Read unencrypted using fallback
	originalState, err := Read(unencryptedInput, enc)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Check status
	if originalState.EncryptionStatus != encryption.StatusMigration {
		t.Fatal("wrong status")
	}

	// Write encrypted
	var encrypted bytes.Buffer
	err = WriteIndent(originalState, &encrypted, enc)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure it is encrypted / not readable
	encryptedCopy := encrypted
	_, err = Read(&encryptedCopy, encryption.StateEncryptionDisabled())
	if err == nil || err.Error() != "Unsupported state file format: This state file is encrypted and can not be read without an encryption configuration" {
		t.Fatalf("expected written state file to be encrypted!")
	}

	// Read encrypted
	newState, err := Read(&encrypted, enc)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Check status
	if newState.EncryptionStatus != encryption.StatusSatisfied {
		t.Fatal("wrong status")
	}

	// Overwrite status for deep comparison
	originalState.EncryptionStatus = newState.EncryptionStatus

	// Compare before/after encryption workflow
	if diff := cmp.Diff(originalState, newState, cmpTransformers...); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}
