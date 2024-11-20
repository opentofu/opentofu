// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// Helper function to parse response body bytes into a string for logging
func parseResponseBodyForLog(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read HTTP response body for Logging: %v", err)
		return ""
	}
	return string(body)
}

// Helper function to parse http headers into a json string for logging
func parseHeadersForLog(resp *http.Response) string {
	// Blacklist of Header keys that need to be masked
	var blacklist = map[string]bool{
		"authorization": true,
		"cookie":        true,
		"set-cookie":    true,
	}

	headers := make(map[string]string, len(resp.Header))
	for key := range resp.Header {
		if blacklist[strings.ToLower(key)] {
			headers[key] = "[MASKED]"
		} else {
			headers[key] = resp.Header.Get(key)
		}
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal headers to JSON for Logging: %v", err)
		return ""
	}
	return string(headersJSON)
}
