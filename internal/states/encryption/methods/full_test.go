package methods

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
	RegisterFullMethod()
	RegisterPartialMethod()
	os.Exit(m.Run())
}

type tstConstantKeyProvider struct {
	key []byte
	err error
}

func tstNewConstantKeyProvider(key []byte, err error) encryptionflow.KeyProvider {
	return &tstConstantKeyProvider{key: key, err: err}
}

func (c *tstConstantKeyProvider) ProvideKey(info *encryptionflow.EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error) {
	return c.key, c.err
}

var validKey1 = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
	16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}

// since encryption introduces some randomness, we test using round trips of encryption, then decryption

type methodRoundtripTestCase struct {
	name                 string
	info                 *encryptionflow.EncryptionInfo
	config               encryptionconfig.Config
	keyProvider          encryptionflow.KeyProvider
	input                string
	skipEncryptSetOutput map[string]any
	expectedEncErr       error
	expectedDecErr       error
}

const validPlaintextFull = `{"cats":["cheetah", "serval", "snow leopard"]}`
const validPayloadKey1 = `ENC[xlHUk75KjmQew/nmfRvhIA+esn3OqI2snmPeXk4yCNZKE51+jcRL0ICMLD7dhQsOS3LLR4Edp+1kynPiS/ilVojaM/RL4QxRmLE=]`

func TestFullImpl(t *testing.T) {
	cut, err := newFull()
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
					Name: encryptionconfig.MethodFull,
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			input:       validPlaintextFull,
		},
		// encryption error cases
		{
			name: "encryption_key_provider_failure",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
				},
			},
			config:         encryptionconfig.Config{},
			keyProvider:    tstNewConstantKeyProvider(nil, errors.New("key provider failure")),
			input:          validPlaintextFull,
			expectedEncErr: errors.New("key provider failure"),
		},
		{
			name: "encryption_providing_salt_fails",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
					// fudge to force this error case. Flow doesn't set this during encryption
					Config: map[string]string{},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			input:       validPlaintextFull,
			// this error cannot normally happen
			expectedEncErr: errors.New("state or plan corrupt for method full - missing salt"),
		},
		{
			name: "encryption_fails",
			info: &encryptionflow.EncryptionInfo{
				Version:     1,
				KeyProvider: nil,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
				},
			},
			config:         encryptionconfig.Config{},
			keyProvider:    tstNewConstantKeyProvider(validKey1, nil),
			input:          "",
			expectedEncErr: errors.New("failed to encrypt with method full: plaintext is empty"),
		},
		// decryption error cases
		{
			name: "payload_not_present",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:               encryptionconfig.Config{},
			keyProvider:          tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{}, // missing field "payload"
			expectedDecErr:       errors.New("no field 'payload' in encrypted data"),
		},
		{
			name: "payload_not_marked",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{
				"payload": "not starting with ENC[", // this is the problem
			},
			expectedDecErr: errors.New("encrypted string did not have prefix - not produced with this method"),
		},
		{
			name: "decryption_key_provider_failure",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
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
					Name: encryptionconfig.MethodFull,
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
			expectedDecErr: errors.New("state or plan corrupt for method full - failed to decode salt"),
		},
		{
			name: "decryption_fails",
			info: &encryptionflow.EncryptionInfo{
				Version: 1,
				Method: encryptionflow.MethodInfo{
					Name: encryptionconfig.MethodFull,
					Config: map[string]string{
						"salt": "000102030405060708090a0b0c0d0e0f",
					},
				},
			},
			config:      encryptionconfig.Config{},
			keyProvider: tstNewConstantKeyProvider(validKey1, nil),
			skipEncryptSetOutput: map[string]any{
				"payload": "ENC[]",
			},
			expectedDecErr: errors.New("failed to decrypt with method full: encrypted payload too short, not even enough for the nonce"),
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
					// validate that the salt was added to info
					if tc.info.Method.Config == nil {
						t.Errorf("encryption failed to add Config to info")
						return
					}
					if _, ok := tc.info.Method.Config["salt"]; !ok {
						t.Errorf("encryption failed to set Config[salt] in info")
						return
					}

					// validate that resulting document has precisely the "payload" key
					if encrypted == nil {
						t.Errorf("encryption returned nil document despite no error")
						return
					}
					if len(encrypted) != 1 {
						t.Errorf("encrypted document has more than 1 key")
						return
					}
					if _, ok := encrypted["payload"]; !ok {
						t.Errorf("encrypted document is missing key 'payload'")
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
