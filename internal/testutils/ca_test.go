// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestCA(t *testing.T) {
	t.Run("correct", testCACorrectCertificate)
	t.Run("incorrect", testCAIncorrectCertificate)
}

func testCAIncorrectCertificate(t *testing.T) {
	ca1 := testutils.CA(t)
	ca2 := testutils.CA(t)

	if bytes.Equal(ca1.GetPEMCACert(), ca2.GetPEMCACert()) {
		t.Fatalf("The two CA's have the same CA PEM!")
	}

	done := make(chan struct{})
	var serverErr error
	t.Logf("🍦 Setting up TLS server...")
	tlsListener := testutils.Must2(tls.Listen(
		"tcp",
		"127.0.0.1:0",
		ca1.CreateLocalhostServerCert().GetServerTLSConfig()),
	)
	t.Cleanup(func() {
		t.Logf("🍦 Server closing listener...")
		_ = tlsListener.Close()
	})
	port := tlsListener.Addr().(*net.TCPAddr).Port
	go func() {
		defer close(done)
		t.Logf("🍦 Server accepting connection...")
		var conn net.Conn
		conn, serverErr = tlsListener.Accept()
		if serverErr != nil {
			t.Logf("🍦 Server correctly received an error: %v", serverErr)
			return
		}
		// Force a handshake even without read/write. The client automatically performs
		// the handshake, but the server listener doesn't before reading.
		serverErr = conn.(*tls.Conn).HandshakeContext(context.Background())
		if serverErr == nil {
			t.Logf("❌ Server unexpectedly did not receive an error.")
		} else {
			t.Logf("🍦 Server correctly received an error: %v", serverErr)
		}
		_ = conn.Close()
	}()
	t.Logf("🔌 Client connecting to server...")
	conn, err := tls.Dial(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		ca2.GetClientTLSConfig(),
	)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("❌ The TLS connection succeeded despite the incorrect CA certificate.")
	}
	t.Logf("🔌 Client correctly received an error: %v", err)
	<-done
	if serverErr == nil {
		t.Fatalf("❌ The TLS server didn't error despite the incorrect CA certificate.")
	}
}

func testCACorrectCertificate(t *testing.T) {
	ca := testutils.CA(t)
	const testGreeting = "Hello world!"

	var serverErr error
	t.Cleanup(func() {
		if serverErr != nil {
			t.Fatalf("❌ TLS server failed: %v", serverErr)
		}
	})

	done := make(chan struct{})

	t.Logf("🍦 Setting up TLS server...")
	tlsListener := testutils.Must2(tls.Listen("tcp", "127.0.0.1:0", ca.CreateLocalhostServerCert().GetServerTLSConfig()))
	t.Cleanup(func() {
		t.Logf("🍦 Server closing listener...")
		_ = tlsListener.Close()
	})
	t.Logf("🍦 Starting TLS server...")
	go func() {
		defer close(done)
		var conn net.Conn
		t.Logf("🍦 Server accepting connection...")
		conn, serverErr = tlsListener.Accept()
		if serverErr != nil {
			t.Errorf("❌ Server accept failed: %v", serverErr)
			return
		}
		defer func() {
			t.Logf("🍦 Server closing connection.")
			_ = conn.Close()
		}()
		t.Logf("🍦 Server writing greeting...")
		_, serverErr = conn.Write([]byte(testGreeting))
		if serverErr != nil {
			t.Errorf("❌ Server write failed: %v", serverErr)
			return
		}
	}()
	t.Logf("🔌 Client connecting to server...")
	port := tlsListener.Addr().(*net.TCPAddr).Port
	client := testutils.Must2(tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), ca.GetClientTLSConfig()))
	defer func() {
		t.Logf("🔌 Client closing connection...")
		_ = client.Close()
	}()
	t.Logf("🔌 Client reading greeting...")
	greeting := testutils.Must2(io.ReadAll(client))
	if string(greeting) != testGreeting {
		t.Fatalf("❌ Client received incorrect greeting: %s", greeting)
	}
	t.Logf("🔌 Waiting for server to finish...")
	<-done
}
