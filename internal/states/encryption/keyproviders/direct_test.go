package keyproviders

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func TestMain(m *testing.M) {
	RegisterDirectKeyProvider()
	RegisterPassphraseKeyProvider()
	os.Exit(m.Run())
}

type keyProviderTestCase struct {
	name      string
	info      *encryptionflow.EncryptionInfo
	config    *encryptionconfig.Config
	expectKey []byte
	expectErr error
}

func TestDirectImpl_ProvideKey(t *testing.T) {
	cut, err := newDirect()
	expectErr(t, err, nil)
	if cut == nil {
		t.Fatalf("constructor unexpectedly returned nil")
	}

	baseError := "configuration for key provider direct needs key_provider.config.key set to a 64 character hexadecimal value (32 byte key) - "

	testCases := []keyProviderTestCase{
		{
			name: "no_config",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{},
			},
			expectKey: nil,
			expectErr: errors.New(baseError + "key_provider.config was not present"),
		},
		{
			name: "no_key_in_config",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{},
				},
			},
			expectKey: nil,
			expectErr: errors.New(baseError + "key_provider.config was present, but key_provider.config.key was not"),
		},
		{
			name: "wrong_key_length",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"key": "a0a1a2a3",
					},
				},
			},
			expectKey: nil,
			expectErr: errors.New(baseError + "value was 8 instead of 64 characters long"),
		},
		{
			name: "hex_decode_failure",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"key": "a0a1 some wonderful non-hex value that is 64 characters long 234",
					},
				},
			},
			expectKey: nil,
			expectErr: errors.New(baseError + "failed to decode hex value - omitting detailed error for security reasons"),
		},
		{
			name: "valid_key",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"key": "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
					},
				},
			},
			expectKey: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
				16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			expectErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := cut.ProvideKey(tc.info, tc.config)
			expectErr(t, err, tc.expectErr)
			expectKey(t, actual, tc.expectKey)
		})
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error '%s' instead of success", actual.Error())
		} else if strings.HasSuffix(expected.Error(), "*") {
			expectStr := strings.TrimSuffix(expected.Error(), "*")
			if !strings.HasPrefix(actual.Error(), expectStr) {
				t.Errorf("received unexpected error '%s' that does not start with '%s'", actual.Error(), expectStr)
			}
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error '%s' instead of '%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}

func expectKey(t *testing.T, actual []byte, expected []byte) {
	if len(actual) != len(expected) {
		t.Errorf("length of actual key %d is not %d as expected", len(actual), len(expected))
	} else {
		for i := range expected {
			if actual[i] != expected[i] {
				t.Errorf("key slice differs at position %d", i)
				return
			}
		}
	}
}

func TestMust_NoPanic(t *testing.T) {
	must(nil)
}

func TestMust_Panic(t *testing.T) {
	err := errors.New("message in the panic")

	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("expected a panic")
		} else {
			actual := fmt.Sprintf("%v", r)
			if !strings.Contains(actual, err.Error()) {
				t.Errorf("panic message did not contain '%s'", err.Error())
			}
		}
	}()

	must(err)
}
