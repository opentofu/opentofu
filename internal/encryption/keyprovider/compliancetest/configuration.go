// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
)

type TestConfiguration[TDescriptor keyprovider.Descriptor, TConfig keyprovider.Config, TMeta any, TKeyProvider keyprovider.KeyProvider] struct {
	// Descriptor is the descriptor for the key provider.
	Descriptor TDescriptor

	// HCLParseTestCases contains the test cases of parsing HCL configuration and then validating it using the Build()
	// function.
	HCLParseTestCases map[string]HCLParseTestCase[TConfig, TKeyProvider]

	// ConfigStructT validates that a certain config results or does not result in a valid Build() call.
	ConfigStructTestCases map[string]ConfigStructTestCase[TConfig, TKeyProvider]

	// MetadataStructTestCases test various metadata values for correct handling.
	MetadataStructTestCases map[string]MetadataStructTestCase[TConfig, TMeta]

	// ProvideTestCase exercises the entire chain and generates two keys.
	ProvideTestCase ProvideTestCase[TConfig, TMeta]
}

// HCLParseTestCase contains a test case that parses HCL into a configuration.
type HCLParseTestCase[TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider] struct {
	// HCL contains the code that should be parsed into the configuration structure.
	HCL string
	// ValidHCL indicates that the HCL block should be parsable into the configuration structure, but not necessarily
	// result in a valid Build() call.
	ValidHCL bool
	// ValidBuild indicates that calling the Build() function should not result in an error.
	ValidBuild bool
	// Validate is an extra optional validation function that can check if the configuration contains the correct
	// values parsed from HCL. If ValidBuild is true, the key provider will be passed as well.
	Validate func(config TConfig, keyProvider TKeyProvider) error
}

// ConfigStructTestCase validates that the config struct is behaving correctly when Build() is called.
type ConfigStructTestCase[TConfig keyprovider.Config, TKeyProvider keyprovider.KeyProvider] struct {
	Config     TConfig
	ValidBuild bool
	Validate   func(keyProvider TKeyProvider) error
}

// MetadataStructTestCase is a test case for metadata.
type MetadataStructTestCase[TConfig keyprovider.Config, TMeta any] struct {
	// Config contains a valid configuration that should be used to construct the key provider.
	ValidConfig TConfig
	// Meta contains the metadata for this test case.
	Meta TMeta
	// IsPresent indicates that the supplied metadata in Meta should be treated as present and the Provide() function
	// should either return an error or a decryption key. If IsPresent is false, the Provide() function must not
	// return an error or a decryption key.
	IsPresent bool
	// IsValid indicates that, if IsPresent is true, the metadata should be valid and the Provide() function should not
	// exit with a *keyprovider.ErrInvalidMetadata error.
	IsValid bool
}

// ProvideTestCase provides a test configuration Provide() test where a key is requested and then
// subsequently compared.
type ProvideTestCase[TConfig keyprovider.Config, TMeta any] struct {
	// ValidConfig is a valid configuration that the integration test can use to generate keys.
	ValidConfig TConfig
	// ExpectedOutput indicates what keys are expected as an output when the integration test is ran with full metadata.
	ExpectedOutput *keyprovider.Output
	// ValidateKeys is a function that compares an encryption and a decryption key. The function should return an error
	// if the two keys don't belong together. If you do not provide this function, bytes.Equal will be used.
	ValidateKeys func(decryptionKey []byte, encryptionKey []byte) error
	// ValidateMetadata is a function that validates that the resulting metadata is correct.
	ValidateMetadata func(meta TMeta) error
}
