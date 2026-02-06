// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/logging"
)

// NewForRegistryRequests is a variant of [New] that deals with some additional
// policy concerns related to "registry requests".
//
// Exactly what constitutes a "registry request" is actually more about
// historical technical debt than intentional design, since these concerns
// were originally supposed to be handled internally within the module and
// provider registry clients but the implementation of that unfortunately caused
// the effects to "leak out" into other parts of the system, which we now
// preserve for backward compatibility here.
//
// Therefore "registry requests" includes the following:
//   - All requests from the client of our network service discovery protocol,
//     even though not all discoverable services are actually "registries".
//   - Requests to module registries during module installation.
//   - Requests to provider registries during provider installation.
//
// The retryCount argument specifies how many times requests from the resulting
// client should be automatically retried when certain transient errors occur.
//
// The timeout argument specifies a deadline for the completion of each
// request made using the client.
func NewForRegistryRequests(ctx context.Context, retryCount int, timeout time.Duration) *retryablehttp.Client {
	// We'll start with the result of New, so that what we return still
	// honors our general policy for HTTP client behavior.
	baseClient := New(ctx)
	baseClient.Timeout = timeout

	// Registry requests historically offered automatic retry on certain
	// transient errors implemented using the retryablehttp library, so
	// we'll now deal with that here.
	retryableClient := retryablehttp.NewClient()
	retryableClient.HTTPClient = baseClient
	retryableClient.RetryMax = retryCount
	retryableClient.RequestLogHook = registryRequestLogHook
	retryableClient.ErrorHandler = registryMaxRetryErrorHandler
	retryableClient.Logger = logging.HCLogger()

	return retryableClient
}

func registryRequestLogHook(logger retryablehttp.Logger, req *http.Request, i int) {
	if i > 0 {
		logger.Printf("[INFO] Failed request to %s; retrying", req.URL.String())
	}
}

func registryMaxRetryErrorHandler(resp *http.Response, err error, numTries int) (*http.Response, error) {
	// Close the body per library instructions
	if resp != nil {
		resp.Body.Close()
	}

	// Additional error detail: if we have a response, use the status code;
	// if we have an error, use that; otherwise nothing. We will never have
	// both response and error.
	var errMsg string
	if resp != nil {
		errMsg = fmt.Sprintf(": %s returned from %s", resp.Status, resp.Request.URL)
	} else if err != nil {
		errMsg = fmt.Sprintf(": %s", err)
	}

	// This function is always called with numTries=RetryMax+1. If we made any
	// retry attempts, include that in the error message.
	if numTries > 1 {
		return resp, fmt.Errorf("request failed after %d attempts%s",
			numTries, errMsg)
	}
	return resp, fmt.Errorf("request failed%s", errMsg)
}
