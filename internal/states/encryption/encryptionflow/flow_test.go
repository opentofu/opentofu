package encryptionflow

import (
	"testing"

	"github.com/hashicorp/go-hclog"
)

// tstNoConfigurationInstance constructs a Flow with no configuration.
//
// This is the most important case, because this will happen if tofu is run without
// any encryption configuration. We need to ensure all state and plans are passed through
// unchanged.
func tstNoConfigurationInstance() Flow {
	return New(
		"testing_no_configuration",
		nil,
		nil,
		hclog.NewNullLogger(),
	)
}

func tstPassthrough(t *testing.T, value string, method func([]byte) ([]byte, error)) {
	actual, err := method([]byte(value))
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if string(actual) != value {
		t.Error("failed to pass through")
	}
}

func TestDecryptState_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `{"version":"4"}`, cut.DecryptState)
}

func TestEncryptState_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `{"version":"4"}`, cut.EncryptState)
}

func TestDecryptPlan_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `zip64`, cut.DecryptPlan)
}

func TestEncryptPlan_Passthrough(t *testing.T) {
	cut := tstNoConfigurationInstance()
	tstPassthrough(t, `zip64`, cut.EncryptPlan)
}
