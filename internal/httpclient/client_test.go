// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/opentofu/opentofu/version"
)

func TestNew_userAgent(t *testing.T) {

	appendUaVal := os.Getenv(appendUaEnvVar)
	os.Unsetenv(appendUaEnvVar)
	defer os.Setenv(appendUaEnvVar, appendUaVal)

	var actualUserAgent string
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		actualUserAgent = req.UserAgent()
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	for i, c := range []struct {
		expected string
		request  func(c *http.Client) error
	}{
		{
			fmt.Sprintf("OpenTofu/%s", version.Version),
			func(c *http.Client) error {
				_, err := c.Get(ts.URL)
				return err
			},
		},
		{
			"foo/1",
			func(c *http.Client) error {
				req := &http.Request{
					Method: "GET",
					URL:    tsURL,
					Header: http.Header{
						"User-Agent": []string{"foo/1"},
					},
				}
				_, err := c.Do(req)
				return err
			},
		},
		{
			"",
			func(c *http.Client) error {
				req := &http.Request{
					Method: "GET",
					URL:    tsURL,
					Header: http.Header{
						"User-Agent": []string{""},
					},
				}
				_, err := c.Do(req)
				return err
			},
		},
	} {
		t.Run(fmt.Sprintf("%d %s", i, c.expected), func(t *testing.T) {
			actualUserAgent = ""
			cli := New(t.Context())
			err := c.request(cli)
			if err != nil {
				t.Fatal(err)
			}
			if actualUserAgent != c.expected {
				t.Fatalf("actual User-Agent '%s' is not '%s'", actualUserAgent, c.expected)
			}
		})
	}
}
