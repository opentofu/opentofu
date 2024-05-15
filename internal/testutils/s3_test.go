// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/opentofu/opentofu/internal/testutils"
)

const s3TestFileName = "test.txt"
const s3TestFileContents = "Hello OpenTofu!"

func TestS3Service(t *testing.T) {

	s3TestBackend := testutils.S3(t)

	awsConfig := &aws.Config{
		Credentials: credentials.NewCredentials(
			&credentials.StaticProvider{
				Value: credentials.Value{
					AccessKeyID:     s3TestBackend.AccessKey(),
					SecretAccessKey: s3TestBackend.SecretKey(),
				},
			},
		),
		Endpoint:         aws.String(s3TestBackend.Endpoint()),
		Region:           aws.String(s3TestBackend.Region()),
		S3ForcePathStyle: aws.Bool(s3TestBackend.PathStyle()),
	}
	if cacert := s3TestBackend.CACert(); cacert != nil {
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
		t.Fatalf("Failed to create S3 session (%v)", err)
	}
	s3Connection := s3.New(sess)

	t.Run("put", func(t *testing.T) {
		testS3Put(t, s3Connection, s3TestBackend)
	})
	t.Run("get", func(t *testing.T) {
		testS3Get(t, s3Connection, s3TestBackend)
	})
}

func testS3Get(t *testing.T, s3Connection *s3.S3, s3TestBackend testutils.TestS3Service) {
	t.Log("Getting test object...")
	getObjectResponse, err := s3Connection.GetObject(
		&s3.GetObjectInput{
			Bucket: aws.String(s3TestBackend.Bucket()),
			Key:    aws.String(s3TestFileName),
		},
	)
	if err != nil {
		t.Fatalf("Failed to get object (%v)", err)
	}
	defer func() {
		_ = getObjectResponse.Body.Close()
	}()
	data, err := io.ReadAll(getObjectResponse.Body)
	if err != nil {
		t.Fatalf("Failed to read get object response body (%v)", err)
	}
	if string(data) != s3TestFileContents {
		t.Fatalf("Incorrect test data in S3 bucket: %s", data)
	}
}

func testS3Put(t *testing.T, s3Connection *s3.S3, s3TestBackend testutils.TestS3Service) {
	t.Log("Creating test object...")
	if _, err := s3Connection.PutObject(
		&s3.PutObjectInput{
			Key:    aws.String(s3TestFileName),
			Body:   bytes.NewReader([]byte(s3TestFileContents)),
			Bucket: aws.String(s3TestBackend.Bucket()),
		},
	); err != nil {
		t.Fatalf("Failed to put object (%v)", err)
	}
}
