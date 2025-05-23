// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-plugin"
	proto "github.com/opentofu/opentofu/internal/tfplugin5"
)

const (
	// The constants below are the names of the plugins that can be dispensed
	// from the plugin server.
	ProviderPluginName    = "provider"
	ProvisionerPluginName = "provisioner"

	// DefaultProtocolVersion is the protocol version assumed for legacy clients that don't specify
	// a particular version during their handshake. This is the version used when Terraform 0.10
	// and 0.11 launch plugins that were built with support for both versions 4 and 5, and must
	// stay unchanged at 4 until we intentionally build plugins that are not compatible with 0.10 and
	// 0.11.
	DefaultProtocolVersion = 4
)

// Handshake is the HandshakeConfig used to configure clients and servers.
var Handshake = plugin.HandshakeConfig{
	// The ProtocolVersion is the version that must match between TF core
	// and TF plugins. This should be bumped whenever a change happens in
	// one or the other that makes it so that they can't safely communicate.
	// This could be adding a new interface value, it could be how
	// helper/schema computes diffs, etc.
	ProtocolVersion: DefaultProtocolVersion,

	// The magic cookie values should NEVER be changed.
	MagicCookieKey:   "TF_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "d602bf8f470bc67ca7faa0386276bbdd4330efaf76d1a219cb4d6991ca9872b2",
}

type GRPCProviderFunc func() proto.ProviderServer
type GRPCProvisionerFunc func() proto.ProvisionerServer

// ServeOpts are the configurations to serve a plugin.
type ServeOpts struct {
	// Wrapped versions of the above plugins will automatically shimmed and
	// added to the GRPC functions when possible.
	GRPCProviderFunc    GRPCProviderFunc
	GRPCProvisionerFunc GRPCProvisionerFunc
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve(opts *ServeOpts) {
	tlsProviderFunc := func() (*tls.Config, error) {
		// FIPS Compliance: Use custom TLS config in FIPS mode.
		isFipsMode := strings.Contains(os.Getenv("GODEBUG"), "fips140=on")
		if !isFipsMode {
			// Not in FIPS mode, let go-plugin handle TLS (likely AutoMTLS default)
			return nil, nil
		}

		log.Println("[INFO] FIPS mode detected, configuring custom mTLS for plugin server (v5)")

		// Use absolute path to ensure certs are found during tests
		certDir := "/Users/topperge/Projects/Personal/opentofu/fips-certs"
		caCertPath := filepath.Join(certDir, "ca.pem")
		serverCertPath := filepath.Join(certDir, "server.pem")
		serverKeyPath := filepath.Join(certDir, "server-key.pem")

		caCertPEM, err := os.ReadFile(caCertPath)
		if err != nil {
			log.Printf("[ERROR] Failed to read FIPS CA cert for server (v5): %v", err)
			return nil, err // Fail hard if certs can't be read in FIPS mode
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCertPEM)

		serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			log.Printf("[ERROR] Failed to load FIPS server cert/key (v5): %v", err)
			return nil, err // Fail hard
		}

		return &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert, // Require client cert
			MinVersion:   tls.VersionTLS12,               // Restore FIPS requirement
		}, nil
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig:  Handshake,
		VersionedPlugins: pluginSet(opts),
		GRPCServer:       plugin.DefaultGRPCServer,
		TLSProvider:      tlsProviderFunc,
	})
}

func pluginSet(opts *ServeOpts) map[int]plugin.PluginSet {
	plugins := map[int]plugin.PluginSet{}

	// add the new protocol versions if they're configured
	if opts.GRPCProviderFunc != nil || opts.GRPCProvisionerFunc != nil {
		plugins[5] = plugin.PluginSet{}
		if opts.GRPCProviderFunc != nil {
			plugins[5]["provider"] = &GRPCProviderPlugin{
				GRPCProvider: opts.GRPCProviderFunc,
			}
		}
		if opts.GRPCProvisionerFunc != nil {
			plugins[5]["provisioner"] = &GRPCProvisionerPlugin{
				GRPCProvisioner: opts.GRPCProvisionerFunc,
			}
		}
	}
	return plugins
}
