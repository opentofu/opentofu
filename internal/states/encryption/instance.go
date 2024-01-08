package encryption

import (
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
	"strings"
)

// Instance obtains the instance of the encryption flow for the given configKey.
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
// The first time a particular instance is requested, Instance may bail out due to invalid configuration.
//
// See also RemoteStateInstance(), StatefileInstance(), PlanfileInstance().
func Instance(configKey string) (encryptionflow.Flow, error) {
	if !strings.Contains(configKey, ".") {
		panic("call to encryption.Instance with a key that does not contain '.'. This is a bug. " +
			"Instance() is intended to obtain named instances only. For predefined instances use " +
			"RemoteStateInstance(), StatefileInstance(), or PlanfileInstance()")
	}
	if cache != nil {
		return cache.cachedOrNewInstance(configKey, true)
	} else {
		return newInstance(configKey, true)
	}
}

// RemoteStateInstance obtains the instance of the encryption flow that is intended for our own remote
// state backend, as opposed to terraform_remote_state data sources.
func RemoteStateInstance() (encryptionflow.Flow, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.ConfigKeyBackend, true)
	} else {
		return newInstance(encryptionconfig.ConfigKeyBackend, true)
	}
}

// StatefileInstance obtains the instance of the encryption flow that is intended for our own local state file.
func StatefileInstance() (encryptionflow.Flow, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.ConfigKeyStatefile, false)
	} else {
		return newInstance(encryptionconfig.ConfigKeyStatefile, false)
	}
}

// PlanfileInstance obtains the instance of the encryption flow that is intended for our plan file.
func PlanfileInstance() (encryptionflow.Flow, error) {
	if cache != nil {
		return cache.cachedOrNewInstance(encryptionconfig.ConfigKeyPlanfile, false)
	} else {
		return newInstance(encryptionconfig.ConfigKeyPlanfile, false)
	}
}

func newInstance(configKey string, defaultsApply bool) (encryptionflow.Flow, error) {
	logging.HCLogger().Trace("constructing new state encryption flow instance", "configKey", configKey)
	instance := encryptionflow.NewMock(configKey)

	if defaultsApply {
		if err := applyEncryptionConfigIfExists(instance, encryptionflow.ConfigurationSourceEnvDefault, encryptionconfig.ConfigKeyDefault); err != nil {
			return nil, err
		}
		if err := applyDecryptionFallbackConfigIfExists(instance, encryptionflow.ConfigurationSourceEnvDefault, encryptionconfig.ConfigKeyDefault); err != nil {
			return nil, err
		}
	}

	if err := applyEncryptionConfigIfExists(instance, encryptionflow.ConfigurationSourceEnv, configKey); err != nil {
		return nil, err
	}
	if err := applyDecryptionFallbackConfigIfExists(instance, encryptionflow.ConfigurationSourceEnv, configKey); err != nil {
		return nil, err
	}

	return instance, nil
}
