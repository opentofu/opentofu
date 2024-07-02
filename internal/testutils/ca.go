// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	mathRand "math/rand"
	"net"
	"sync"
	"testing"
	"time"
)

const caKeySize = 2048
const expirationYears = 10

// CA creates an x509 CA certificate that can produce certificates for testing purposes.
func CA(t *testing.T) CertificateAuthority {
	// We use a non-deterministic cheap randomness source because the certificate won't be reproducible anyway due to
	// the NotBefore / NotAfter being different every time. We don't use crypto/rand.Rand because it can get blocked
	// if not enough entropy is available and it doesn't matter for the test use case.
	random := RandomSource()

	caCert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"OpenTofu a Series of LF Projects, LLC"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(expirationYears, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivateKey, err := rsa.GenerateKey(random, caKeySize)
	if err != nil {
		t.Skipf("Failed to create private key: %v", err)
	}
	caCertData, err := x509.CreateCertificate(random, caCert, caCert, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		t.Skipf("Failed to create CA certificate: %v", err)
	}
	caPEM := new(bytes.Buffer)
	if err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertData,
	}); err != nil {
		t.Skipf("Failed to encode CA cert: %v", err)
	}
	return &ca{
		t:          t,
		random:     random,
		caCert:     caCert,
		caCertPEM:  caPEM.Bytes(),
		privateKey: caPrivateKey,
		serial:     big.NewInt(0),
		lock:       &sync.Mutex{},
	}
}

// CertConfig is the configuration structure for creating specialized certificates using
// CertificateAuthority.CreateConfiguredServerCert.
type CertConfig struct {
	// IPAddresses contains a list of IP addresses that should be added to the SubjectAltName field of the certificate.
	IPAddresses []string
	// Hosts contains a list of host names that should be added to the SubjectAltName field of the certificate.
	Hosts []string
	// Subject is the subject (CN, etc) setting for the certificate. Most commonly, you will want the CN field to match
	// one of hour host names.
	Subject pkix.Name
	// ExtKeyUsage describes the extended key usage. Typically, this should be:
	//
	//     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	ExtKeyUsage []x509.ExtKeyUsage
}

// KeyPair contains a certificate and private key in PEM format.
type KeyPair struct {
	// Certificate contains an x509 certificate in PEM format.
	Certificate []byte
	// PrivateKey contains an RSA or other private key in PEM format.
	PrivateKey []byte
}

// GetPrivateKey returns a crypto.Signer for the private key.
func (k KeyPair) GetPrivateKey() crypto.PrivateKey {
	block, _ := pem.Decode(k.PrivateKey)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return key
}

// GetTLSCertificate returns the tls.Certificate based on this key pair.
func (k KeyPair) GetTLSCertificate() tls.Certificate {
	cert, err := tls.X509KeyPair(k.Certificate, k.PrivateKey)
	if err != nil {
		panic(err)
	}
	return cert
}

// GetServerTLSConfig returns a tls.Config suitable for a TLS server with this key pair.
func (k KeyPair) GetServerTLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{
			k.GetTLSCertificate(),
		},
		MinVersion: tls.VersionTLS12,
	}
}

// CertificateAuthority provides simple access to x509 CA functions for testing purposes only.
type CertificateAuthority interface {
	// GetPEMCACert returns the CA certificate in PEM format.
	GetPEMCACert() []byte
	// GetCertPool returns an x509.CertPool configured for this CA.
	GetCertPool() *x509.CertPool
	// GetClientTLSConfig returns a *tls.Config with a valid cert pool configured for this CA.
	GetClientTLSConfig() *tls.Config
	// CreateLocalhostServerCert creates a server certificate pre-configured for "localhost", which is sufficient for
	// most test cases.
	CreateLocalhostServerCert() KeyPair
	// CreateLocalhostClientCert creates a client certificate pre-configured for "localhost", which is sufficient for
	// most test cases.
	CreateLocalhostClientCert() KeyPair
	// CreateConfiguredCert creates a certificate with a specialized configuration.
	CreateConfiguredCert(config CertConfig) KeyPair
}

type ca struct {
	caCert     *x509.Certificate
	caCertPEM  []byte
	privateKey *rsa.PrivateKey
	serial     *big.Int
	lock       *sync.Mutex
	t          *testing.T
	random     *mathRand.Rand
}

func (c *ca) GetClientTLSConfig() *tls.Config {
	certPool := c.GetCertPool()

	return &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}
}

func (c *ca) GetCertPool() *x509.CertPool {
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(c.caCertPEM)
	return certPool
}

func (c *ca) GetPEMCACert() []byte {
	return c.caCertPEM
}

func (c *ca) CreateConfiguredCert(config CertConfig) KeyPair {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.serial.Add(c.serial, big.NewInt(1))

	ipAddresses := make([]net.IP, len(config.IPAddresses))
	for i, ip := range config.IPAddresses {
		ipAddresses[i] = net.ParseIP(ip)
	}

	cert := &x509.Certificate{
		SerialNumber: c.serial,
		Subject:      config.Subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, 1),
		SubjectKeyId: []byte{1},
		ExtKeyUsage:  config.ExtKeyUsage,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     config.Hosts,
		IPAddresses:  ipAddresses,
	}
	certPrivKey, err := rsa.GenerateKey(c.random, caKeySize)
	if err != nil {
		c.t.Skipf("Failed to generate private key: %v", err)
	}
	certBytes, err := x509.CreateCertificate(
		c.random,
		cert,
		c.caCert,
		&certPrivKey.PublicKey,
		c.privateKey,
	)
	if err != nil {
		c.t.Skipf("Failed to create certificate: %v", err)
	}
	certPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	}); err != nil {
		c.t.Skipf("Failed to encode private key: %v", err)
	}
	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM,
		&pem.Block{Type: "CERTIFICATE", Bytes: certBytes},
	); err != nil {
		c.t.Skipf("Failed to encode certificate: %v", err)
	}
	return KeyPair{
		Certificate: certPEM.Bytes(),
		PrivateKey:  certPrivKeyPEM.Bytes(),
	}
}

func (c *ca) CreateLocalhostServerCert() KeyPair {
	return c.CreateConfiguredCert(CertConfig{
		IPAddresses: []string{"127.0.0.1", "::1"},
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"OpenTofu a Series of LF Projects, LLC"},
			CommonName:   "localhost",
		},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		Hosts: []string{
			"localhost",
		},
	})
}

func (c *ca) CreateLocalhostClientCert() KeyPair {
	return c.CreateConfiguredCert(CertConfig{
		IPAddresses: []string{"127.0.0.1", "::1"},
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"OpenTofu a Series of LF Projects, LLC"},
			CommonName:   "localhost",
		},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		Hosts: []string{
			"localhost",
		},
	})
}
