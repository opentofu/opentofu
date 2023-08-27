// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	appendUaEnvVar         = "TF_APPEND_USER_AGENT"
	customUaEnvVar         = "OPENTF_USER_AGENT"
	defaultApplicationName = "OpenTF"
)

type userAgentRoundTripper struct {
	inner     http.RoundTripper
	userAgent string
}

func (rt *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", rt.userAgent)
	}
	log.Printf("[TRACE] HTTP client %s request to %s", req.Method, req.URL.String())
	return rt.inner.RoundTrip(req)
}

func Application() string {
	if customApplication := os.Getenv(customUaEnvVar); customApplication != "" {
		return customApplication
	}
	return defaultApplicationName
}

func OpenTfUserAgent(version string) string {
	ua := fmt.Sprintf("%s/%s", defaultApplicationName, version)
	if customUa := os.Getenv(customUaEnvVar); customUa != "" {
		ua = customUa
	}

	if add := os.Getenv(appendUaEnvVar); add != "" {
		add = strings.TrimSpace(add)
		if len(add) > 0 {
			ua += " " + add
			log.Printf("[DEBUG] Using modified User-Agent: %s", ua)
		}
	}

	return ua
}
