// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/compliancetest"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

func ComplianceTest[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TMeta keyprovider.KeyMeta, TKeyProvider keyprovider.KeyProvider](
	t *testing.T,
	config TestConfiguration[TDescriptor, TConfig, TMeta, TKeyProvider],
) {
	var cfg TConfig
	cfgType := reflect.TypeOf(cfg)
	if cfgType.Kind() != reflect.Ptr || cfgType.Elem().Kind() != reflect.Struct {
		compliancetest.Fail(t, "You declared the config type to be %T, but it should be a pointer to a struct. Please fix your call to ComplianceTest().", cfg)
	}

	var meta TMeta
	metaType := reflect.TypeOf(cfg)
	if metaType.Kind() != reflect.Interface {
		if metaType.Kind() != reflect.Ptr || metaType.Elem().Kind() != reflect.Struct {
			compliancetest.Log(t, "You declared a metadata type as %T, but it should be a pointer to a struct. Please fix your call to ComplianceTest().", meta)
		}
	} else {
		compliancetest.Log(t, "Metadata type declared as interface{}, assuming the key provider does not need metadata. (This will be validated later.)")
	}

	t.Run("ID", func(t *testing.T) {
		complianceTestID(t, config)
	})

	t.Run("ConfigStruct", func(t *testing.T) {
		compliancetest.ConfigStruct[TConfig](t, config.Descriptor.ConfigStruct())

		t.Run("hcl-parsing", func(t *testing.T) {
			if config.HCLParseTestCases == nil {
				compliancetest.Fail(t, "Please provide a map in HCLParseTestCases.")
			}
			for name, tc := range config.HCLParseTestCases {
				tc := tc
				t.Run(name, func(t *testing.T) {
					complianceTestHCLParsingTestCase(t, tc, config)
				})
			}
		})

		t.Run("config", func(t *testing.T) {
			if config.ConfigStructTestCases == nil {
				compliancetest.Fail(t, "Please provide a map in ConfigStructTestCases.")
			}
			for name, tc := range config.ConfigStructTestCases {
				tc := tc
				t.Run(name, func(t *testing.T) {
					complianceTestConfigCase[TConfig, TKeyProvider, TMeta](t, tc)
				})
			}
		})
	})

	t.Run("metadata", func(t *testing.T) {
		if config.MetadataStructTestCases == nil {
			compliancetest.Fail(t, "Please provide a map in MetadataStructTestCases.")
		}
		for name, tc := range config.MetadataStructTestCases {
			tc := tc
			t.Run(name, func(t *testing.T) {
				complianceTestMetadataTestCase[TConfig, TKeyProvider, TMeta](t, tc)
			})
		}
	})

	t.Run("provide", func(t *testing.T) {
		complianceTestProvide[TDescriptor, TConfig, TKeyProvider, TMeta](t, config)
	})

	t.Run("test-completeness", func(t *testing.T) {
		t.Run("HCL", func(t *testing.T) {
			hasNotValidHCL := false
			hasValidHCLNotValidBuild := false
			hasValidHCLAndBuild := false
			for _, tc := range config.HCLParseTestCases {
				if !tc.ValidHCL {
					hasNotValidHCL = true
				} else {
					if tc.ValidBuild {
						hasValidHCLAndBuild = true
					} else {
						hasValidHCLNotValidBuild = true
					}
				}
			}
			if !hasNotValidHCL {
				compliancetest.Fail(t, "Please define at least one test with an invalid HCL.")
			}
			if !hasValidHCLNotValidBuild {
				compliancetest.Fail(t, "Please define at least one test with a valid HCL that will fail on Build() for validation.")
			}
			if !hasValidHCLAndBuild {
				compliancetest.Fail(t, "Please define at least one test with a valid HCL that will succeed on Build() for validation.")
			}
		})
		t.Run("metadata", func(t *testing.T) {
			hasNotPresent := false
			hasNotValid := false
			hasValid := false
			for _, tc := range config.MetadataStructTestCases {
				if !tc.IsPresent {
					hasNotPresent = true
				} else {
					if tc.IsValid {
						hasValid = true
					} else {
						hasNotValid = true
					}
				}
			}
			if !hasNotPresent {
				compliancetest.Fail(t, "Please provide at least one metadata test that represents non-present metadata.")
			}
			if !hasNotValid {
				compliancetest.Log(t, "Warning: Please provide at least one metadata test that represents an invalid metadata that is present.")
			}
			if !hasValid {
				compliancetest.Log(t, "Warning: Please provide at least one metadata test that represents a valid metadata.")
			}
		})
	})
}

func complianceTestProvide[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider, TMeta keyprovider.KeyMeta](
	t *testing.T,
	cfg TestConfiguration[TDescriptor, TConfig, TMeta, TKeyProvider],
) {
	if reflect.ValueOf(cfg.ProvideTestCase.ValidConfig).IsNil() {
		compliancetest.Fail(t, "Please provide a ValidConfig in ProvideTestCase.")
	}
	keyProviderConfig := cfg.ProvideTestCase.ValidConfig
	t.Run("nil-metadata", func(t *testing.T) {
		keyProvider, inMeta := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, keyProviderConfig, true)

		if reflect.ValueOf(inMeta).IsNil() {
			compliancetest.Skip(t, "The key provider does not have metadata (no metadata returned from Build()).")
			return
		}
		_, _, err := keyProvider.Provide(nil)
		if err == nil {
			compliancetest.Fail(t, "Provide() did not return no error when provided with nil metadata.")
		} else {
			compliancetest.Log(t, "Provide() correctly returned an error when provided with nil metadata (%v).", err)
		}
		var typedError *keyprovider.ErrInvalidMetadata
		if !errors.As(err, &typedError) {
			compliancetest.Fail(t, "Provide() returned an error of the type %T instead of %T. Please use the correct typed errors.", err, typedError)
		} else {
			compliancetest.Log(t, "Provide() correctly returned a %T when provided with nil metadata.", typedError)
		}
	})
	t.Run("incorrect-metadata-type", func(t *testing.T) {
		keyProvider, inMeta := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, keyProviderConfig, true)
		if reflect.ValueOf(inMeta).IsNil() {
			compliancetest.Skip(t, "The key provider does not have metadata (no metadata returned from Build()).")
			return
		}
		_, _, err := keyProvider.Provide(&struct{}{})
		if err == nil {
			compliancetest.Fail(t, "Provide() did not return no error when provided with an incorrect metadata type.")
		} else {
			compliancetest.Log(t, "Provide() correctly returned an error when provided with an metadata type (%v).", err)
		}
		var typedError *keyprovider.ErrInvalidMetadata
		if !errors.As(err, &typedError) {
			compliancetest.Fail(t, "Provide() returned an error of the type %T instead of %T. Please use the correct typed errors.", err, typedError)
		} else {
			compliancetest.Log(t, "Provide() correctly returned a %T when provided with an incorrect metadata type.", typedError)
		}
	})
	t.Run("round-trip", func(t *testing.T) {
		complianceTestRoundTrip(t, keyProviderConfig, cfg)
	})
}

func complianceTestRoundTrip[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider, TMeta keyprovider.KeyMeta](
	t *testing.T,
	keyProviderConfig TConfig,
	cfg TestConfiguration[TDescriptor, TConfig, TMeta, TKeyProvider],
) {
	keyProvider, inMeta := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, keyProviderConfig, true)
	output, outMeta, err := keyProvider.Provide(inMeta)
	if err != nil {
		compliancetest.Fail(t, "Provide() failed (%v).", err)
	} else {
		compliancetest.Log(t, "Provide() succeeded.")
	}
	if cfg.ProvideTestCase.ValidateMetadata != nil {
		if err := cfg.ProvideTestCase.ValidateMetadata(outMeta.(TMeta)); err != nil {
			compliancetest.Fail(t, "The metadata after the second Provide() call failed the test (%v).", err)
		}
	}

	// Create a second key provider to avoid internal state.
	keyProvider2, inMeta2 := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, keyProviderConfig, true)

	marshalledMeta, err := json.Marshal(outMeta)
	if err != nil {
		compliancetest.Fail(t, "JSON-marshalling output meta failed (%v).", err)
	} else {
		compliancetest.Log(t, "JSON-marshalling output meta succeeded: %s", marshalledMeta)
	}

	if err := json.Unmarshal(marshalledMeta, &inMeta2); err != nil {
		compliancetest.Fail(t, "JSON-unmarshalling meta failed (%v).", err)
	} else {
		compliancetest.Log(t, "JSON-unmarshalling meta succeeded.")
	}

	output2, outMeta2, err := keyProvider2.Provide(inMeta2)
	if err != nil {
		compliancetest.Fail(t, "Provide() on the subsequent run failed (%v).", err)
	} else {
		compliancetest.Log(t, "Provide() on the subsequent run succeeded.")
	}

	if cfg.ProvideTestCase.ExpectedOutput != nil {
		if !bytes.Equal(cfg.ProvideTestCase.ExpectedOutput.EncryptionKey, output.EncryptionKey) {
			compliancetest.Fail(t, "Incorrect encryption key received after the first Provide() call. Please set a break point to the line of this error message to debug this error.")
		}
		if !bytes.Equal(cfg.ProvideTestCase.ExpectedOutput.DecryptionKey, output2.DecryptionKey) {
			compliancetest.Fail(t, "Incorrect decryption key received after the second Provide() call. Please set a break point to the line of this error message to debug this error.")
		}
		if !bytes.Equal(cfg.ProvideTestCase.ExpectedOutput.EncryptionKey, output2.EncryptionKey) {
			compliancetest.Fail(t, "Incorrect encryption key received after the second Provide() call. Please set a break point to the line of this error message to debug this error.")
		}
	}
	if cfg.ProvideTestCase.ValidateMetadata != nil {
		if err := cfg.ProvideTestCase.ValidateMetadata(outMeta2.(TMeta)); err != nil {
			compliancetest.Fail(t, "The metadata after the second Provide() call failed the test (%v).", err)
		}
	}
	if cfg.ProvideTestCase.ValidateKeys == nil {
		if !bytes.Equal(output2.DecryptionKey, output.EncryptionKey) {
			compliancetest.Fail(
				t,
				"The encryption key from the first call to Provide() does not match the decryption key provided by the second Provide() call. If you intend the two keys to be different, please provide an ProvideTestCase.ValidateKeys function. If this is not intended, please set a break point to the line of this error message.",
			)
		} else {
			compliancetest.Log(
				t,
				"The encryption and decryption keys match.",
			)
		}
	} else {
		if err := cfg.ProvideTestCase.ValidateKeys(output2.DecryptionKey, output.EncryptionKey); err != nil {
			compliancetest.Fail(
				t,
				"The encryption key from the first call to Provide() does not match the decryption key provided by the second Provide() call (%v),",
				err,
			)
		} else {
			compliancetest.Log(
				t,
				"The encryption and decryption keys match.",
			)
		}
	}
}

func complianceTestID[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TMeta keyprovider.KeyMeta, TKeyProvider keyprovider.KeyProvider](
	t *testing.T,
	config TestConfiguration[TDescriptor, TConfig, TMeta, TKeyProvider],
) {
	id := config.Descriptor.ID()
	if id == "" {
		compliancetest.Fail(t, "ID is empty.")
	} else {
		compliancetest.Log(t, "ID is not empty.")
	}
	if err := id.Validate(); err != nil {
		compliancetest.Fail(t, "ID failed validation: %s", id)
	} else {
		compliancetest.Log(t, "ID passed validation.")
	}
}

func complianceTestHCLParsingTestCase[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TMeta keyprovider.KeyMeta, TKeyProvider keyprovider.KeyProvider](
	t *testing.T,
	tc HCLParseTestCase[TConfig, TKeyProvider],
	cfg TestConfiguration[TDescriptor, TConfig, TMeta, TKeyProvider],
) {
	parseError := false
	parsedConfig, diags := config.LoadConfigFromString("config.hcl", tc.HCL)
	if tc.ValidHCL {
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

	configStruct := cfg.Descriptor.ConfigStruct()
	diags = gohcl.DecodeBody(
		parsedConfig.KeyProviderConfigs[0].Body,
		nil,
		configStruct,
	)
	var keyProvider TKeyProvider
	if tc.ValidHCL {
		if diags.HasErrors() {
			compliancetest.Fail(t, "Failed to parse empty HCL block into config struct (%v).", diags)
		} else {
			compliancetest.Log(t, "HCL successfully loaded into config struct.")
		}

		keyProvider, _ = complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, configStruct, tc.ValidBuild)
	} else {
		if !parseError && !diags.HasErrors() {
			compliancetest.Fail(t, "Expected error during HCL parsing, but no error was returned.")
		} else {
			compliancetest.Log(t, "HCL loading errored correctly (%v).", diags)
		}
	}

	if tc.Validate != nil {
		if err := tc.Validate(configStruct.(TConfig), keyProvider); err != nil {
			compliancetest.Fail(t, "Error during validation and configuration (%v).", err)
		} else {
			compliancetest.Log(t, "Successfully validated parsed HCL config and applied modifications.")
		}
	} else {
		compliancetest.Log(t, "No ValidateAndConfigure provided, skipping HCL parse validation.")
	}
}

func complianceTestConfigCase[TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider, TMeta keyprovider.KeyMeta](
	t *testing.T,
	tc ConfigStructTestCase[TConfig, TKeyProvider],
) {
	keyProvider, _ := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, tc.Config, tc.ValidBuild)
	if tc.Validate != nil {
		if err := tc.Validate(keyProvider); err != nil {
			compliancetest.Fail(t, "Error during validation and configuration (%v).", err)
		} else {
			compliancetest.Log(t, "Successfully validated parsed HCL config and applied modifications.")
		}
	} else {
		compliancetest.Log(t, "No ValidateAndConfigure provided, skipping HCL parse validation.")
	}
}

func complianceTestBuildConfigAndValidate[TKeyProvider keyprovider.KeyProvider, TMeta keyprovider.KeyMeta](
	t *testing.T,
	configStruct keyprovider.Config,
	validBuild bool,
) (TKeyProvider, TMeta) {
	if configStruct == nil {
		compliancetest.Fail(t, "Nil struct passed!")
	}

	var typedKeyProvider TKeyProvider
	var typedMeta TMeta
	var ok bool
	kp, meta, err := configStruct.Build()
	if validBuild {
		if err != nil {
			compliancetest.Fail(t, "Build() returned an unexpected error: %v.", err)
		} else {
			compliancetest.Log(t, "Build() did not return an error.")
		}
		typedKeyProvider, ok = kp.(TKeyProvider)
		if !ok {
			compliancetest.Fail(t, "Build() returned an invalid key provider type of %T, expected %T", kp, typedKeyProvider)
		} else {
			compliancetest.Log(t, "Build() returned the correct key provider type of %T.", typedKeyProvider)
		}

		metaType := reflect.TypeOf(typedMeta)
		if meta == nil {
			if metaType.Kind() != reflect.Interface {
				compliancetest.Fail(t, "Build() did not return a metadata, but you declared a metadata type. Please make sure that you always return the same metadata type.")
			} else {
				compliancetest.Log(t, "Build() did not return a metadata and the declared metadata type is interface{}.")
			}
		} else {
			if metaType.Kind() == reflect.Interface {
				compliancetest.Fail(t, "Build() returned metadata, but you declared an interface type as the metadata type. Please always declare a pointer to a struct as a metadata type.")
			} else {
				compliancetest.Log(t, "Build() returned metadata and the declared metadata type is not an interface.")
			}
			typedMeta, ok = meta.(TMeta)
			if !ok {
				compliancetest.Fail(t, "Build() returned an invalid metadata type of %T, expected %T", meta, typedMeta)
			} else {
				compliancetest.Log(t, "Build() returned the correct metadata type of %T.", meta)
			}
		}
	} else {
		if err == nil {
			compliancetest.Fail(t, "Build() did not return an error.")
		} else {
			compliancetest.Log(t, "Build() correctly returned an error: %v", err)
		}

		var typedError *keyprovider.ErrInvalidConfiguration
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
	return typedKeyProvider, typedMeta
}

func complianceTestMetadataTestCase[TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider, TMeta keyprovider.KeyMeta](
	t *testing.T,
	tc MetadataStructTestCase[TConfig, TMeta],
) {
	keyProvider, _ := complianceTestBuildConfigAndValidate[TKeyProvider, TMeta](t, tc.ValidConfig, true)

	output, _, err := keyProvider.Provide(tc.Meta)
	if tc.IsPresent {
		// This test case means that the input metadata should be considered present, so it's either an error or a
		// decryption key.
		if tc.IsValid {
			if err != nil {
				var typedError *keyprovider.ErrKeyProviderFailure
				if !errors.As(err, &typedError) {
					compliancetest.Fail(
						t,
						"The Provide() function returned an unexpected error, which was also of the incorrect type of %T instead of %T: %v",
						err,
						typedError,
						err,
					)
				}
				compliancetest.Fail(t, "The Provide() function returned an unexpected error: %v", err)
			}
		} else {
			if err == nil {
				compliancetest.Fail(t, "The Provide() function did not return an error as expected.")
			} else {
				compliancetest.Log(t, "The Provide() function returned an expected error: %v", err)
			}

			var typedError *keyprovider.ErrInvalidMetadata
			if !errors.As(err, &typedError) {
				compliancetest.Fail(
					t,
					"The Provide() function returned the error type of %T instead of %T. Please use the correct typed errors.",
					err,
					typedError,
				)
			}
		}
	} else {
		if err != nil {
			var typedError *keyprovider.ErrKeyProviderFailure
			if !errors.As(err, &typedError) {
				compliancetest.Fail(
					t,
					"The Provide() function returned an unexpected error, which was also of the incorrect type of %T instead of %T: %v",
					err,
					typedError,
					err,
				)
			}
			compliancetest.Fail(t, "The Provide() function returned an unexpected error: %v", err)
		}
		if len(output.DecryptionKey) != 0 {
			compliancetest.Fail(
				t,
				"The Provide() function returned a decryption key despite not receiving input meta. This is incorrect, please don't return a decryption key unless you receive the input metadata.",
			)
		} else {
			compliancetest.Log(
				t,
				"The Provide() function correctly did not return a decryption key without input metadata.",
			)
		}
	}
}
