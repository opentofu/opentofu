package encryption

import (
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/flow"
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
	// TODO validate "at least one .", bail out if wrong
	return instance(configKey, true)
}

// RemoteStateInstance obtains the instance of the encryption flow that is intended for our own remote
// state backend, as opposed to terraform_remote_state data sources.
func RemoteStateInstance() flow.Flow {
	return instance(encryptionconfig.ConfigKeyBackend, true)
}

// StatefileInstance obtains the instance of the encryption flow that is intended for our own local state file.
func StatefileInstance() flow.Flow {
	return instance(encryptionconfig.ConfigKeyStatefile, false)
}

// PlanfileInstance obtains the instance of the encryption flow that is intended for our plan file.
func PlanfileInstance() flow.Flow {
	return instance(encryptionconfig.ConfigKeyPlanfile, false)
}

func instance(configKey string, defaultsApply bool) flow.Flow {
	// TODO parse config in env when first called + cache result
	// TODO bail out and log a meaningful error for invalid configurations in env
	// TODO cache instances
	// TODO instances must remember their configurations
	// TODO instances must be provided with configurations (default if true+configKey) from Env when first created
	return flow.NewMock(configKey)
}
