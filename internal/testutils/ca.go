// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

const caKeySize = 1024
const expirationYears = 10

// CA creates an x509 CA certificate that can produce certificates for testing purposes.
func CA(t *testing.T) CertificateAuthority {
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
	caPrivateKey, err := rsa.GenerateKey(rand.Reader, caKeySize)
	if err != nil {
		t.Skipf("Failed to create private key: %v", err)
	}
	caCertData, err := x509.CreateCertificate(rand.Reader, caCert, caCert, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		t.Skipf("Failed to create CA certificate: %v", err)
	}
	caPEM := new(bytes.Buffer)
	if err := pem.Encode(
		caPEM,
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caCertData,
		},
	); err != nil {
		t.Skipf("Failed to encode CA cert: %v", err)
	}
	return &ca{
		t:          t,
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

func (k KeyPair) GetTLSCertificate() tls.Certificate {
	cert, err := tls.X509KeyPair(k.Certificate, k.PrivateKey)
	if err != nil {
		panic(err)
	}
	return cert
}

// CertificateAuthority provides simple access to x509 CA functions for testing purposes only.
type CertificateAuthority interface {
	// GetPEMCACert returns the CA certificate in PEM format.
	GetPEMCACert() []byte
	// CreateLocalhostServerCert creates a certificate pre-configured for "localhost", which is sufficient for most test
	// cases.
	CreateLocalhostServerCert() KeyPair
	// CreateConfiguredServerCert creates a server certificate with a specialized configuration.
	CreateConfiguredServerCert(config CertConfig) KeyPair
}

type ca struct {
	caCert     *x509.Certificate
	caCertPEM  []byte
	privateKey *rsa.PrivateKey
	serial     *big.Int
	lock       *sync.Mutex
	t          *testing.T
}

func (c *ca) GetPEMCACert() []byte {
	return c.caCertPEM
}

func (c *ca) CreateConfiguredServerCert(config CertConfig) KeyPair {
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
	certPrivKey, err := rsa.GenerateKey(rand.Reader, caKeySize)
	if err != nil {
		c.t.Skipf("Failed to generate private key: %v", err)
	}
	certBytes, err := x509.CreateCertificate(
		rand.Reader,
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
	return c.CreateConfiguredServerCert(CertConfig{
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
