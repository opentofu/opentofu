// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// HTTP request body reader that deliberately causes a read error
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("read error")
}

func (e *errorReader) Close() error {
	return nil
}

func TestParseResponseBodyForLog(t *testing.T) {
	testCases := []struct {
		name           string
		responseBody   string
		expectedOutput string
	}{{
		name:           "Valid json response body",
		responseBody:   `{"error":"Unauthorized"}`,
		expectedOutput: `{"error":"Unauthorized"}`,
	},
		{
			name:           "Empty response body",
			responseBody:   "",
			expectedOutput: "",
		},
		{
			name:           "Response body with special characters",
			responseBody:   "Special characters: !@#$%^&*()",
			expectedOutput: "Special characters: !@#$%^&*()",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewBufferString(tt.responseBody)),
			}

			output := parseResponseBodyForLog(resp)
			if output != tt.expectedOutput {
				t.Errorf("parseResponseBody() = %v; want %v", output, tt.expectedOutput)
			}
		})
	}

	t.Run("Error reading response body", func(t *testing.T) {
		resp := &http.Response{
			Body: io.NopCloser(&errorReader{}),
		}

		output := parseResponseBodyForLog(resp)
		if output != "" {
			t.Errorf("parseResponseBody() = %v; want %v", output, "")
		}
	})
}

func TestParseHeadersForLog(t *testing.T) {
	testCases := []struct {
		name           string
		headers        http.Header
		expectedOutput string
	}{
		{
			name: "Headers with sensitive information",
			headers: http.Header{
				"Authorization": []string{"token"},
				"Set-Cookie":    []string{"cookies"},
				"Cookie":        []string{"cookies"},
				"Content-Type":  []string{"application/json"},
			},
			expectedOutput: `{"Authorization":"[MASKED]","Content-Type":"application/json","Cookie":"[MASKED]","Set-Cookie":"[MASKED]"}`,
		},
		{
			name: "Case senstive test with sensitive information Headers",
			headers: http.Header{
				"authorization": []string{"token"},
				"set-Cookie":    []string{"cookies"},
				"cookie":        []string{"cookies"},
				"content-type":  []string{"application/json"},
			},
			expectedOutput: `{"authorization":"[MASKED]","content-type":"","cookie":"[MASKED]","set-Cookie":"[MASKED]"}`,
		},
		{
			name: "Headers without sensitive information",
			headers: http.Header{
				"Content-Type":                []string{"application/json"},
				"Access-Control-Allow-Origin": []string{"*"},
				"Connection":                  []string{"Keep-Alive"},
				"Server":                      []string{"Apache"},
				"Keep-Alive":                  []string{"timeout=5, max=997"},
			},
			expectedOutput: `{"Access-Control-Allow-Origin":"*","Connection":"Keep-Alive","Content-Type":"application/json","Keep-Alive":"timeout=5, max=997","Server":"Apache"}`,
		},
		{
			name:           "Empty headers",
			headers:        http.Header{},
			expectedOutput: "{}",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: tt.headers,
			}

			output := parseHeadersForLog(resp)
			if output != tt.expectedOutput {
				t.Errorf("parseResponseHeaders() = %v; want %v", output, tt.expectedOutput)
			}
		})
	}
}
