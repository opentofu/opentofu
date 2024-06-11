// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
)

// HTTPClientForCA returns an HTTP client for testing purposes only configured for a specific CA certificate.
func HTTPClientForCA(caCert []byte) *http.Client {
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
	}
}
