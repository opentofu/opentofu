package encryption

/*
func tstCodeConfigurationInstance(encValid bool, decValid bool) Flow {
	configKey := encryptionconfig.Key("testing_code_configuration.foo")
	encConfig := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Config: map[string]string{},
		},
		Method: encryptionconfig.MethodConfig{},
	}
	if encValid {
		encConfig.KeyProvider.Config["passphrase"] = "a new passphrase"
	}

	decConfig := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Config: map[string]string{},
		},
		Method: encryptionconfig.MethodConfig{},
	}
	if decValid {
		decConfig.KeyProvider.Config["passphrase"] = "the old passphrase"
	}

	return New(
		configKey,
		&encConfig,
		&decConfig,
		hclog.NewNullLogger(),
	)
}

// TODO move this to encryption
func TestMergeAndValidateConfigurations(t *testing.T) {
	testCases := []struct {
		testcase    string
		cut         Flow
		expectError error
	}{
		{
			testcase:    "no_configuration",
			cut:         tstNoConfigurationInstance(),
			expectError: nil,
		},
		{
			testcase:    "valid_configurations",
			cut:         tstCodeConfigurationInstance(true, true),
			expectError: nil,
		},
		{
			testcase:    "invalid_enc_config",
			cut:         tstCodeConfigurationInstance(false, true),
			expectError: errors.New("failed to merge encryption configuration (invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty)))"),
		},
		{
			testcase:    "invalid_dec_config",
			cut:         tstCodeConfigurationInstance(true, false),
			expectError: errors.New("failed to merge fallback configuration (invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty)))"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			_, err := tc.cut.Build()
			expectErr(t, err, tc.expectError)
		})
	}
}

func TestDecryptEncryptPropagateErrors(t *testing.T) {
	cut := tstCodeConfigurationInstance(false, true)
	expected := errors.New("failed to merge encryption configuration (invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty)))")

	_, err := cut.Build()
	expectErr(t, err, expected)
}

func TestEncryptionConfigurationEnforcesSource(t *testing.T) {
	cut := tstNoConfigurationInstance()

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.EncryptionConfiguration(
		encryptionconfig.Config{
			Meta: encryptionconfig.Meta{
				"invalid",
				encryptionconfig.KeyDefaultRemote,
			},
		},
	)
}

func TestDecryptionFallbackConfigurationEnforcesSource(t *testing.T) {
	cut := tstNoConfigurationInstance()

	defer tstExpectPanic(t, "called with invalid source value")()
	_ = cut.DecryptionFallbackConfiguration(
		encryptionconfig.Config{
			Meta: encryptionconfig.Meta{
				"invalid",
				encryptionconfig.KeyDefaultRemote,
			},
		},
	)
}

func tstExpectPanic(t *testing.T, snippet string) func() {
	return func() {
		r := recover()
		if r == nil {
			t.Errorf("expected a panic")
		} else {
			actual := fmt.Sprintf("%v", r)
			if !strings.Contains(actual, snippet) {
				t.Errorf("panic message did not contain '%s'", snippet)
			}
		}
	}
}
*/
