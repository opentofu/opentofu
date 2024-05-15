// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// CA creates an x509 CA certificate that can produce certificates for testing purposes.
func CA(t *testing.T) CertificateAuthority {
	caCert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"OpenTofu a Series of LF Projects, LLC"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
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

type CertConfig struct {
	IPAddresses []string
	Hosts       []string
	Subject     pkix.Name
	ExtKeyUsage []x509.ExtKeyUsage
}

type KeyPair struct {
	Certificate []byte
	PrivateKey  []byte
}

type CertificateAuthority interface {
	GetPEMCACert() []byte
	CreateServerCert(config CertConfig) *KeyPair
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

func (c *ca) CreateServerCert(config CertConfig) *KeyPair {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.serial.Add(c.serial, big.NewInt(1))

	extAltName := pkix.Extension{}
	extAltName.Id = asn1.ObjectIdentifier{2, 5, 29, 17}
	extAltName.Value = []byte{}

	var valueParts []string
	for _, hostname := range config.Hosts {
		valueParts = append(valueParts, fmt.Sprintf("DNS:%s", hostname))
	}
	for _, ip := range config.IPAddresses {
		valueParts = append(valueParts, fmt.Sprintf("IP:%s", ip))
	}
	extAltName.Value = []byte(strings.Join(valueParts, ", "))

	ipAddresses := make([]net.IP, len(config.IPAddresses))
	for i, ip := range config.IPAddresses {
		ipAddresses[i] = net.ParseIP(ip)
	}

	cert := &x509.Certificate{
		SerialNumber:    c.serial,
		Subject:         config.Subject,
		IPAddresses:     ipAddresses,
		NotBefore:       time.Now(),
		NotAfter:        time.Now().AddDate(0, 0, 1),
		SubjectKeyId:    []byte{1},
		ExtKeyUsage:     config.ExtKeyUsage,
		KeyUsage:        x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{extAltName},
	}
	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
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
	return &KeyPair{
		certPrivKeyPEM.Bytes(),
		certPEM.Bytes(),
	}
}
