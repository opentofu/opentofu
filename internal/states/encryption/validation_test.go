package encryption

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestValidateAllCachedInstances_NoCache(t *testing.T) {
	if cache != nil {
		t.Fatal("cache was enabled at start of test - probably some other test forgot to defer DisableCache()")
	}

	diags := ValidateAllCachedInstances()
	if len(diags) != 1 {
		t.Fatal("did not see expected diags")
	}
	expectDiag(t, diags[0], tfdiags.Warning,
		"no encryption instance cache available, cannot validate configurations",
		"this warning may be an indication of a bug. ValidateAllCachedInstances() was called, but the cache is not enabled")
}

func TestValidateAllCachedInstances(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.validate_all_cached_instances")

	testCases := []struct {
		testcase        string
		key             encryptionconfig.Key
		encEnv          string
		expectDiagCount int
		expectSeverity  tfdiags.Severity
		expectSummary   string
		expectDetail    string
	}{
		// success case
		{
			testcase:        "full_configuration",
			key:             configKey,
			encEnv:          envConfig(configKey, true),
			expectDiagCount: 0,
		},
		// validation error case
		{
			testcase:        "logically_invalid_enc",
			key:             configKey,
			encEnv:          envConfig(configKey, false),
			expectDiagCount: 1,
			expectSeverity:  tfdiags.Error,
			expectSummary:   fmt.Sprintf("Invalid state encryption configuration for configuration key %s", configKey),
			expectDetail: "failed to merge encryption configuration " +
				"(invalid configuration after merge " +
				"(error in configuration for key provider passphrase (passphrase missing or empty)))",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			EnableSingletonCaching()
			defer DisableSingletonCaching()

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}

			_, err := GetSingleton(tc.key)
			if err != nil {
				t.Fatal("unexpected error during instance creation")
			}

			diags := ValidateAllCachedInstances()
			if len(diags) != tc.expectDiagCount {
				t.Fatal("unexpected diagnostics count")
			}
			if len(diags) > 0 {
				expectDiag(t, diags[0], tc.expectSeverity, tc.expectSummary, tc.expectDetail)
			}
		})
	}
}

func expectDiag(t *testing.T, actual tfdiags.Diagnostic, expectSeverity tfdiags.Severity, expectSummary string, expectDetail string) {
	t.Helper()
	if actual == nil {
		t.Error("unexpected nil diag")
	} else {
		if expectSeverity != actual.Severity() {
			t.Error("unexpected severity")
		}
		if expectSummary != actual.Description().Summary || expectDetail != actual.Description().Detail {
			t.Errorf("unexpected:\n%s\n%s\nexpected:\n%s\n%s",
				actual.Description().Summary, actual.Description().Detail,
				expectSummary, expectDetail)
		}
	}
}
