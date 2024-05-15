// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestS3Service interface {

	// Endpoint returns the endpoint URL for the S3 access.
	Endpoint() string

	// AccessKey returns the access key to use for authentication.
	AccessKey() string

	// SecretKey returns the secret key to use for authentication.
	SecretKey() string

	// Region returns the AWS region to use.
	Region() string

	// CACert returns the CA certificate the service will present. This may be empty if the endpoint is not an
	// https endpoint.
	CACert() []byte

	// PathStyle returns true if path-style access should be used.
	PathStyle() bool

	// Bucket returns the name of the S3 bucket that can be used for testing.
	Bucket() string
}

// S3 returns a TestS3Service for use in a single test case and cleans up the test case afterwards. If no facility is
// available for running the S3 backend, the test will be skipped.
func S3(t *testing.T) TestS3Service {
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel func()
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}

	ca := CA(t)
	pair := ca.CreateServerCert(CertConfig{
		IPAddresses: []string{},
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"OpenTofu a Series of LF Projects, LLC"},
			CommonName:   "localhost",
		},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		Hosts:       []string{},
	})

	accessKey := "test"
	secretKey := "testtest"
	bucket := "test"

	tempDir := t.TempDir()
	if err := os.WriteFile(path.Join(tempDir, "private.key"), pair.Certificate, 0777); err != nil {
		t.Skipf("Cannot write to test directory %s: %v", tempDir, err)
	}
	if err := os.WriteFile(path.Join(tempDir, "public.crt"), pair.PrivateKey, 0777); err != nil {
		t.Skipf("Cannot write to test directory %s: %v", tempDir, err)
	}

	request := testcontainers.ContainerRequest{
		HostAccessPorts:   nil,
		Image:             "docker.io/minio/minio",
		ImageSubstitutors: nil,
		Entrypoint:        nil,
		Env: map[string]string{
			"MINIO_ROOT_USER":     accessKey,
			"MINIO_ROOT_PASSWORD": secretKey,
		},
		ExposedPorts: []string{
			"9000/tcp",
		},
		Cmd: []string{
			"server", "/data", "--console-address", ":9001",
			"--certs-dir", "/opt/certs",
		},
		Name: t.Name(),
		HostConfigModifier: func(config *container.HostConfig) {
			config.Binds = []string{fmt.Sprintf("%s:/opt/certs", tempDir)}
		},
		WaitingFor: &s3Strategy{
			t:         t,
			accessKey: accessKey,
			secretKey: secretKey,
			caCert:    ca.GetPEMCACert(),
		},
	}
	minioContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	if err != nil {
		t.Skipf("Failed to start Minio backend: %v", err)
		return nil
	}
	t.Cleanup(func() {
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to stop Minio container %s: %v", minioContainer.GetContainerID(), err)
		}
	})

	endpoint, err := minioContainer.Endpoint(ctx, "")
	if err != nil {
		t.Skipf("Failed to get port endpoint: %v", err)
	}

	return &s3TestBackend{
		endpoint:  endpoint,
		cacert:    ca.GetPEMCACert(),
		accessKey: accessKey,
		secretKey: secretKey,
		region:    "us-east-1",
		pathStyle: true,
		bucket:    bucket,
	}
}

type s3Strategy struct {
	t         *testing.T
	accessKey string
	secretKey string
	caCert    []byte
}

func (s *s3Strategy) String() string {
	return "S3 backend to come up"
}

func (s *s3Strategy) WaitUntilReady(ctx context.Context, target wait.StrategyTarget) error {
	mappedPort, err := target.MappedPort(ctx, "9000/tcp")
	if err != nil {
		return err
	}
	port := mappedPort.Port()
	host, err := target.Host(ctx)
	if err != nil {
		return err
	}

	tries := 0
	sleepTime := 3
	for {
		if tries > 30 {
			return fmt.Errorf("backend failed to come up in %d seconds", sleepTime*10)
		}
		awsConfig := &aws.Config{
			Credentials: credentials.NewCredentials(
				&credentials.StaticProvider{
					Value: credentials.Value{
						AccessKeyID:     s.accessKey,
						SecretAccessKey: s.secretKey,
					},
				},
			),
			Endpoint:         aws.String(fmt.Sprintf("https://%s:%s", host, port)),
			Region:           aws.String("us-east-1"),
			S3ForcePathStyle: aws.Bool(true),
		}
		if cacert := s.caCert; cacert != nil {
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(cacert)
			awsConfig.WithHTTPClient(&http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs: certPool,
					},
				},
			})
		}
		sess, err := session.NewSession(awsConfig)
		if err != nil {
			s.t.Logf("Failed to create S3 session, S3 backend is not yet up (%v)", err)
		} else {
			s3Connection := s3.New(sess)
			if _, err := s3Connection.ListBuckets(&s3.ListBucketsInput{}); err != nil {
				s.t.Logf("ListBuckets call failed, S3 backend is not yet up (%v)", err)
			} else {
				return nil
			}
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
		tries++
	}
}

type s3TestBackend struct {
	endpoint  string
	cacert    []byte
	accessKey string
	secretKey string
	region    string
	pathStyle bool
	bucket    string
}

func (s *s3TestBackend) Endpoint() string {
	if len(s.cacert) != 0 {
		return fmt.Sprintf("https://%s/", s.endpoint)
	}
	//goland:noinspection HttpUrlsUsage
	return fmt.Sprintf("http://%s/", s.endpoint)
}

func (s *s3TestBackend) AccessKey() string {
	return s.accessKey
}

func (s *s3TestBackend) SecretKey() string {
	return s.secretKey
}

func (s *s3TestBackend) Region() string {
	return s.region
}

func (s *s3TestBackend) CACert() []byte {
	return s.cacert
}

func (s *s3TestBackend) PathStyle() bool {
	return s.pathStyle
}

func (s *s3TestBackend) Bucket() string {
	return s.bucket
}
