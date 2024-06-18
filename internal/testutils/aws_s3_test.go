// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/opentofu/opentofu/internal/testutils"
)

const s3TestFileName = "test.txt"
const s3TestFileContents = "Hello OpenTofu!"

func testS3Service(t *testing.T, s3TestBackend testutils.AWSS3TestService) {
	s3Connection := s3.NewFromConfig(s3TestBackend.Config(), func(options *s3.Options) {
		options.UsePathStyle = s3TestBackend.S3UsePathStyle()
	})

	t.Run("put", func(t *testing.T) {
		testS3Put(t, s3Connection, s3TestBackend)
	})
	t.Run("get", func(t *testing.T) {
		testS3Get(t, s3Connection, s3TestBackend)
	})
}

func testS3Get(t *testing.T, s3Connection *s3.Client, s3TestBackend testutils.AWSS3TestService) {
	ctx := testutils.Context(t)
	t.Logf("📂 Checking if an object can be retrieved...")
	getObjectResponse, err := s3Connection.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(s3TestBackend.S3Bucket()),
			Key:    aws.String(s3TestFileName),
		},
	)
	if err != nil {
		t.Fatalf("❌ Failed to get object (%v)", err)
	}
	defer func() {
		_ = getObjectResponse.Body.Close()
	}()
	data, err := io.ReadAll(getObjectResponse.Body)
	if err != nil {
		t.Fatalf("❌ Failed to read get object response body (%v)", err)
	}
	if string(data) != s3TestFileContents {
		t.Fatalf("❌ Incorrect test data in S3 bucket: %s", data)
	}
}

func testS3Put(t *testing.T, s3Connection *s3.Client, s3TestBackend testutils.AWSS3TestService) {
	ctx := testutils.Context(t)
	t.Logf("💾 Checking if an object can be stored...")
	if _, err := s3Connection.PutObject(
		ctx,
		&s3.PutObjectInput{
			Key:    aws.String(s3TestFileName),
			Body:   bytes.NewReader([]byte(s3TestFileContents)),
			Bucket: aws.String(s3TestBackend.S3Bucket()),
		},
	); err != nil {
		t.Fatalf("❌ Failed to put object (%v)", err)
	}
}
