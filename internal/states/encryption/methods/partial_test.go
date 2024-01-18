package methods

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

const validPlaintextPartial = `{
  "cats": [
    "cheetah",
    {
      "snow leopard": {
        "color": "grey",
        "empty": "",
        "weight": 32
      }
    }
  ]
}`
const validEncryptedPartial = `{
  "cats": [
    "ENC[pIMUY/XPMMazC5s6B/R7j45h+rTjq37vidpxe/a9t9G9nTw=]",
    {
      "snow leopard": {
        "color": "ENC[AtaM+5ws9uDVmtDSsnGTjVcuAI7x6mPjmh2q2GsXORE=]",
        "empty": "ENC[QLsGVLXKFTML8AufLtNoxFloRqb5VogTFwasjw==]",
        "weight": 32
      }
    }
  ]
}`

func TestPartialImpl(t *testing.T) {
	cut, err := newPartial()
	expectErr(t, err, nil)
	if cut == nil {
		t.Fatalf("constructor unexpectedly returned nil")
	}

	testCases := []methodRoundtripTestCase{
		{
			name: "successful_roundtrip",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			input:       validPlaintextPartial,
		},
		// encryption error cases
		{
			name: "encryption_key_provider_failure",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
				},
			},
			config:         encryptionconfig.Config{},
			keyProvider:    tstNewConstantKeyProvider(nil, errors.New("key provider failure")),
			input:          validPlaintextPartial,
			expectedEncErr: errors.New("key provider failure"),
		},
		{
			name: "encryption_providing_salt_fails",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
					// fudge to force this error case. Flow doesn't set this during encryption
					Config: map[string]string{},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			input:       validPlaintextPartial,
			// this error cannot normally happen
			expectedEncErr: errors.New("state or plan corrupt for method partial - missing salt"),
		},
		{
			name: "encryption_empty_plaintext",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
				},
			},
			config:         encryptionconfig.Config{},
			keyProvider:    tstNewConstantKeyProvider(validKey1, nil),
			input:          "",
			expectedEncErr: errors.New("failed to parse state from json: detailed error not shown for security reasons"),
		},
		// decryption error cases
		{
			name: "payload_not_marked",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{
				"cats": []any{
					"not starting with ENC[", // this is the problem
				},
			},
			expectedDecErr: errors.New("error processing string field '(root).cats[0]': encrypted string did not have prefix - not produced with this method"),
		},
		{
			name: "decryption_key_provider_failure",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(nil, errors.New("key provider failure")), // causes error
			skipEncryptSetOutput: map[string]any{
				"payload": validPayloadKey1,
			},
			expectedDecErr: errors.New("key provider failure"),
		},
		{
			name: "decryption_providing_salt_fails",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
					Config: map[string]string{
						"salt": "ffeeddccbbaa",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{
				"payload": validPayloadKey1,
			},
			expectedDecErr: errors.New("state or plan corrupt for method partial - failed to decode salt"),
		},
		{
			name: "decryption_fails",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionMethodPartial,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{
				"cats": []any{
					"ENC[]",
				},
			},
			expectedDecErr: errors.New("error processing string field '(root).cats[0]': failed to decrypt: encrypted payload too short, not even enough for the nonce"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipEncryptSetOutput != nil {
				encrypted := tc.skipEncryptSetOutput
				decrypted, err := cut.Decrypt(encrypted, tc.info, tc.config, tc.keyProvider)
				expectErr(t, err, tc.expectedDecErr)
				if err == nil {
					if string(decrypted) != tc.input {
						t.Errorf("got decryption output %s instead of expected %s", string(decrypted), tc.input)
					}
				}
			} else {
				encrypted, err := cut.Encrypt([]byte(tc.input), tc.info, tc.config, tc.keyProvider)
				expectErr(t, err, tc.expectedEncErr)

				if err == nil {
					asJson, err := json.Marshal(encrypted)
					// validate that the basic structure is there
					if !strings.HasPrefix(string(asJson), `{"cats":["ENC[`) {
						t.Errorf("")
					}

					// validate that the salt was added to info
					if tc.info.Method.Config == nil {
						t.Errorf("encryption failed to add Config to info")
						return
					}
					if _, ok := tc.info.Method.Config["salt"]; !ok {
						t.Errorf("encryption failed to set Config[salt] in info")
						return
					}

					decrypted, err := cut.Decrypt(encrypted, tc.info, tc.config, tc.keyProvider)
					expectErr(t, err, tc.expectedDecErr)
					if err == nil {
						if string(decrypted) != tc.input {
							t.Errorf("got roundtrip output %s instead of expected %s", string(decrypted), tc.input)
						}
					}
				}
			}
		})
	}
}

func TestValidateEMPartialConfig(t *testing.T) {
	err := validateEMPartialConfig(encryptionconfig.MethodConfig{})
	expectErr(t, err, nil)

	err = validateEMPartialConfig(encryptionconfig.MethodConfig{
		Config: map[string]string{
			"unexpected": "unexpected value",
		},
	})
	expectErr(t, err, errors.New("unexpected fields, this method needs no configuration"))
}
