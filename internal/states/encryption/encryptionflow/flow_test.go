package encryptionflow

import (
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

func TestMain(m *testing.M) {
	registerTestingKeyProvider()
	registerTestingErrorKeyProvider()
	registerTestingMethod()
	registerTestingErrorMethod()
	os.Exit(m.Run())
}

// --- passthrough ---

// tstNoConfigurationInstance constructs a Flow with no configuration.
//
// This is the most important case, because this will happen if tofu is run without
// any encryption configuration. We need to ensure all state and plans are passed through
// unchanged.
func tstNoConfigurationInstance() Flow {
	return New(
		"testing_no_configuration",
		nil,
		nil,
		hclog.NewNullLogger(),
	)
}

func tstPassthrough(t *testing.T, value string, method func([]byte) ([]byte, error)) {
	actual, err := method([]byte(value))
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if string(actual) != value {
		t.Error("failed to pass through")
	}
}

func TestDecryptState_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `{"version":"4"}`, cut.DecryptState)
}

func TestEncryptState_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `{"version":"4"}`, cut.EncryptState)
}

func TestDecryptPlan_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `zip64`, cut.DecryptPlan)
}

func TestEncryptPlan_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `zip64`, cut.EncryptPlan)
}

func TestDecryptState_PassthroughInvalid(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `invalid`, cut.DecryptState)
}

func TestEncryptState_PassthroughInvalid(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `invalid`, cut.EncryptState)
}

// --- valid configurations ---

func tstValidConfiguredInstance(t *testing.T, encPhrase string, decPhrase string) Flow {
	configKey := encryptionconfig.Key("unit_testing.testing_valid_configurations")

	var encConfig *encryptionconfig.Config
	var decConfig *encryptionconfig.Config

	if encPhrase != "" {
		encConfig = &encryptionconfig.Config{
			KeyProvider: encryptionconfig.KeyProviderConfig{
				Name: testingKeyProviderName,
				Config: map[string]string{
					"testphrase": encPhrase,
				},
			},
			Method: encryptionconfig.MethodConfig{
				Name: testingMethodName,
			},
		}
	}

	if decPhrase != "" {
		decConfig = &encryptionconfig.Config{
			KeyProvider: encryptionconfig.KeyProviderConfig{
				Name: testingKeyProviderName,
				Config: map[string]string{
					"testphrase": decPhrase,
				},
			},
			Method: encryptionconfig.MethodConfig{
				Name: testingMethodName,
			},
		}
	}

	return New(configKey, encConfig, decConfig, hclog.NewNullLogger())
}

type roundtripTestCase struct {
	name           string
	description    string
	primaryPhrase  string // used to construct encryption configuration
	fallbackPhrase string // used to construct decryption fallback config
	input          string
	injectOutput   string // inject this output after encryption step
	expectedEncErr error
	expectedDecErr error
}

const validPlaintext = `{"animals":[{"species":"cheetah","genus":"acinonyx"}]}`
const validEncryptedPhrase1 = `{"encryption":{"version":1,"key_provider":{"name":"testingkp","config":{"seed":"a real key provider might need something like this"}},"method":{"name":"testingmethod"}},"payload":"valid.dGhpcyBpcyB0aGUgZmlyc3QgcGhyYXNlYSByZWFsIGtleSBwcm92aWRlciBtaWdodCBuZWVkIHNvbWV0aGluZyBsaWtlIHRoaXM=.eyJhbmltYWxzIjpbeyJzcGVjaWVzIjoiY2hlZXRhaCIsImdlbnVzIjoiYWNpbm9ueXgifV19"}`
const invalidEncryptedInfo = `{"encryption":{"version":"hello"}}`

const phrase1 = `this is the first phrase`
const phrase2 = `this is the second phrase`
const phrase3 = `this is the third phrase`

func TestEncryptDecrypt(t *testing.T) {
	testCases := []roundtripTestCase{
		// happy path cases
		{
			name:        "no encryption",
			description: "unencrypted operation - no encryption configuration present, no fallback",
			input:       validPlaintext,
		},
		{
			name:          "normal",
			description:   "normal operation on encrypted data - main configuration, no fallback",
			primaryPhrase: phrase1,
			input:         validPlaintext,
		},
		{
			name:          "initial encrypt",
			description:   "initial encryption - main configuration, no fallback - must work anyway",
			primaryPhrase: phrase1,
			input:         validPlaintext,
			injectOutput:  validPlaintext,
		},
		{
			name:           "decrypt",
			description:    "decryption - no main configuration, but fallback",
			fallbackPhrase: phrase1,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
		},
		{
			name:           "already decrypted",
			description:    "unencrypted operation with fallback still present (decryption edge case) - no encryption configuration, but fallback - must still work anyway",
			input:          validPlaintext,
			fallbackPhrase: phrase1,
		},
		{
			name:           "key rotation",
			description:    "key rotation - main configuration for key 2, fallback key 1, read state with key 1 encryption - must work",
			primaryPhrase:  phrase2,
			fallbackPhrase: phrase1,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
		},
		{
			name:           "already rotated",
			description:    "key rotation - main configuration for key 2, fallback key 1, read state with key 2 encryption",
			primaryPhrase:  phrase2,
			fallbackPhrase: phrase1,
			input:          validPlaintext,
		},
		{
			name:           "initial encrypt during rotation",
			description:    "initial encryption happens during key rotation (key rotation edge case) - main configuration for key 1, fallback for key 2 - must still work anyway",
			primaryPhrase:  phrase1,
			fallbackPhrase: phrase2,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validPlaintext,
		},

		// error cases
		{
			name:           "wrong key",
			description:    "decryption fails due to wrong key - main configuration for key 3 - but state was encrypted with key 1",
			primaryPhrase:  phrase3,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
			expectedDecErr: errors.New("fake encrypted with wrong key"),
		},
		{
			name:           "wrong fallback key",
			description:    "decryption fails due to wrong fallback key during decrypt lifecycle - no main configuration, fallback configuration for key 3 - but state was encrypted with key 1 - must fail and not use passthrough",
			fallbackPhrase: phrase3,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
			expectedDecErr: errors.New("fake encrypted with wrong key"),
		},
		{
			name:           "two wrong keys",
			description:    "decryption fails due to two wrong keys - main configuration for key 3, fallback for key 2 - but state was encrypted with key 1",
			primaryPhrase:  phrase3,
			fallbackPhrase: phrase2,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
			expectedDecErr: errors.New("fake encrypted with wrong key"),
		},
		{
			name:           "no config but encrypted",
			description:    "decryption fails due to no config and no fallback - but the state is encrypted",
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   validEncryptedPhrase1,
			expectedDecErr: errors.New("failed to decrypt encrypted state or plan - completely missing configuration, maybe forgot to set environment variables"),
		},

		// corrupt state cases
		{
			name:           "invalid encryption info",
			description:    "decryption fails due to invalid encryption info structure",
			primaryPhrase:  phrase1,
			input:          validPlaintext, // exact value irrelevant for this test case
			injectOutput:   invalidEncryptedInfo,
			expectedDecErr: errors.New("encryption marker in payload has invalid structure: *"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log.Printf("test case: %s: %s", tc.name, tc.description)
			cut := tstValidConfiguredInstance(t, tc.primaryPhrase, tc.fallbackPhrase)

			encOutput, err := cut.EncryptState([]byte(tc.input))
			expectErr(t, err, tc.expectedEncErr)
			if err == nil {
				if tc.injectOutput != "" {
					encOutput = []byte(tc.injectOutput)
				}

				decOutput, err := cut.DecryptState(encOutput)
				expectErr(t, err, tc.expectedDecErr)
				if err == nil {
					if !compareSlices(decOutput, []byte(tc.input)) {
						t.Errorf("round trip error, got %#v; want %#v", decOutput, []byte(tc.input))
					}
				}
			}
		})
	}
}

func compareSlices(got []byte, expected []byte) bool {
	eEmpty := len(expected) == 0
	gEmpty := len(got) == 0
	if eEmpty != gEmpty {
		return false
	}
	if eEmpty {
		return true
	}
	if len(expected) != len(got) {
		return false
	}
	for i, v := range expected {
		if v != got[i] {
			return false
		}
	}
	return true
}

// --- configuration merge and validation ---

// --- error handling coverage ---

/*
func TestEncryptionConfigurationEnforcesSource(t *testing.T) {
	cut := New("testing_no_configuration")

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.EncryptionConfiguration(invalidConfigurationSource, encryptionconfig.Config{})
}

func TestDecryptionFallbackConfigurationEnforcesSource(t *testing.T) {
	cut := New("testing_no_configuration")

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.DecryptionFallbackConfiguration(invalidConfigurationSource, encryptionconfig.Config{})
}

func TestTryEncryptionDecryptionConstructionFailures(t *testing.T) {
	cut, err := New("testing_try_construction_failures").Build()
	if err != nil {
		t.Fatalf("unexpected construction failure of no configuration flow")
	}

	testCases := []struct {
		testcase    string
		method      encryptionconfig.EncryptionMethodName
		keyProvider encryptionconfig.KeyProviderName
		expectError error
	}{
		{
			testcase:    "method_unknown",
			method:      "unknown",
			keyProvider: testingKeyProviderName,
			expectError: errors.New("no registered encryption method 'unknown'"),
		},
		{
			testcase:    "keyprovider_unknown",
			method:      testingMethodName,
			keyProvider: "unknown",
			expectError: errors.New("no registered key provider 'unknown'"),
		},
	}

	for _, tc := range testCases {
		config := encryptionconfig.Config{
			Method:      encryptionconfig.EncryptionMethodConfig{Name: tc.method},
			KeyProvider: encryptionconfig.KeyProviderConfig{Name: tc.keyProvider},
		}

		t.Run(tc.testcase+"_try_decryption", func(t *testing.T) {
			result, err := cut.(*encryptionFlow).tryDecryptionWithConfig(nil, nil, &config)
			expectErr(t, err, tc.expectError)
			if len(result) != 0 {
				t.Errorf("tryDecryptionWithConfig() returned a nonempty result of length %d despite error", len(result))
			}
		})
		t.Run(tc.testcase+"_try_encryption", func(t *testing.T) {
			result, err := cut.(*encryptionFlow).tryEncryptionWithConfig([]byte{}, &config)
			expectErr(t, err, tc.expectError)
			if len(result) != 0 {
				t.Errorf("tryEncryptionWithConfig() returned a nonempty result of length %d despite error", len(result))
			}
		})
	}
}
*/

func TestConvertToEncryptionInfo_Errors(t *testing.T) {
	cut := New(
		"unit_testing.convert_to_encryption_info",
		nil,
		nil,
		hclog.NewNullLogger(),
	)

	var cannotMarshal any = func() {}
	var cannotUnmarshal = make(map[string]string)
	cannotUnmarshal["method"] = "hello"

	actual, err := cut.(*flow).convertToEncryptionInfo(cannotMarshal)
	expectErr(t, err, errors.New("encryption marker in payload has invalid structure: *"))
	if actual != nil {
		t.Errorf("returned a value despite error")
	}

	actual, err = cut.(*flow).convertToEncryptionInfo(cannotUnmarshal)
	expectErr(t, err, errors.New("encryption marker in payload has invalid structure: *"))
	if actual != nil {
		t.Errorf("returned a value despite error")
	}
}

func testingErrorInstance(t *testing.T, configKey string) (*flow, encryptionconfig.Config) {
	config := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Name: testingErrorKeyProviderName,
			Config: map[string]string{
				"key": "hello",
			},
		},
		Method: encryptionconfig.MethodConfig{
			Name:   testingErrorMethodName,
			Config: make(map[string]string),
		},
	}

	cut := New(
		"unit_testing.convert_to_encryption_info",
		nil,
		nil,
		hclog.NewNullLogger(),
	)

	return cut.(*flow), config
}

func TestMarshalEncrypted_Errors(t *testing.T) {
	cut, config := testingErrorInstance(t, "testing_marshal_encrypted")

	// this simulates an implementation error in the method where the result of the method
	// has EncryptionTopLevelJsonKey as a top level key
	payload := fmt.Sprintf(`{
  "encryptKey": "%s",
  "encryptValue": "value"
}`, EncryptionTopLevelJsonKey)

	actual, err := cut.tryEncryptionWithConfig([]byte(payload), &config)
	expectErr(t, err, errors.New("encryption internal error, reserved key 'encryption' is not empty in "+
		"encrypted document produced by method 'testingerrormethod' - this is a bug in the encryption method"))
	if len(actual) > 0 {
		t.Errorf("returned a value despite error")
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	t.Helper()
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error '%s' instead of success", actual.Error())
		} else if strings.HasSuffix(expected.Error(), "*") {
			expectStr := strings.TrimSuffix(expected.Error(), "*")
			if !strings.HasPrefix(actual.Error(), expectStr) {
				t.Errorf("received unexpected error\n'%s'\nthat does not start with\n'%s'", actual.Error(), expectStr)
			}
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error\n'%s'\ninstead of\n'%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}
