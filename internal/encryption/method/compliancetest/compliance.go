// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/compliancetest"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// ComplianceTest tests the functionality of a method to make sure it conforms to the expectations of the method
// interface.
func ComplianceTest[TDescriptor method.Descriptor, TConfig method.Config, TMethod method.Method](
	t *testing.T,
	testConfig TestConfiguration[TDescriptor, TConfig, TMethod],
) {
	testConfig.execute(t)
}

type TestConfiguration[TDescriptor method.Descriptor, TConfig method.Config, TMethod method.Method] struct {
	Descriptor TDescriptor
	// HCLParseTestCases contains the test cases of parsing HCL configuration and then validating it using the Build()
	// function.
	HCLParseTestCases map[string]HCLParseTestCase[TDescriptor, TConfig, TMethod]

	// ConfigStructT validates that a certain config results or does not result in a valid Build() call.
	ConfigStructTestCases map[string]ConfigStructTestCase[TConfig, TMethod]

	// ProvideTestCase exercises the entire chain and generates two keys.
	EncryptDecryptTestCase EncryptDecryptTestCase[TConfig, TMethod]
}

func (cfg *TestConfiguration[TDescriptor, TConfig, TMethod]) execute(t *testing.T) {
	t.Run("id", func(t *testing.T) {
		cfg.testID(t)
	})
	t.Run("hcl", func(t *testing.T) {
		cfg.testHCL(t)
	})
	t.Run("config-struct", func(t *testing.T) {
		cfg.testConfigStruct(t)
	})
	t.Run("encrypt-decrypt", func(t *testing.T) {
		cfg.EncryptDecryptTestCase.execute(t)
	})
}

func (cfg *TestConfiguration[TDescriptor, TConfig, TMethod]) testID(t *testing.T) {
	id := cfg.Descriptor.ID()
	if err := id.Validate(); err != nil {
		compliancetest.Fail(t, "Invalid ID returned from method descriptor: %s (%v)", id, err)
	} else {
		compliancetest.Log(t, "The ID provided by the method descriptor is valid: %s", id)
	}
}

func (cfg *TestConfiguration[TDescriptor, TConfig, TMethod]) testHCL(t *testing.T) {
	if cfg.HCLParseTestCases == nil {
		compliancetest.Fail(t, "Please provide a map to HCLParseTestCases.")
	}
	hasInvalidHCL := false
	hasValidHCLInvalidBuild := false
	hasValidBuild := false
	for name, tc := range cfg.HCLParseTestCases {
		if !tc.ValidHCL {
			hasInvalidHCL = true
		} else {
			if tc.ValidBuild {
				hasValidBuild = true
			} else {
				hasValidHCLInvalidBuild = true
			}
		}
		t.Run(name, func(t *testing.T) {
			tc.execute(t, cfg.Descriptor)
		})
	}
	t.Run("completeness", func(t *testing.T) {
		if !hasInvalidHCL {
			compliancetest.Fail(t, "Please provide at least one test case with an invalid HCL.")
		}
		if !hasValidHCLInvalidBuild {
			compliancetest.Fail(t, "Please provide at least one test case with a valid HCL that fails on Build()")
		}
		if !hasValidBuild {
			compliancetest.Fail(
				t,
				"Please provide at least one test case with a valid HCL that succeeds on Build()",
			)
		}
	})
}

func (cfg *TestConfiguration[TDescriptor, TConfig, TMethod]) testConfigStruct(t *testing.T) {
	compliancetest.ConfigStruct[TConfig](t, cfg.Descriptor.ConfigStruct())

	if cfg.ConfigStructTestCases == nil {
		compliancetest.Fail(t, "Please provide a map to ConfigStructTestCases.")
	}

	for name, tc := range cfg.ConfigStructTestCases {
		t.Run(name, func(t *testing.T) {
			tc.execute(t)
		})
	}
}

// HCLParseTestCase contains a test case that parses HCL into a configuration.
type HCLParseTestCase[TDescriptor method.Descriptor, TConfig method.Config, TMethod method.Method] struct {
	// HCL contains the code that should be parsed into the configuration structure.
	HCL string
	// ValidHCL indicates that the HCL block should be parsable into the configuration structure, but not necessarily
	// result in a valid Build() call.
	ValidHCL bool
	// ValidBuild indicates that calling the Build() function should not result in an error.
	ValidBuild bool
	// Validate is an extra optional validation function that can check if the configuration contains the correct
	// values parsed from HCL. If ValidBuild is true, the method will be passed as well.
	Validate func(config TConfig, method TMethod) error
}

func (h *HCLParseTestCase[TDescriptor, TConfig, TMethod]) execute(t *testing.T, descriptor TDescriptor) {
	parseError := false
	parsedConfig, diags := config.LoadConfigFromString("config.hcl", h.HCL)
	if h.ValidHCL {
		if diags.HasErrors() {
			compliancetest.Fail(t, "Unexpected HCL error (%v).", diags)
		} else {
			compliancetest.Log(t, "HCL successfully parsed.")
		}
	} else {
		if diags.HasErrors() {
			parseError = true
		}
	}

	configStruct := descriptor.ConfigStruct()
	diags = gohcl.DecodeBody(
		parsedConfig.MethodConfigs[0].Body,
		nil,
		configStruct,
	)
	var m TMethod
	if h.ValidHCL {
		if diags.HasErrors() {
			compliancetest.Fail(t, "Failed to parse empty HCL block into config struct (%v).", diags)
		} else {
			compliancetest.Log(t, "HCL successfully loaded into config struct.")
		}

		m = buildConfigAndValidate[TMethod](t, configStruct, h.ValidBuild)
	} else {
		if !parseError && !diags.HasErrors() {
			compliancetest.Fail(t, "Expected error during HCL parsing, but no error was returned.")
		} else {
			compliancetest.Log(t, "HCL loading errored correctly (%v).", diags)
		}
	}

	if h.Validate != nil {
		if err := h.Validate(configStruct.(TConfig), m); err != nil {
			compliancetest.Fail(t, "Error during validation and configuration (%v).", err)
		} else {
			compliancetest.Log(t, "Successfully validated parsed HCL config and applied modifications.")
		}
	} else {
		compliancetest.Log(t, "No ValidateAndConfigure provided, skipping HCL parse validation.")
	}
}

// ConfigStructTestCase validates that the config struct is behaving correctly when Build() is called.
type ConfigStructTestCase[TConfig method.Config, TMethod method.Method] struct {
	Config     TConfig
	ValidBuild bool
	Validate   func(method TMethod) error
}

func (m ConfigStructTestCase[TConfig, TMethod]) execute(t *testing.T) {
	newMethod := buildConfigAndValidate[TMethod, TConfig](t, m.Config, m.ValidBuild)
	if m.Validate != nil {
		if err := m.Validate(newMethod); err != nil {
			compliancetest.Fail(t, "method validation failed (%v)", err)
		}
	}
}

// EncryptDecryptTestCase handles a full encryption-decryption cycle.
type EncryptDecryptTestCase[TConfig method.Config, TMethod method.Method] struct {
	// ValidEncryptOnlyConfig is a configuration that has no decryption key and can only be used for encryption. The
	// key must match ValidFullConfig.
	ValidEncryptOnlyConfig TConfig
	// ValidFullConfig is a configuration that contains both an encryption and decryption key.
	ValidFullConfig TConfig
	// DecryptCannotBeVerified allows the decryption to succeed unencrypted data. This is needed for methods that
	// cannot verify if data decrypted successfully (e.g. xor).
	DecryptCannotBeVerified bool
}

func (m EncryptDecryptTestCase[TConfig, TMethod]) execute(t *testing.T) {
	if reflect.ValueOf(m.ValidEncryptOnlyConfig).IsNil() {
		compliancetest.Fail(t, "Please provide a ValidEncryptOnlyConfig to EncryptDecryptTestCase.")
	}
	if reflect.ValueOf(m.ValidFullConfig).IsNil() {
		compliancetest.Fail(t, "Please provide a ValidFullConfig to EncryptDecryptTestCase.")
	}

	encryptMethod := buildConfigAndValidate[TMethod, TConfig](t, m.ValidEncryptOnlyConfig, true)
	decryptMethod := buildConfigAndValidate[TMethod, TConfig](t, m.ValidFullConfig, true)

	plainData := []byte("Hello world!")
	encryptedData, err := encryptMethod.Encrypt(plainData)
	if err != nil {
		compliancetest.Fail(t, "Unexpected error after Encrypt() on the encrypt-only method (%v).", err)
	}

	_, err = encryptMethod.Decrypt(encryptedData)
	if err == nil {
		compliancetest.Fail(t, "Decrypt() did not fail without a decryption key.")
	} else {
		compliancetest.Log(t, "Decrypt() returned an error with a decryption key.")
	}
	var noDecryptionKeyError *method.ErrDecryptionKeyUnavailable
	if !errors.As(err, &noDecryptionKeyError) {
		compliancetest.Fail(t, "Decrypt() returned a %T instead of a %T without a decryption key. Please use the correct typed errors.", err, noDecryptionKeyError)
	} else {
		compliancetest.Log(t, "Decrypt() returned the correct error type of %T without a decryption key.", noDecryptionKeyError)
	}

	_, err = decryptMethod.Decrypt([]byte{})
	if err == nil {
		compliancetest.Fail(t, "Decrypt() must return an error when decrypting empty data, no error returned.")
	} else {
		compliancetest.Log(t, "Decrypt() correctly returned an error when decrypting empty data.")
	}
	var typedDecryptError *method.ErrDecryptionFailed
	if !errors.As(err, &typedDecryptError) {
		compliancetest.Fail(t, "Decrypt() returned a %T instead of a %T when decrypting empty data. Please use the correct typed errors.", err, typedDecryptError)
	} else {
		compliancetest.Log(t, "Decrypt() returned the correct error type of %T when decrypting empty data.", typedDecryptError)
	}
	typedDecryptError = nil

	if !m.DecryptCannotBeVerified {
		_, err = decryptMethod.Decrypt(plainData)
		if err == nil {
			compliancetest.Fail(t, "Decrypt() must return an error when decrypting unencrypted data, no error returned.")
		} else {
			compliancetest.Log(t, "Decrypt() correctly returned an error when decrypting unencrypted data.")
		}
		if !errors.As(err, &typedDecryptError) {
			compliancetest.Fail(t, "Decrypt() returned a %T instead of a %T when decrypting unencrypted data. Please use the correct typed errors.", err, typedDecryptError)
		} else {
			compliancetest.Log(t, "Decrypt() returned the correct error type of %T when decrypting unencrypted data.", typedDecryptError)
		}
	}

	decryptedData, err := decryptMethod.Decrypt(encryptedData)
	if err != nil {
		compliancetest.Fail(t, "Decrypt() failed to decrypt previously-encrypted data (%v).", err)
	} else {
		compliancetest.Log(t, "Decrypt() succeeded.")
	}

	if !bytes.Equal(decryptedData, plainData) {
		compliancetest.Fail(t, "Decrypt() returned incorrect plain text data:\n%v\nexpected:\n%v", decryptedData, plainData)
	} else {
		compliancetest.Log(t, "Decrypt() returned the correct plain text data.")
	}
}

func buildConfigAndValidate[TMethod method.Method, TConfig method.Config](
	t *testing.T,
	configStruct TConfig,
	validBuild bool,
) TMethod {
	if reflect.ValueOf(configStruct).IsNil() {
		compliancetest.Fail(t, "Nil struct passed!")
	}

	var typedMethod TMethod
	var ok bool
	kp, err := configStruct.Build()
	if validBuild {
		if err != nil {
			compliancetest.Fail(t, "Build() returned an unexpected error: %v.", err)
		} else {
			compliancetest.Log(t, "Build() did not return an error.")
		}
		typedMethod, ok = kp.(TMethod)
		if !ok {
			compliancetest.Fail(t, "Build() returned an invalid method type of %T, expected %T", kp, typedMethod)
		} else {
			compliancetest.Log(t, "Build() returned the correct method type of %T.", typedMethod)
		}
	} else {
		if err == nil {
			compliancetest.Fail(t, "Build() did not return an error.")
		} else {
			compliancetest.Log(t, "Build() correctly returned an error: %v", err)
		}

		var typedError *method.ErrInvalidConfiguration
		if !errors.As(err, &typedError) {
			compliancetest.Fail(
				t,
				"Build() did not return the correct error type, got %T but expected %T",
				err,
				typedError,
			)
		} else {
			compliancetest.Log(t, "Build() returned the correct error type of %T", typedError)
		}
	}
	return typedMethod
}
