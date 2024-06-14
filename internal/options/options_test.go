package options

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetGlobalOptions(t *testing.T) {
	testCases := []struct {
		name     string
		args      []string
		expected map[string]string
	}{
		{"xx", []string{"-chdir=target", "plan", "-state=file.tfstate"}, map[string]string{"chdir": "target"}},
		{"xx", []string{"-version", "plan", "-state=file.tfstate"}, map[string]string{"version": ""}},
		{"xx", []string{"--version", "plan", "-state=file.tfstate"}, map[string]string{"version": ""}},
		{"xx", []string{"-random", "plan", "-state=file.tfstate"}, map[string]string{"random": ""}},
		{"xx", []string{"plan", "-state=file.tfstate"}, map[string]string{}},
	}

	var opts map[string]string
	var err error

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err = GetGlobalOptions(tc.args)
			assert.EqualValues(t, tc.expected, opts)
			assert.Nil(t, err)
		})
	}

	opts, err = GetGlobalOptions([]string{"-chdir", "plan", "-state=file.tfstate"})
	assert.Nil(t, opts)
	assert.Error(t, err)
}


func TestIsGlobalOptionSet(t *testing.T) {
	t.Skip()
}
