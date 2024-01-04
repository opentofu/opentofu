package encryption

import (
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/flow"
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
func Instance(configKey string) flow.Flow {
	if !strings.Contains(configKey, ".") {
		panic("call to encryption.Instance with a key that does not contain '.'. This is a bug. " +
			"Instance() is intended to obtain named instances only. For predefined instances use " +
			"RemoteStateInstance(), StatefileInstance(), or PlanfileInstance()")
	}
	if !environmentParsedSuccessfully {
		panic("call to Instance() before ParseEnvironmentVariables(). This is a bug.")
	}
	return cachedInstance(configKey, true)
}

// RemoteStateInstance obtains the instance of the encryption flow that is intended for our own remote
// state backend, as opposed to terraform_remote_state data sources.
func RemoteStateInstance() flow.Flow {
	if !environmentParsedSuccessfully {
		panic("call to RemoteStateInstance() before ParseEnvironmentVariables(). This is a bug.")
	}
	return cachedInstance(encryptionconfig.ConfigKeyBackend, true)
}

// StatefileInstance obtains the instance of the encryption flow that is intended for our own local state file.
func StatefileInstance() flow.Flow {
	if !environmentParsedSuccessfully {
		panic("call to StatefileInstance() before ParseEnvironmentVariables(). This is a bug.")
	}
	return cachedInstance(encryptionconfig.ConfigKeyStatefile, false)
}

// PlanfileInstance obtains the instance of the encryption flow that is intended for our plan file.
func PlanfileInstance() flow.Flow {
	if !environmentParsedSuccessfully {
		panic("call to PlanfileInstance() before ParseEnvironmentVariables(). This is a bug.")
	}
	return cachedInstance(encryptionconfig.ConfigKeyPlanfile, false)
}

var instanceCache = make(map[string]flow.Flow)

func cachedInstance(configKey string, defaultsApply bool) flow.Flow {
	instance, found := instanceCache[configKey]
	if found {
		return instance
	}

	return newInstance(configKey, defaultsApply)
}

func newInstance(configKey string, defaultsApply bool) flow.Flow {
	instance := flow.NewMock(configKey)

	if defaultsApply {
		_ = applyEncryptionConfigIfExists(instance, flow.ConfigurationSourceEnvDefault, encryptionconfig.ConfigKeyDefault)
		_ = applyDecryptionFallbackConfigIfExists(instance, flow.ConfigurationSourceEnvDefault, encryptionconfig.ConfigKeyDefault)
	}

	_ = applyEncryptionConfigIfExists(instance, flow.ConfigurationSourceEnv, configKey)
	_ = applyDecryptionFallbackConfigIfExists(instance, flow.ConfigurationSourceEnv, configKey)

	return instance
}
