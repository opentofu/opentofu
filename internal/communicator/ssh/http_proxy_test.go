// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ssh

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/proxy"
)

func TestProxyDialer_Dial_connectResponseError(t *testing.T) {
	// A proxy that accepts the CONNECT request and then closes without
	// answering must surface a dial error, not crash the process
	addr := fakeProxy(t, func(net.Conn) {})

	pd := &proxyDialer{
		proxy:   *newProxyInfo(addr, "http", "", ""),
		forward: proxy.Direct,
	}

	conn, err := pd.Dial("tcp", "192.0.2.1:22")
	if err == nil {
		conn.Close()
		t.Fatal("expected an error, got nil")
	}
}

func TestProxyDialer_Dial_success(t *testing.T) {
	// A proxy that answers the CONNECT request with 200 must yield a connection
	addr := fakeProxy(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	})

	pd := &proxyDialer{
		proxy:   *newProxyInfo(addr, "http", "", ""),
		forward: proxy.Direct,
	}

	conn, err := pd.Dial("tcp", "192.0.2.1:22")
	if err != nil {
		t.Fatalf("Dial returned error: %v, want nil", err)
	}
	defer conn.Close()
}

func TestProxyDialer_Dial_nonOKStatus(t *testing.T) {
	// A proxy that rejects the CONNECT request must surface its status code as a dial error
	addr := fakeProxy(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n"))
	})

	pd := &proxyDialer{
		proxy:   *newProxyInfo(addr, "http", "", ""),
		forward: proxy.Direct,
	}

	conn, err := pd.Dial("tcp", "192.0.2.1:22")
	if err == nil {
		conn.Close()
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "407") {
		t.Fatalf("error = %v, want it to mention 407", err)
	}
}

// fakeProxy starts an HTTP proxy on the loopback interface and returns its address.
// It accepts one connection, reads the CONNECT request, calls respond and then
// closes: a respond that writes nothing models a proxy that hangs up without answering
func fakeProxy(t *testing.T, respond func(net.Conn)) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		_, _ = http.ReadRequest(bufio.NewReader(conn))
		respond(conn)
	}()

	t.Cleanup(func() {
		ln.Close()
		<-done
	})

	return ln.Addr().String()
}
