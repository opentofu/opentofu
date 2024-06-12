// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestHTTPProxy(t *testing.T) {
	testutils.SetupTestLogger(t)
	ctx := testutils.Context(t)

	t.Logf("🌎 Setting up backing HTTP server...")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := listener.Addr().(*net.TCPAddr) //nolint:errcheck //This is always a TCPAddr, see above.
	addr := tcpAddr.IP.String() + ":" + strconv.Itoa(tcpAddr.Port)
	server := http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("Hello world!"))
		}),
		ReadTimeout:       httpTimeouts,
		ReadHeaderTimeout: httpTimeouts,
		WriteTimeout:      httpTimeouts,
		IdleTimeout:       httpTimeouts,
	}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		_ = server.Shutdown(cleanupCtx)
	})

	t.Logf("⌚ Waiting for backing server to come up...")
	directClient := http.Client{}
	upContext, upCancel := context.WithTimeout(ctx, 30*time.Second)
	defer upCancel()
	for {
		//goland:noinspection HttpUrlsUsage
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr, nil)
		if err != nil {
			t.Fatalf("❌ Failed to create HTTP request: %v", err)
		}
		var resp *http.Response
		resp, err = directClient.Do(req)
		if err == nil {
			break
		}
		_ = resp.Body.Close()
		t.Logf("⌚ Still waiting for backing server to come up...")
		select {
		case <-upContext.Done():
			t.Fatalf("❌ Timed out waiting for backing HTTP server to come up.")
		case <-time.After(time.Second):
		}
	}

	t.Logf("🪧 Setting up proxy server...")
	proxy := testutils.HTTPProxy(t, testutils.HTTPProxyOptionForceHTTPTarget(addr))
	proxyClient := http.Client{
		Transport: &http.Transport{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return proxy.HTTPProxy(), nil
			},
		},
	}

	t.Logf("📡 Testing proxy functionality in HTTP mode...")
	// This request normally shouldn't work, but the proxy server should override it and connect to the correct
	// backing server, proving that the proxying works as intended.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("❌ Failed to create HTTP request: %v", err)
	}
	resp, err := proxyClient.Do(req)
	if err != nil {
		t.Fatalf("❌ HTTP request to proxy failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("❌ Incorrect status code from proxy: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Hello world!" {
		t.Fatalf("❌ Incorrect response from proxy: %s", string(body))
	}
	t.Logf("✅ Proxy server works as intended.")
}
