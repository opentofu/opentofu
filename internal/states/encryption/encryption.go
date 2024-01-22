package encryption

import (
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"sync"
)

// New creates a new Encryption object. You can use this object to feed in encryption configuration and then create
// an encryptionflow.StateFlow or encryptionflow.PlanFlow as needed.
//
// Note: due to a lot of procedural code you probably want to use GetSingleton() to obtain the Encryption implementation
// that's globally scoped.
func New(logger hclog.Logger) Encryption {
	return &encryption{
		encryptionConfigs:         make(encryptionconfig.ConfigMap),
		decryptionFallbackConfigs: make(encryptionconfig.ConfigMap),
		mutex:                     &sync.Mutex{},
		logger:                    logger,
	}
}

// Encryption is the main interface to feed the encryption configuration and obtain the encryptionflow.Flow for running
// the actual encryption.
//
// Note: due to a lot of procedural code you probably want to use GetSingleton() to obtain the Encryption implementation
// that's globally scoped.
type Encryption interface {
	// ApplyEnvConfigurations applies a configuration coming from the operating system environment. The
	// encryptionconfig.Config structures embedded in the parameters should have the source set to Environment in order
	// for the priorities to be applies correctly.
	ApplyEnvConfigurations(
		encryption map[encryptionconfig.Key]encryptionconfig.Config,
		decryptionFallback map[encryptionconfig.Key]encryptionconfig.Config,
	) error

	// ApplyHCLEncryptionConfiguration applies a single encryption configuration coming from HCL. The config should
	// have its source set to HCL.
	ApplyHCLEncryptionConfiguration(key encryptionconfig.Key, config encryptionconfig.Config) error
	// ApplyHCLDecryptionFallbackConfiguration applies a single decryption fallback configuration coming from HCL.
	// The config should have its source set to HCL.
	ApplyHCLDecryptionFallbackConfiguration(key encryptionconfig.Key, config encryptionconfig.Config) error

	// Validate ensures that the previously-applied configuration is valid.
	Validate() tfdiags.Diagnostics

	// RemoteState returns an encryption flow suitable for the remote state of the current project.
	//
	// When implementing this interface:
	//
	// - If the user provided no configuration, this function must return a flow that passes through the data
	//   unmodified.
	// - If the user only provided an environment configuration with the key encryptionconfig.KeyDefaultRemote, the
	//   returned flow should use this configuration.
	// - If the user provided a non-default HCL or environment configuration, these configurations should be merged
	//   with the environment taking precedence. The default configuration should be ignored.
	//
	// Please note, the encryption and decryption fallback configuration may have separate configuration. This method
	// should support this scenario to allow for encryption rollover.
	//
	// Tip: encryptionconfig.ConfigMap.Merge implements these precedence rules.
	RemoteState() (encryptionflow.StateFlow, error)

	// RemoteStateDatasource returns an encryption flow suitable for the remote state of a remote state data source.
	// You should pass the remote state data source name as follows:
	//
	//    encryptionconfig.Key("terraform_remote_state.foo")
	//
	// For indexed resources, please pass the index as follows:
	//
	//    encryptionconfig.Key("terraform_remote_state.foo[42]")
	//    encryptionconfig.Key("terraform_remote_state.foo[test]")
	//
	// See encryptionconfig.Key for more details on the key format.
	//
	// When implementing this interface:
	//
	// - If the user provided no configuration, this function must return a flow that passes through the data
	//   unmodified.
	// - If the user only provided an environment configuration with the key encryptionconfig.KeyDefaultRemote, the
	//   returned flow should use this configuration.
	// - If the user provided a non-default HCL or environment configuration, these configurations should be merged
	//   with the environment taking precedence. The default configuration should be ignored.
	//
	// Please note, the encryption and decryption fallback configuration may have separate configuration. This method
	// should support this scenario to allow for encryption rollover.
	//
	// Tip: encryptionconfig.ConfigMap.Merge implements these precedence rules.
	RemoteStateDatasource(configKey encryptionconfig.Key) (encryptionflow.StateFlow, error)

	// StateFile returns an encryption flow suitable for encrypting the state file.
	//
	// When implementing this interface:
	//
	// - If the user provided no configuration, this function must return a flow that passes through the data
	//   unmodified.
	// - The default configuration is always ignored in this case because it is only the default for remote states.
	// - If the user provided a non-default HCL or environment configuration, these configurations should be merged
	//   with the environment taking precedence. The default configuration should be ignored.
	//
	// Please note, the encryption and decryption fallback configuration may have separate configuration. This method
	// should support this scenario to allow for encryption rollover.
	//
	// Tip: encryptionconfig.ConfigMap.Merge implements these precedence rules.
	StateFile() (encryptionflow.StateFlow, error)

	// PlanFile returns an encryption flow suitable for encrypting the plan file.
	//
	// When implementing this interface:
	//
	// - If the user provided no configuration, this function must return a flow that passes through the data
	//   unmodified.
	// - The default configuration is always ignored in this case because it is only the default for remote states.
	// - If the user provided a non-default HCL or environment configuration, these configurations should be merged
	//   with the environment taking precedence. The default configuration should be ignored.
	//
	// Tip: encryptionconfig.ConfigMap.Merge implements these precedence rules.
	PlanFile() (encryptionflow.PlanFlow, error)
}

type encryption struct {
	encryptionConfigs         encryptionconfig.ConfigMap
	decryptionFallbackConfigs encryptionconfig.ConfigMap
	mutex                     *sync.Mutex
	logger                    hclog.Logger
}

func (e *encryption) ApplyEnvConfigurations(
	encryption map[encryptionconfig.Key]encryptionconfig.Config,
	decryptionFallback map[encryptionconfig.Key]encryptionconfig.Config,
) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for key, config := range encryption {
		if err := key.Validate(); err != nil {
			return fmt.Errorf("failed to parse encryption configuration from environment (%w)", err)
		}
		meta := encryptionconfig.Meta{
			encryptionconfig.SourceEnv,
			key,
		}
		e.encryptionConfigs[meta] = config
	}

	for key, config := range decryptionFallback {
		if err := key.Validate(); err != nil {
			return fmt.Errorf("failed to parse decryption fallback configuration from environment (%w)", err)
		}
		e.decryptionFallbackConfigs[encryptionconfig.Meta{
			encryptionconfig.SourceEnv,
			key,
		}] = config
	}

	return nil
}

func (e *encryption) ApplyHCLEncryptionConfiguration(key encryptionconfig.Key, config encryptionconfig.Config) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if err := key.Validate(); err != nil {
		return fmt.Errorf("failed to parse encryption configuration from HCL (%w)", err)
	}
	meta := encryptionconfig.Meta{
		encryptionconfig.SourceHCL,
		key,
	}
	e.encryptionConfigs[meta] = config
	return nil
}

func (e *encryption) ApplyHCLDecryptionFallbackConfiguration(key encryptionconfig.Key, config encryptionconfig.Config) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if err := key.Validate(); err != nil {
		return fmt.Errorf("failed to parse decryption fallback configuration from HCL (%w)", err)
	}
	meta := encryptionconfig.Meta{
		encryptionconfig.SourceHCL,
		key,
	}
	e.decryptionFallbackConfigs[meta] = config
	return nil
}

func (e *encryption) Validate() (diags tfdiags.Diagnostics) {
	keys := e.allConfigKeys()

	for key, _ := range keys {
		if _, err := e.build(key); err != nil {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Invalid state encryption configuration for configuration key %s", key),
				err.Error(),
			))
		}
	}

	return diags
}

func (e *encryption) allConfigKeys() map[encryptionconfig.Key]struct{} {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Collect all the keys between the encryption and decryption fallback configuration. We do this to make sure
	// we don't process keys twice, which would result in duplicate errors. We also make sure that the
	// default key is tested as a backend key since that's where it will actually be used.
	keys := map[encryptionconfig.Key]struct{}{}
	addKey := func(key encryptionconfig.Key) {
		if key == encryptionconfig.KeyDefaultRemote {
			// The presence of the default key means that there should be a valid backend configuration as the
			// default key is automatically applied to it.
			keys[encryptionconfig.KeyBackend] = struct{}{}
		} else {
			keys[key] = struct{}{}
		}
	}
	for meta, _ := range e.encryptionConfigs {
		addKey(meta.Key)
	}
	for meta, _ := range e.decryptionFallbackConfigs {
		addKey(meta.Key)
	}

	return keys
}

func (e *encryption) RemoteState() (encryptionflow.StateFlow, error) {
	return e.build(encryptionconfig.KeyBackend)
}

func (e *encryption) StateFile() (encryptionflow.StateFlow, error) {
	return e.build(encryptionconfig.KeyStateFile)
}

func (e *encryption) PlanFile() (encryptionflow.PlanFlow, error) {
	return e.build(encryptionconfig.KeyPlanFile)
}

func (e *encryption) RemoteStateDatasource(configKey encryptionconfig.Key) (encryptionflow.StateFlow, error) {
	if !configKey.IsRemoteDataSource() {
		return nil, fmt.Errorf("the specified configuration key is not a valid remote data source key (this is likely a bug, did you want to call RemoteState(), StateFile(), or PlanFile()?)")
	}
	return e.build(configKey)
}

func (e *encryption) build(key encryptionconfig.Key) (encryptionflow.Flow, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	mergedEncryptionConfig, err := e.encryptionConfigs.Merge(key)
	if err != nil {
		return nil, fmt.Errorf("failed to merge encryption configuration (%w)", err)
	}

	mergedDecryptionFallbackConfig, err := e.decryptionFallbackConfigs.Merge(key)
	if err != nil {
		return nil, fmt.Errorf("failed to merge fallback configuration (%w)", err)
	}

	return encryptionflow.New(key, mergedEncryptionConfig, mergedDecryptionFallbackConfig, e.logger), nil
}
