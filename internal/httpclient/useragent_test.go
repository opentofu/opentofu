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

func TestCustomUserAgentEnvVarIgnored(t *testing.T) {
	const removedCustomUAEnvVar = "OPENTOFU_USER_AGENT"
	expectedBase := "OpenTofu/0.0.0"

	testCases := []struct {
		customUaValue string
		appendUaValue string
		expected      string
	}{
		{"custom/agent", "", expectedBase},
		{"custom/agent", " ", expectedBase},
		{"custom/agent", " \n", expectedBase},
		{"custom/agent", "test/1", expectedBase + " test/1"},
		{"override/all", "extra/info", expectedBase + " extra/info"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Setenv(removedCustomUAEnvVar, tc.customUaValue)
			t.Setenv(appendUaEnvVar, tc.appendUaValue)
			actual := OpenTofuUserAgent("0.0.0")
			if actual != tc.expected {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", tc.expected, actual)
			}
		})
	}
}
