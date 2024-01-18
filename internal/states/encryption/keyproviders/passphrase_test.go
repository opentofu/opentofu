package keyproviders

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func TestPassphraseImpl_ProvideKey(t *testing.T) {
	cut, err := newPassphrase()
	expectErr(t, err, nil)
	if cut == nil {
		t.Fatalf("constructor unexpectedly returned nil")
	}

	baseError := "configuration for key provider passphrase needs key_provider.config.passphrase set - "

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
			name: "no_phrase_in_config",
			info: nil,
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{},
				},
			},
			expectKey: nil,
			expectErr: errors.New(baseError + "key_provider.config was present, but key_provider.config.passphrase was not"),
		},
		{
			name: "valid_phrase_no_salt",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Name: encryptionconfig.KeyProviderPassphrase,
				},
			},
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"passphrase": "super passphrase",
					},
				},
			},
			expectKey: nil,
			expectErr: errors.New("state or plan corrupt or not suitable for key provider passphrase - missing salt needed to recover the key"),
		},
		{
			name: "short_phrase", // still works
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Name: encryptionconfig.KeyProviderPassphrase,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"passphrase": "short",
					},
				},
			},
			expectKey: []byte{9, 49, 59, 226, 216, 91, 131, 77, 145, 84, 39, 186, 203, 36, 65, 228,
				150, 85, 2, 68, 81, 38, 216, 98, 20, 193, 27, 137, 170, 48, 211, 244},
			expectErr: nil,
		},
		{
			name: "valid_phrase_valid_salt",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Name: encryptionconfig.KeyProviderPassphrase,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config: &encryptionconfig.Config{
				KeyProvider: encryptionconfig.KeyProviderConfig{
					Config: map[string]string{
						"passphrase": "super passphrase",
					},
				},
			},
			expectKey: []byte{131, 180, 79, 116, 186, 196, 159, 245, 127, 233, 40, 206, 188, 241, 85, 54,
				182, 104, 58, 237, 145, 18, 60, 239, 104, 182, 137, 115, 127, 127, 218, 17},
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

func TestPassphraseImpl_provideAndRecordSalt(t *testing.T) {
	cut, err := newPassphrase()
	expectErr(t, err, nil)
	if cut == nil {
		t.Fatalf("constructor unexpectedly returned nil")
	}

	baseError := "state or plan corrupt or not suitable for key provider passphrase - "

	testCases := []struct {
		name             string
		info             *encryptionflow.EncryptionInfo
		expectSalt       string
		expectRandomSalt bool
		expectErr        error
	}{
		// encryption cases - randomly create a new salt and place it in info
		{
			name: "create_random_salt",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: nil,
			},
			expectRandomSalt: true,
			expectErr:        nil,
		},
		// decryption cases - salt is provided in info
		{
			name: "success",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			expectSalt: "000102030405060708090a0b0c0d0e0f",
			expectErr:  nil,
		},
		{
			name: "missing_config",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Config: nil,
				},
			},
			expectErr: errors.New(baseError + "missing salt needed to recover the key"),
		},
		{
			name: "missing_salt_in_config",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Config: map[string]string{},
				},
			},
			expectErr: errors.New(baseError + "missing salt needed to recover the key"),
		},
		{
			name: "salt_not_hex",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Config: map[string]string{
						"salt": "not a hex of 16 bytes",
					},
				},
			},
			expectErr: errors.New(baseError + "failed to decode salt needed to recover the key"),
		},
		{
			name: "salt_too_short",
			info: &encryptionflow.EncryptionInfo{
				KeyProvider: &encryptionflow.KeyProviderInfo{
					Config: map[string]string{
						"salt": "a0a1a2a3",
					},
				},
			},
			expectErr: errors.New(baseError + "failed to decode salt needed to recover the key"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := cut.(*passphraseImpl).provideAndRecordSalt(tc.info)
			expectErr(t, err, tc.expectErr)
			if err == nil {
				if tc.expectRandomSalt {
					// check salt was entered in info as hex
					if tc.info.KeyProvider == nil || tc.info.KeyProvider.Config == nil {
						t.Error("failed to add info.KeyProvider or info.KeyProvider.Config")
					} else {
						randomHexSalt, ok := tc.info.KeyProvider.Config["salt"]
						if !ok || len(randomHexSalt) != 32 {
							t.Error("failed to set info.KeyProvider.Config[salt] to a 32 character string")
						} else {
							salt, err := hex.DecodeString(randomHexSalt)
							expectErr(t, err, nil)

							// check same salt was returned
							expectKey(t, actual, salt)
						}
					}
				} else {
					if tc.info.KeyProvider == nil || tc.info.KeyProvider.Config == nil {
						t.Error("failed to conserve info.KeyProvider or info.KeyProvider.Config (or mistake in testcase)")
					} else {
						actualHexSalt, ok := tc.info.KeyProvider.Config["salt"]
						if !ok || actualHexSalt != tc.expectSalt {
							t.Error("failed to conserve info.KeyProvider.Config[salt] (or mistake in testcase)")
						} else {
							salt, err := hex.DecodeString(actualHexSalt)
							expectErr(t, err, nil)

							// check same salt was returned
							expectKey(t, actual, salt)
						}
					}
				}
			}
		})
	}
}
