// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"fmt"
	"os"
	"testing"

	"github.com/opentofu/opentofu/version"
)

func TestUserAgentString_env(t *testing.T) {
	expectedBase := fmt.Sprintf("%s/%s", DefaultApplicationName, version.Version)
	if oldenv, isSet := os.LookupEnv(appendUaEnvVar); isSet {
		defer os.Setenv(appendUaEnvVar, oldenv)
	} else {
		defer os.Unsetenv(appendUaEnvVar)
	}

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
			if c.additional == "" {
				os.Unsetenv(appendUaEnvVar)
			} else {
				os.Setenv(appendUaEnvVar, c.additional)
			}

			actual := OpenTofuUserAgent(version.Version)

			if c.expected != actual {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", c.expected, actual)
			}
		})
	}
}

func TestUserAgentAppendViaEnvVar(t *testing.T) {
	if oldenv, isSet := os.LookupEnv(appendUaEnvVar); isSet {
		defer os.Setenv(appendUaEnvVar, oldenv)
	} else {
		defer os.Unsetenv(appendUaEnvVar)
	}

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
			os.Unsetenv(appendUaEnvVar)
			os.Setenv(appendUaEnvVar, tc.envVarValue)
			givenUA := OpenTofuUserAgent("0.0.0")
			if givenUA != tc.expected {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", tc.expected, givenUA)
			}
		})
	}
}
func TestCustomUserAgentViaEnvVar(t *testing.T) {
	if oldenv, isSet := os.LookupEnv(customUaEnvVar); isSet {
		defer os.Setenv(customUaEnvVar, oldenv)
	} else {
		defer os.Unsetenv(customUaEnvVar)
	}

	testCases := []struct {
		envVarValue string
	}{
		{" "},
		{" \n"},
		{"test/1"},
		{"test/1 (comment)"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			os.Unsetenv(customUaEnvVar)
			os.Setenv(customUaEnvVar, tc.envVarValue)
			givenUA := OpenTofuUserAgent("0.0.0")
			if givenUA != tc.envVarValue {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", tc.envVarValue, givenUA)
			}
		})
	}
}
func TestCustomUserAgentAndAppendViaEnvVar(t *testing.T) {
	if oldenv, isSet := os.LookupEnv(appendUaEnvVar); isSet {
		defer os.Setenv(appendUaEnvVar, oldenv)
	} else {
		defer os.Unsetenv(appendUaEnvVar)
	}
	if oldenv, isSet := os.LookupEnv(customUaEnvVar); isSet {
		defer os.Setenv(customUaEnvVar, oldenv)
	} else {
		defer os.Unsetenv(customUaEnvVar)
	}

	testCases := []struct {
		customUaValue string
		appendUaValue string
		expected      string
	}{
		{"", "", "OpenTofu/0.0.0"},
		{"", " ", "OpenTofu/0.0.0"},
		{"", " \n", "OpenTofu/0.0.0"},
		{"", "testy test", "OpenTofu/0.0.0 testy test"},
		{"opensource", "opentofu", "opensource opentofu"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			os.Unsetenv(customUaEnvVar)
			os.Unsetenv(appendUaEnvVar)
			os.Setenv(customUaEnvVar, tc.customUaValue)
			os.Setenv(appendUaEnvVar, tc.appendUaValue)
			givenUA := OpenTofuUserAgent("0.0.0")
			if givenUA != tc.expected {
				t.Fatalf("Expected User-Agent '%s' does not match '%s'", tc.expected, givenUA)
			}
		})
	}
}
