package encryption

/*
type tstAlwaysFailingFlowBuilder struct{}

var alwaysFailError = errors.New("always fails")

func (t *tstAlwaysFailingFlowBuilder) EncryptionConfiguration(_ encryptionconfig.Config) error {
	return alwaysFailError
}

func (t *tstAlwaysFailingFlowBuilder) DecryptionFallbackConfiguration(_ encryptionconfig.Config) error {
	return alwaysFailError
}

func (t *tstAlwaysFailingFlowBuilder) Build() (encryptionflow.Flow, error) {
	return nil, alwaysFailError
}

func TestApplyEncryptionConfigIfExists_ApplyError(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.apply_encryption_config_if_exists")
	t.Setenv(encryptionconfig.ConfigEnvName, envConfig(configKey, true))

	failFlow := &tstAlwaysFailingFlowBuilder{}
	err := applyEncryptionConfigIfExists(failFlow, configKey)
	expectErr(t, err, alwaysFailError)
}

func TestApplyDecryptionFallbackConfigIfExists_ApplyError(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.apply_decryption_fallback_config_if_exists")
	t.Setenv(encryptionconfig.FallbackConfigEnvName, envConfig(configKey, true))

	failFlow := &tstAlwaysFailingFlowBuilder{}
	err := applyDecryptionFallbackConfigIfExists(failFlow, configKey)
	expectErr(t, err, alwaysFailError)
}
*/
