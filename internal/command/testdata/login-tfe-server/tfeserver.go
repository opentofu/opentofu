// Package tfeserver is a test stub implementing a subset of the TFE API used
// only for the testing of the "tofu login" command.
package tfeserver

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	goodToken      = "good-token"
	accountDetails = `{"data":{"id":"user-abc123","type":"users","attributes":{"username":"testuser","email":"testuser@example.com"}}}`
	// NOTE: The [bold] and [reset] sequences here are the supported way for
	// a motd.v1 implementation to introduce limited color sequences to the
	// output. No _direct_ control characters are allowed, so the U+0000
	// character at the end is a disallowed character included intentionally
	// so we can verify that it is filtered as expected.
	MOTD = `{"msg":"Welcome to [bold]the cloud backend[reset]!\u0000"}`
)

// Handler is an implementation of net/http.Handler that provides a stub
// TFE API server implementation with the following endpoints:
//
//	/ping            - API existence endpoint
//	/account/details - current user endpoint
var Handler http.Handler

type handler struct{}

func (h handler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	resp.Header().Set("Content-Type", "application/vnd.api+json")
	switch req.URL.Path {
	case "/api/v2/ping":
		h.servePing(resp, req)
	case "/api/v2/account/details":
		h.serveAccountDetails(resp, req)
	case "/api/terraform/motd":
		h.serveMOTD(resp, req)
	default:
		fmt.Printf("404 when fetching %s\n", req.URL.String())
		http.Error(resp, `{"errors":[{"status":"404","title":"not found"}]}`, http.StatusNotFound)
	}
}

func (h handler) servePing(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusNoContent)
}

func (h handler) serveAccountDetails(resp http.ResponseWriter, req *http.Request) {
	if !strings.Contains(req.Header.Get("Authorization"), goodToken) {
		http.Error(resp, `{"errors":[{"status":"401","title":"unauthorized"}]}`, http.StatusUnauthorized)
		return
	}

	resp.WriteHeader(http.StatusOK)
	resp.Write([]byte(accountDetails))
}

func (h handler) serveMOTD(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
	resp.Write([]byte(MOTD))
}

func init() {
	Handler = handler{}
}
