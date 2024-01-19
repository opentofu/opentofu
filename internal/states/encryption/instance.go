package encryption

import (
	"strings"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

// --------------------------------------------------------------------------------------------
// IMPORTANT NOTE:
// This package contains a cache for singleton instances of encryptionflow.Builder
//   - there is one instance for our own remote state
//   - there is one instance for our local state file
//   - there is one instance for our local plan file
//   - there is one instance for each remote state data source
//
// One way to configure state/plan encryption is adding settings to the terraform {} block
// (or the terraform_remote_state block, respectively). These blocks are parsed by code far removed
// from where they are used, and they are parsed multiple times by OpenTofu.
//
// Combine that with procedural code in internal/states/statefile (local state file) and
// internal/plans (local plan file), which is called from all over the place.
//
// This is why caching singleton instances in this package is the less painful option.
// --------------------------------------------------------------------------------------------
// How to write tests?
//
// Solution 1: Suitable for more integrative tests
//
//   EnableSingletonCaching()
//   defer DisableSingletonCaching()
//
// Solution 2: Suitable for lower level tests
//
// Obtain your singleton once during the test, and configure it directly using its methods such as
//
//   singleton.EncryptionConfiguration(...)
//
// --------------------------------------------------------------------------------------------

// GetSingleton obtains the singleton instance of the encryption flow builder for the given configKey.
//
// configKey specifies a resource that accesses remote state. It must contain at least one ".".
//
// If the key is "terraform_remote_state.foo", the returned Flow is intended for
//
//	data "terraform_remote_state" "foo" {}
//
// For enumerated resources, the format is "terraform_remote_state.foo[17]" or
// "terraform_remote_state.foo[key]" (no quotes around the for_each key).
//
// The first time a particular instance is requested, GetSingleton may bail out due to invalid configuration.
//
// See also GetRemoteStateSingleton(), GetStatefileSingleton(), GetPlanfileSingleton().
func GetSingleton(configKey encryptionconfig.Key) (encryptionflow.Builder, error) {
	if !strings.Contains(string(configKey), ".") {
		panic("call to encryption.GetSingleton with a key that does not contain '.'. This is a bug. " +
			"GetSingleton() is intended to obtain named instances only. For predefined instances use " +
			"GetRemoteStateSingleton(), GetStatefileSingleton(), or GetPlanfileSingleton()")
	}
	if cache != nil {
		return cache.cachedOrNewInstance(configKey, true)
	} else {
		return newInstance(configKey, true)
	}
}

// GetRemoteStateSingleton obtains the singleton instance of the encryption flow builder that is intended for our own remote
// state backend, as opposed to terraform_remote_state data sources.
func GetRemoteStateSingleton() (encryptionflow.Builder, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.KeyBackend, true)
	} else {
		return newInstance(encryptionconfig.KeyBackend, true)
	}
}

// GetStatefileSingleton obtains the singleton instance of the encryption flow builder that is intended for our own local state file.
func GetStatefileSingleton() (encryptionflow.Builder, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.KeyStatefile, false)
	} else {
		return newInstance(encryptionconfig.KeyStatefile, false)
	}
}

// GetPlanfileSingleton obtains the instance of the encryption flow builder that is intended for our plan file.
func GetPlanfileSingleton() (encryptionflow.Builder, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.KeyPlanfile, false)
	} else {
		return newInstance(encryptionconfig.KeyPlanfile, false)
	}
}

func newInstance(configKey encryptionconfig.Key, defaultsApply bool) (encryptionflow.Builder, error) {
	logging.HCLogger().Trace("constructing new state encryption flow instance", "configKey", configKey)
	instance := encryptionflow.NewBuilder(configKey)

	if defaultsApply {
		if err := applyEncryptionConfigIfExists(instance, encryptionconfig.KeyDefault); err != nil {
			return nil, err
		}
		if err := applyDecryptionFallbackConfigIfExists(instance, encryptionconfig.KeyDefault); err != nil {
			return nil, err
		}
	}

	if err := applyEncryptionConfigIfExists(instance, configKey); err != nil {
		return nil, err
	}
	if err := applyDecryptionFallbackConfigIfExists(instance, configKey); err != nil {
		return nil, err
	}

	return instance, nil
}
