// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/testutils"
)

// TestHTTPProxy tests the HTTPProxy functionality using traditional HTTP and HTTPS connections.
func TestHTTPProxy(t *testing.T) {
	testutils.SetupTestLogger(t)
	ctx := testutils.Context(t)

	t.Run("Backend: HTTP", func(t *testing.T) {
		httpAddr := testHTTPProxySetupBackingHTTPServer(t, ctx)
		t.Logf("🪧 Setting up proxy server...")
		proxy := testutils.HTTPProxy(t, testutils.HTTPProxyOptionForceHTTPTarget(httpAddr))

		testHTTPProxyRequests(t, proxy, ctx)
	})
	t.Run("Backend: HTTPS", func(t *testing.T) {
		backingCA := testutils.CA(t)
		httpAddr := testHTTPProxySetupBackingHTTPSServer(t, ctx, backingCA)
		t.Logf("🪧 Setting up proxy server...")
		proxy := testutils.HTTPProxy(t, testutils.HTTPProxyOptionForceHTTPSTarget(httpAddr, backingCA.GetPEMCACert()))

		testHTTPProxyRequests(t, proxy, ctx)
	})
	t.Run("Backend: TLS", func(t *testing.T) {
		testHTTPProxyInConnectMode(t)
	})
}

func testHTTPProxyInConnectMode(t *testing.T) {
	backingServer, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("❌ Failed to start TCP server (%v)", err)
	}
	t.Cleanup(func() {
		// Note: this will also stop the goroutine below.
		_ = backingServer.Close()
	})
	addr := backingServer.Addr().(*net.TCPAddr) //nolint:errcheck //This is always a TCPAddr, see above.
	addrPort := addr.IP.String() + ":" + strconv.Itoa(addr.Port)

	t.Logf("🪧 Setting up proxy server...")
	proxy := testutils.HTTPProxy(t, testutils.HTTPProxyOptionForceCONNECTTarget(addrPort))

	t.Logf("🔍 Running functionality tests...")
	var backingErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := backingServer.Accept()
		if e != nil {
			backingErr = fmt.Errorf("backing server failed to accept connection (%w)", e)
			return
		}
		t.Logf("✅ Backing server accepted the connection from the proxy.")
		expectedBytes := len("Say hi!")
		request := make([]byte, expectedBytes)
		n, e := io.ReadAtLeast(conn, request, expectedBytes)
		if e != nil {
			backingErr = fmt.Errorf("failed to read request (%w)", e)
			return
		}
		if n != len(request) {
			backingErr = fmt.Errorf("incorrect number of bytes read: %d", n)
			return
		}
		response := "Hello world!"
		if string(request) != "Say hi!" {
			t.Logf("❌ Backing server read an incorrect request: %s", request)
			response = fmt.Sprintf("Incorrect request received: %s", request)
		} else {
			t.Logf("✅ Backing server read the correct request.")
		}
		_, e = conn.Write([]byte(response))
		if e != nil {
			backingErr = fmt.Errorf("backing server failed to write to connection (%w)", e)
			return
		}
		t.Logf("✅ Backing sent the response.")
		e = conn.Close()
		if e != nil {
			backingErr = fmt.Errorf("backing server failed to close connection (%w)", e)
			return
		}
		t.Logf("✅ Backing server finished working.")
	}()

	t.Logf("🔌 Client connecting to the proxy server...")
	proxyConn, err := net.Dial("tcp", proxy.HTTPProxy().Host)
	if err != nil {
		t.Fatalf("❌ Failed to connect to the proxy server: %v", err)
	}
	t.Cleanup(func() {
		_ = proxyConn.Close()
	})
	t.Logf("✅ Proxy connection established.")
	t.Logf("🙇 Client sending the CONNECT request to the proxy server...")
	// We provide an obviously invalid address here to make sure the proxy connect override works as intended.
	_, err = proxyConn.Write([]byte("CONNECT 127.0.0.1:1 HTTP/1.1\r\nHost: " + proxy.HTTPProxy().Host + "\r\n\r\n"))
	if err != nil {
		t.Fatalf("❌ Failed to send CONNECT header to the proxy server: %v", err)
	}
	t.Logf("✅ CONNECT request sent.")

	// We send our greeting:
	t.Logf("👋 Client sending the greeting to the backing service via the proxy...")
	_, err = proxyConn.Write([]byte("Say hi!"))
	if err != nil {
		t.Fatalf("❌ Failed to send greeting through the proxy server: %v", err)
	}
	t.Logf("✅ Greeting request sent to backing server.")

	t.Logf("⌚ Client waiting for the response from the backing server...")
	response, err := io.ReadAll(proxyConn)
	if err != nil {
		t.Fatalf("❌ Failed to read response from proxy server: %v", err)
	}
	t.Logf("✅ Response received.")

	if string(response) != "Hello world!" {
		t.Fatalf("❌ Invalid response received from proxy server: %s", string(response))
	}
	t.Logf("✅ Response is correct.")

	t.Logf("⌚ Waiting for the backing server goroutine to finish...")
	<-done
	if backingErr != nil {
		t.Fatalf("❌ Backing server error: %v", backingErr)
	}
	t.Logf("✅ Proxy server works as intended in CONNECT mode.")
}

func testHTTPProxyRequests(t *testing.T, proxy testutils.HTTPProxyService, ctx context.Context) {
	t.Logf("🔍 Running functionality tests...")

	t.Run("Client: HTTP", func(t *testing.T) {
		t.Logf("📡 Testing proxy functionality in HTTP mode...")

		proxyClient := http.Client{
			Transport: &http.Transport{
				Proxy: func(_ *http.Request) (*url.URL, error) {
					return proxy.HTTPProxy(), nil
				},
			},
		}

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
		t.Logf("✅ Proxy server works as intended in HTTP mode.")
	})
	t.Run("Client: HTTPS", func(t *testing.T) {
		t.Logf("📡 Testing proxy functionality in HTTPS mode...")

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(proxy.CACert())
		proxyClient := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:    certPool,
					MinVersion: tls.VersionTLS12,
				},
				Proxy: func(_ *http.Request) (*url.URL, error) {
					return proxy.HTTPSProxy(), nil
				},
			},
		}

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
		t.Logf("✅ Proxy server works as intended in HTTPS mode.")
	})
	t.Logf("🔍 Functionality tests complete.")
}

func testHTTPProxySetupBackingHTTPServer(t *testing.T, ctx context.Context) string {
	t.Logf("🌎 Setting up backing HTTP server...")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := listener.Addr().(*net.TCPAddr) //nolint:errcheck //This is always a TCPAddr, see above.
	addr := testHTTPProxyStartHTTPServer(t, tcpAddr, listener)
	testHTTPProxyWaitForHTTPServer(t, ctx, addr, nil)
	return addr
}

func testHTTPProxySetupBackingHTTPSServer(t *testing.T, ctx context.Context, ca testutils.CertificateAuthority) string {
	t.Logf("🌎 Setting up backing HTTPS server...")
	cert := ca.CreateLocalhostServerCert()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{
			cert.GetTLSCertificate(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := listener.Addr().(*net.TCPAddr) //nolint:errcheck //This is always a TCPAddr, see above.
	addr := testHTTPProxyStartHTTPServer(t, tcpAddr, listener)
	testHTTPProxyWaitForHTTPServer(t, ctx, addr, ca.GetPEMCACert())
	return addr
}

func testHTTPProxyStartHTTPServer(t *testing.T, tcpAddr *net.TCPAddr, listener net.Listener) string {
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
	return addr
}

func testHTTPProxyWaitForHTTPServer(t *testing.T, ctx context.Context, addr string, caCert []byte) {
	var err error
	t.Logf("⌚ Waiting for backing server to come up...")

	upContext, upCancel := context.WithTimeout(ctx, 30*time.Second)
	defer upCancel()

	directClient := http.Client{}
	var checkAddr string
	if len(caCert) > 0 {
		checkAddr = "https://" + addr
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCert)
		directClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
			},
		}
	} else {
		//goland:noinspection HttpUrlsUsage
		checkAddr = "http://" + addr
	}

	for {
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, checkAddr, nil)
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
}
