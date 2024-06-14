package options

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetGlobalOptions(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{"positive tc chdir", []string{"-chdir=target", "plan", "-state=file.tfstate"}, map[string]string{"chdir": "target"}},
		{"positive tc 1 version", []string{"-version", "plan", "-state=file.tfstate"}, map[string]string{"version": ""}},
		{"positive tc 2 version", []string{"--version", "plan", "-state=file.tfstate"}, map[string]string{"version": ""}},
		{"positive tc random", []string{"-random", "plan", "-state=file.tfstate"}, map[string]string{"random": ""}},
		{"positive tc multi", []string{"-help", "-version", "plan", "-state=file.tfstate"}, map[string]string{"help": "", "version": ""}},
		{"positive tc omitted", []string{"plan", "-state=file.tfstate"}, map[string]string{}},
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

	t.Run("negative tc chdir", func(t *testing.T) {
		opts, err = GetGlobalOptions([]string{"-chdir", "plan", "-state=file.tfstate"})
		assert.Nil(t, opts)
		assert.Error(t, err)
	})
}

func TestIsGlobalOptionSet(t *testing.T) {
	testCases := []struct {
		name     string
		find     string
		args     []string
		expected bool
	}{
		{"positive tc found", "chdir", []string{"-chdir=target", "plan", "-state=file.tfstate"}, true},
		{"positive tc not found", "random", []string{"-chdir=target", "plan", "-state=file.tfstate"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.EqualValues(t, tc.expected, IsGlobalOptionSet(tc.find, tc.args))
		})
	}
}