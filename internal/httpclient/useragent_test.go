// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"fmt"
	"os"
	"testing"

	"github.com/opentofu/opentofu/version"
)

func TestUserAgentString_env(t *testing.T) {

	appendUaVal := os.Getenv(appendUaEnvVar)
	os.Unsetenv(appendUaEnvVar)
	defer os.Setenv(appendUaEnvVar, appendUaVal)

	expectedBase := fmt.Sprintf("%s/%s", DefaultApplicationName, version.Version)

	for i, c := range []struct {
		expected   string
		additional string
	}{
		{expectedBase, ""},
		{expectedBase, " "},
		{expectedBase, " \n"},

		{fmt.Sprintf("%s test/1", expectedBase), "test/1"},
		{fmt.Sprintf("%s test/2", expectedBase), "test/2 "},
		{fmt.Sprintf("%s test/3", expectedBase), " test/3 "},
		{fmt.Sprintf("%s test/4", expectedBase), "test/4 \n"},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			if c.additional != "" {
				t.Setenv(appendUaEnvVar, c.additional)
			}

			actual := OpenTofuUserAgent(version.Version)

			if c.expected != actual {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", c.expected, actual)
			}
		})
	}
}

func TestUserAgentAppendViaEnvVar(t *testing.T) {
	expectedBase := "OpenTofu/0.0.0"

	testCases := []struct {
		envVarValue string
		expected    string
	}{
		{"", expectedBase},
		{" ", expectedBase},
		{" \n", expectedBase},
		{"test/1", expectedBase + " test/1"},
		{"test/1 (comment)", expectedBase + " test/1 (comment)"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Setenv(appendUaEnvVar, tc.envVarValue)
			givenUA := OpenTofuUserAgent("0.0.0")
			if givenUA != tc.expected {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", tc.expected, givenUA)
			}
		})
	}
}
func TestCustomUserAgentViaEnvVarIgnored(t *testing.T) {
	// removedCustomUAEnvVar was an undocumented variable that was removed.
	// Verify that setting it has no effect on the User-Agent string.
	const removedCustomUAEnvVar = "OPENTOFU_USER_AGENT"
	t.Setenv(removedCustomUAEnvVar, "custom/agent")
	t.Setenv(appendUaEnvVar, "")
	expected := fmt.Sprintf("%s/%s", DefaultApplicationName, "0.0.0")
	actual := OpenTofuUserAgent("0.0.0")
	if actual != expected {
		t.Fatalf("Expected User-Agent '%s' does not match '%s'", expected, actual)
	}
}
