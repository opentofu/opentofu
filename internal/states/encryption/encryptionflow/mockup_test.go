package encryptionflow

import (
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"strings"
	"testing"
)

// tstNoConfigurationInstance constructs a mockup Flow with no configuration.
//
// This is the most important case, because this will happen if tofu is run without
// any encryption configuration. We need to ensure all state and plans are passed through
// unchanged.
func tstNoConfigurationInstance() Flow {
	return NewMock("testing_no_configuration")
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

func tstCodeConfigurationInstance(encValid bool, decValid bool) Flow {
	encConfig := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Config: map[string]string{},
		},
		Method: encryptionconfig.EncryptionMethodConfig{},
	}
	if encValid {
		encConfig.KeyProvider.Config["passphrase"] = "a new passphrase"
	}

	decConfig := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Config: map[string]string{},
		},
		Method: encryptionconfig.EncryptionMethodConfig{},
	}
	if decValid {
		decConfig.KeyProvider.Config["passphrase"] = "the old passphrase"
	}

	cut := NewMock("testing_code_configuration")
	_ = cut.EncryptionConfiguration(ConfigurationSourceCode, encConfig)
	_ = cut.DecryptionFallbackConfiguration(ConfigurationSourceCode, decConfig)
	return cut
}

func TestMergeAndValidateConfigurations(t *testing.T) {
	testCases := []struct {
		testcase    string
		cut         Flow
		expectError error
	}{
		{
			testcase:    "no_configuration",
			cut:         tstNoConfigurationInstance(),
			expectError: nil,
		},
		{
			testcase:    "valid_configurations",
			cut:         tstCodeConfigurationInstance(true, true),
			expectError: nil,
		},
		{
			testcase:    "invalid_enc_config",
			cut:         tstCodeConfigurationInstance(false, true),
			expectError: errors.New("error invalid encryption configuration after merge: error in configuration for key provider passphrase: passphrase missing or empty"),
		},
		{
			testcase:    "invalid_dec_config",
			cut:         tstCodeConfigurationInstance(true, false),
			expectError: errors.New("error invalid decryption fallback configuration after merge: error in configuration for key provider passphrase: passphrase missing or empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			err := tc.cut.MergeAndValidateConfigurations()
			expectErr(t, err, tc.expectError)
		})
	}
}

func TestDecryptEncryptPropagateErrors(t *testing.T) {
	cut := tstCodeConfigurationInstance(false, true)
	expected := errors.New("error invalid encryption configuration after merge: error in configuration for key provider passphrase: passphrase missing or empty")

	_, err := cut.DecryptState([]byte(`{"version":"4"}`))
	expectErr(t, err, expected)

	_, err = cut.EncryptState([]byte(`{"version":"4"}`))
	expectErr(t, err, expected)

	_, err = cut.DecryptPlan([]byte(`zip64`))
	expectErr(t, err, expected)

	_, err = cut.EncryptPlan([]byte(`zip64`))
	expectErr(t, err, expected)
}

func expectErr(t *testing.T, actual error, expected error) {
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error '%s' instead of success", actual.Error())
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error '%s' instead of '%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}

func TestEncryptionConfigurationEnforcesSource(t *testing.T) {
	cut := tstNoConfigurationInstance()

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.EncryptionConfiguration(invalidConfigurationSource, encryptionconfig.Config{})
}

func TestDecryptionFallbackConfigurationEnforcesSource(t *testing.T) {
	cut := tstNoConfigurationInstance()

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.DecryptionFallbackConfiguration(invalidConfigurationSource, encryptionconfig.Config{})
}

func tstExpectPanic(t *testing.T, snippet string) func() {
	return func() {
		r := recover()
		if r == nil {
			t.Errorf("expected a panic")
		} else {
			actual := fmt.Sprintf("%v", r)
			if !strings.Contains(actual, snippet) {
				t.Errorf("panic message did not contain '%s'", snippet)
			}
		}
	}
}
