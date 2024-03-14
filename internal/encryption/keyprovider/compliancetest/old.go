// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// TestCase contains a full compliance test case, starting from HCL and ending with an expected output.
type TestCase struct {
	// HCL is the initial HCL configuration that will be parsed into the ConfigStruct.
	HCL string
	// ValidHCL should be set to true if the HCL results in a valid configuration.
	ValidHCL bool
	// ValidateAndConfigure will be called after the HCL configuration is parsed. You should validate the values of the
	// parsed configuration here. Additionally, you may modify the configuration to inject additional values in this
	// function.
	ValidateAndConfigure ValidateAndConfigureFunc
	// StopAfterHCLValidation indicates that the test case should not continue with the full suite after the
	// ValidateAndConfigure function is done.
	StopAfterHCLValidation bool

	// ValidBuild should be true if the Build() function should succeed, or if a configuration error should be returned.
	ValidBuild bool

	// KeysMatchFunc is a function that compares an encryption and decryption key. It should return true if the two keys
	// belong together. Defaults to bytes.Equal
	KeysMatchFunc KeysMatchFunc
	// ExpectedOutput contains the expected key output of this key provider.
	ExpectedOutput *keyprovider.Output

	// CreateInvalidMetadata must create a metadata set that the provider considers invalid. This is only relevant
	// if the provider needs metadata.
	CreateInvalidMetadata func() any

	// ValidateOutput is a hook to validate if the created output and metadata are correct.
	ValidateOutput func(output keyprovider.Output, meta any) error
}

func (tc TestCase) prepare(t *testing.T, descriptor keyprovider.Descriptor) keyprovider.Config {
	log(t, "Preparing key provider config.")
	// Parse the config:
	parseError := false
	parsedConfig, diags := config.LoadConfigFromString("config.hcl", tc.HCL)
	if tc.ValidHCL {
		if diags.HasErrors() {
			fail(t, "Unexpected HCL error (%v).", diags)
		} else {
			log(t, "HCL successfully parsed.")
		}
	} else {
		if diags.HasErrors() {
			parseError = true
		}
	}

	configStruct := descriptor.ConfigStruct()
	diags = gohcl.DecodeBody(
		parsedConfig.KeyProviderConfigs[0].Body,
		nil,
		configStruct,
	)
	if tc.ValidHCL {
		if diags.HasErrors() {
			fail(t, "Failed to parse empty HCL block into config struct (%v).", diags)
		} else {
			log(t, "HCL successfully loaded into config struct.")
		}
	} else {
		if !parseError && !diags.HasErrors() {
			fail(t, "Expected error during HCL parsing, but no error was returned.")
		} else {
			log(t, "HCL loading errored correctly (%v).", diags)
		}
	}

	if tc.ValidateAndConfigure != nil {
		if err := tc.ValidateAndConfigure(configStruct); err != nil {
			fail(t, "Error during validation and configuration (%v).", err)
		} else {
			log(t, "Successfully validated parsed HCL config and applied modifications.")
		}
	} else {
		log(t, "No ValidateAndConfigure provided, skipping HCL parse validation.")
	}

	return configStruct
}

// ValidateAndConfigureFunc is a function that validates a parsed configuration and potentially modifies the
// configuration for the test case.
type ValidateAndConfigureFunc func(config keyprovider.Config) error

// TestCases is a map of a configuration name to a ValidateAndConfigureFunc.
type TestCases map[string]TestCase

// Validate checks if the TestCases is valid.
func (c TestCases) Validate() error {
	for name, testcase := range c {
		if !testNameRe.MatchString(name) {
			return fmt.Errorf("the TestCases name %s does not match the required expression %s", name, testNameRe.String())
		}
		if testcase.ValidateAndConfigure == nil {
			return fmt.Errorf("the ValidateAndConfigure function for %s is nil", name)
		}
	}
	return nil
}

// ComplianceConfigureResult contains the result of the configuration phase of the compliance test. You can provide
// a key comparison function and an expected key output.
type ComplianceConfigureResult struct {
	KeysMatchFunc  KeysMatchFunc
	ExpectedOutput *keyprovider.Output
}

// KeysMatchFunc is a function that returns true if the encryption and decryption keys match.
// It defaults to bytes.Equal.
type KeysMatchFunc func(encryptionKey []byte, decryptionKey []byte) bool

type HCLParseTestFunc func()

// OldComplianceTest tests if the key provider behaves as expected in certain situations. The error messages will indicate
// what changes need to be made. You can use this function in your tests to make sure your provider behaves correctly.
func OldComplianceTest(t *testing.T, descriptor keyprovider.Descriptor, testCases TestCases) {
	t.Run("config-struct", func(t *testing.T) {
	})

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			complianceTestCase(t, descriptor, testCase)
		})
	}
}

func complianceTestCase(t *testing.T, descriptor keyprovider.Descriptor, testCase TestCase) {
	configStruct := testCase.prepare(t, descriptor)

	if testCase.StopAfterHCLValidation {
		return
	}

	configStructPtrType := reflect.TypeOf(configStruct)
	configStructType := configStructPtrType.Elem()

	if !testCase.ValidBuild {
		complianceTestInvalidConfig(t, testCase, descriptor, configStructType)
	} else {
		complianceTestValidConfig(t, testCase, descriptor, configStructType)
	}
}

// complianceTestInvalidConfig tests that the key provider behavior is correct when supplied with a single invalid
// configuration.
func complianceTestInvalidConfig(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {
	configStruct := testCase.prepare(t, descriptor)

	_, _, err := configStruct.Build()
	if err == nil {
		fail(
			t,
			"Calling Build() with an invalid configuration on %s did not return an error.",
			configStructType.Name(),
		)
	} else {
		log(t, "Build() called successfully.")
	}
	var configError *keyprovider.ErrInvalidConfiguration
	var keyProviderFailure *keyprovider.ErrKeyProviderFailure
	if !errors.As(err, &configError) && !errors.As(err, &keyProviderFailure) {
		fail(
			t,
			"Calling Build() with an invalid configuration on %s returned an error, but it was not a *keyprovider.ErrInvalidConfiguration or *keyprovider.ErrKeyProviderFailure. Please make sure all your errors are typed errors.",
			configStructType.Name(),
		)
	} else {
		log(
			t,
			"The returned error is of the expected type.",
		)
	}
}

// complianceTestValidConfig tests the key provider behavior when the configuration is valid.
func complianceTestValidConfig(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {

	configStruct := testCase.prepare(t, descriptor)

	provider, inMeta, err := configStruct.Build()
	if err != nil {
		fail(
			t,
			"Error while calling Build() with a valid configuration on %s (%v).",
			configStructType.Name(),
			err,
		)
	} else {
		log(
			t,
			"Build() called successfully.",
		)
	}
	if provider == nil {
		fail(
			t,
			"Calling Build() with a valid configuration on %s returned nil for a key provider.",
			configStructType.Name(),
		)
	} else {
		log(
			t,
			"Provide() resulted in a non-nil provider.",
		)
	}

	_, _, err = provider.Provide(inMeta)
	if err != nil {
		fail(t, "Calling Provide() on %T with a valid configuration resulted in an error (%v).", provider, err)
	} else {
		log(t, "Provide() called successfully.")
	}
	if inMeta != nil {
		t.Run("meta-consistency", func(t *testing.T) {
			complianceTestMetaConsistency(t, testCase, descriptor, configStructType)
		})
		t.Run("no-decryption-key-on-missing-in-meta", func(t *testing.T) {
			complianceTestNoDecryptionKeyOnMissingInMeta(t, testCase, descriptor, configStructType)
		})

		if testCase.CreateInvalidMetadata != nil {
			t.Run("error-handling-invalid-metadata", func(t *testing.T) {
				complianceTestInvalidMetadata(t, testCase, descriptor, configStructType)
			})
		}
	} else {
		t.Run("meta-consistency", func(t *testing.T) {
			complianceTestNoMetaConsistency(t, testCase, descriptor, configStructType)
		})
	}

	t.Run("full-circle", func(t *testing.T) {
		complianceTestFullCircle(t, testCase, descriptor, configStructType)
	})
}

func complianceTestInvalidMetadata(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, structType reflect.Type) {
	log(t, "Testing provider behavior with invalid metadata.")

	cfg := testCase.prepare(t, descriptor)

	provider, _, err := cfg.Build()
	if err != nil {
		fail(t, "Build() returned an unexpected error (%v).", err)
	} else {
		log(t, "Build() succeeded.")
	}

	_, _, err = provider.Provide(testCase.CreateInvalidMetadata())
	if err == nil {
		fail(t, "Expected Provide() to return an error error, but nil returned.")
	} else {
		log(t, "Provide() correctly returned an error.")
	}
	var typedError *keyprovider.ErrInvalidMetadata
	if !errors.As(err, &typedError) {
		fail(
			t,
			"Provide() returned an incorrect error type of %T when supplied with invalid metadata. Please make sure that Provide() returns a %T instead.",
			err,
			typedError,
		)
	} else {
		log(t, "Provide() returned the correct error type of %T.", typedError)
	}
}

// complianceTestMetaConsistency tests if the key provider behaves consistently when it comes to metadata. This is
// important because errors in metadata handling may result in unrecoverable data, and we have no simple way of
// enforcing correctness through the compiler.
func complianceTestMetaConsistency(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {
	log(t, "Testing metadata type consistency for non-nil metadata...")

	configStruct := testCase.prepare(t, descriptor)
	provider, inMeta, err := configStruct.Build()
	if err != nil {
		fail(t, "Failed call Build() with valid configuration on %T (%v).", configStruct, err)
	} else {
		log(t, "Build() called successfully.")
	}

	inMetaPtrType := reflect.TypeOf(inMeta)
	if inMetaPtrType.Kind() != reflect.Ptr {
		fail(t, "When calling Build() on %s, the metadata returned is not a pointer to a struct but a %T.", configStructType.Name(), inMeta)
	} else {
		log(t, "The returned input metadata from Build() is a pointer.")
	}
	inMetaType := inMetaPtrType.Elem()
	if inMetaType.Kind() != reflect.Struct {
		fail(t, "When calling Build() on %s, the metadata returned is not a pointer to a struct but a pointer to %s.", configStructType.Name(), inMetaType.Name())
	} else {
		log(t, "The returned input metadata from Build() is a pointer to a struct.")
	}

	_, _, err = provider.Provide(nil)
	if err == nil {
		fail(t, "Calling Provide() on %T with a nil metadata did not result in an error even though the ConfigStruct() returned a meta structure.", provider)
	} else {
		log(t, "Provide() returned an error as expected.")
	}
	var typedErr *keyprovider.ErrInvalidMetadata
	if !errors.As(err, &typedErr) {
		fail(t, "Calling Provide() on %T with nil metadata returned a %T instead of a %T. Please use the correct typed errors.", provider, err, typedErr)
	} else {
		log(t, "The returned error is a %T", typedErr)
	}

	_, _, err = provider.Provide(struct{}{})
	if err == nil {
		fail(t, "Calling Provide() on %T with an incorrect data type metadata did not result in an error even though the ConfigStruct() returned a meta structure.", provider)
	} else {
		log(t, "Provide() returned an error as expected.")
	}
	if !errors.As(err, &typedErr) {
		fail(t, "Calling Provide() on %T with an incorrect data type metadata returned a %T instead of a %T. Please use the correct typed errors.", provider, err, typedErr)
	} else {
		log(t, "The returned error is a %T", typedErr)
	}

	_, outMeta, err := provider.Provide(inMeta)
	if err != nil {
		fail(t, "Calling Provide() on %T with a valid configuration resulted in an error (%v).", provider, err)
	} else {
		log(t, "Provide() returned no error, as expected.")
	}

	outMetaPtrType := reflect.TypeOf(outMeta)
	if outMetaPtrType.Kind() != reflect.Ptr {
		fail(t, "Calling Provide() on %T resulted in a metadata of the type %T, but it should have been a pointer to %s", provider, outMeta, inMetaType.Name())
	} else {
		log(t, "Output meta is a pointer.")
	}
	outMetaType := outMetaPtrType.Elem()
	if outMetaType.Kind() != reflect.Struct || outMetaType.Name() != inMetaType.Name() {
		fail(t, "Calling Provide() on %T resulted in a metadata of the type %T, but it should have been a pointer to %s", provider, outMeta, inMetaType.Name())
	} else {
		log(t, "Output meta is a pointer to a struct and matches the input meta.")
	}
}

// complianceTestNoMetaConsistency tests that the key provider returns no metadata on Provide if the Build() function
// did not return one. This is important because inconsistent metadata may lead to undefined behavior.
func complianceTestNoMetaConsistency(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {
	log(t, "Testing metadata consistency for nil metadata type...")

	cfg := testCase.prepare(t, descriptor)
	provider, inMeta, err := cfg.Build()
	if err != nil {
		fail(t, "Failed call Build() with valid configuration on %T (%v).", cfg, err)
	} else {
		log(t, "Build() called successfully.")
	}

	_, outMeta, err := provider.Provide(inMeta)
	if err != nil {
		fail(t, "Calling Provide() on %T with a valid configuration resulted in an error (%v).", provider, err)
	} else {
		log(t, "Provide() called successfully.")
	}
	if outMeta != nil {
		fail(
			t,
			"Calling Provide() on %T resulted in a non-nil metadata, even though calling Build() on %s returned a nil metadata. Please make sure that the metadata is always the same type.",
			provider,
			configStructType.Name(),
		)
	} else {
		log(t, "The output metadata is nil.")
	}
}

// complianceTestNoDecryptionKeyOnMissingInMeta tests if the key provider returns an empty decryption key if no input
// meta was provided. This is important because key providers that expect an input meta should not return a decryption
// key if they don't receive one.
func complianceTestNoDecryptionKeyOnMissingInMeta(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {
	log(t, "Testing if the missing input meta returns no decryption key...")

	cfg := testCase.prepare(t, descriptor)
	provider, inMeta, err := cfg.Build()
	if err != nil {
		fail(t, "Failed call Build() with valid configuration on %T (%v).", cfg, err)
	} else {
		log(t, "Build() called successfully.")
	}

	output, _, err := provider.Provide(inMeta)
	if err != nil {
		fail(t, "Calling Provide() on %T with a valid configuration resulted in an error (%v).", provider, err)
	} else {
		log(t, "Provide() called successfully.")
	}

	if len(output.DecryptionKey) != 0 {
		fail(
			t,
			"Calling Provide() on %T without input metadata resulted in a decryption key even though the metadata is required for this provider. Please make sure you only return a decryption key if you receive input metadata.",
			provider,
		)
	} else {
		log(t, "Calling Provide() on %T without input metadata returned no decryption key as expected.", provider)
	}
}

// complianceTestFullCircle tests a full-circle encryption behavior by calling the provider twice.
func complianceTestFullCircle(t *testing.T, testCase TestCase, descriptor keyprovider.Descriptor, configStructType reflect.Type) {
	log(t, "Running full circle test...")

	cfg := testCase.prepare(t, descriptor)
	provider, inMeta, err := cfg.Build()
	if err != nil {
		fail(t, "Failed call Build() with valid configuration on %T (%v).", cfg, err)
	} else {
		log(t, "Build() called successfully.")
	}

	output, outMeta, err := provider.Provide(inMeta)
	if err != nil {
		fail(t, "Calling Provide() on %T with a valid configuration resulted in an error (%v).", provider, err)
	} else {
		log(t, "Provide() called successfully.")
	}

	provider2, _, err := cfg.Build()
	if err != nil {
		fail(
			t,
			"Failed call Build() with valid configuration on %T (%v).",
			cfg,
			err,
		)
	} else {
		log(
			t,
			"Build() called successfully.",
		)
	}

	output2, _, err := provider2.Provide(outMeta)
	if err != nil {
		fail(
			t,
			"Failed call Provide() with valid configuration on %T (%v).",
			cfg,
			err,
		)
	} else {
		log(t, "Provide() called successfully.")
	}

	keysMatchfunc := testCase.KeysMatchFunc
	if keysMatchfunc == nil {
		keysMatchfunc = bytes.Equal
	}

	if !keysMatchfunc(output.EncryptionKey, output2.DecryptionKey) {
		fail(
			t,
			"The first encryption key does not match the second decryption key. The behavior of your key provider is not consistent or your key comparison function is incorrect. Please set a break point to the line of this error message and inspect the keys manually.",
		)
	} else {
		log(t, "The resulting encryption and decryption keys match each other.")
	}
	if testCase.ExpectedOutput != nil {
		if !bytes.Equal(testCase.ExpectedOutput.DecryptionKey, output2.DecryptionKey) {
			fail(
				t,
				"The resulting decryption key did not match the expected value. Please set a break point to the line of this error message and inspect the keys manually.",
			)
		} else {
			log(t, "The resulting decryption key matches the expected output.")
		}
		if !bytes.Equal(testCase.ExpectedOutput.EncryptionKey, output2.EncryptionKey) {
			fail(
				t,
				"The resulting encryption key did not match the expected value. Please set a break point to the line of this error message and inspect the keys manually.",
			)
		} else {
			log(t, "The resulting encryption key matches the expected output.")
		}
	}
}

func log(t *testing.T, msg string, params ...any) {
	t.Helper()
	t.Logf("\033[32m%s\033[0m", fmt.Sprintf(msg, params...))
}

func fail(t *testing.T, msg string, params ...any) {
	t.Helper()
	t.Fatalf("\033[31m%s\033[0m", fmt.Sprintf(msg, params...))
}
