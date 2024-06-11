// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/opentofu/opentofu/internal/testutils"
)

func TestKMSService(t *testing.T) {
	ctx := testutils.Context(t)
	var kmsService testutils.AWSKMSTestService = testutils.AWS(t)
	kmsClient := kms.NewFromConfig(kmsService.ConfigV2(), func(options *kms.Options) {})
	if _, err := kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(kmsService.KMSKeyID()),
	}); err != nil {
		t.Fatalf("failed to describe KMS test key: %v", err)
	}
}
