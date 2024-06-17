// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"testing"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestAWS(t *testing.T) {
	awsService := testutils.AWS(t)
	t.Run("DynamoDB", func(t *testing.T) {
		t.Parallel()
		testutils.SetupTestLogger(t)
		testDynamoDBService(t, awsService)
	})
	t.Run("IAM", func(t *testing.T) {
		t.Parallel()
		testutils.SetupTestLogger(t)
		testIAMService(t, awsService)
	})
	t.Run("KMS", func(t *testing.T) {
		t.Parallel()
		testutils.SetupTestLogger(t)
		testKMSService(t, awsService)
	})
	t.Run("S3", func(t *testing.T) {
		t.Parallel()
		testutils.SetupTestLogger(t)
		testS3Service(t, awsService)
	})
	t.Run("STS", func(t *testing.T) {
		t.Parallel()
		testutils.SetupTestLogger(t)
		testSTSService(t, awsService)
	})
}
