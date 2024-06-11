// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// AWSKMSTestService is a specialized extension to the AWSTestServiceBase containing KMS-specific functions.
type AWSKMSTestService interface {
	AWSTestServiceBase

	// KMSEndpoint returns the endpoint for the KMS service.
	KMSEndpoint() string

	// KMSKeyID returns a key ID suitable for testing.
	KMSKeyID() string
}

type kmsServiceFixture struct {
}

func (k kmsServiceFixture) Name() string {
	return "KMS"
}

func (k kmsServiceFixture) LocalStackID() string {
	return "kms"
}

func (k kmsServiceFixture) Setup(service *awsTestService) error {
	kmsClient := kms.NewFromConfig(service.ConfigV2())

	// TODO replace with variable if the config comes from env.
	const needsKeyDeletion = true

	key, err := kmsClient.CreateKey(service.ctx, &kms.CreateKeyInput{})
	if err != nil {
		return fmt.Errorf("failed to create the KMS key: %w", err)
	}
	service.awsKMSParameters = awsKMSParameters{
		kmsEndpoint:      service.endpoint,
		kmsKeyID:         *key.KeyMetadata.KeyId,
		needsKeyDeletion: needsKeyDeletion,
	}
	return nil
}

func (k kmsServiceFixture) Teardown(service *awsTestService) error {
	if !service.needsKeyDeletion {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	kmsClient := kms.NewFromConfig(service.ConfigV2())
	if _, err := kmsClient.ScheduleKeyDeletion(cleanupCtx, &kms.ScheduleKeyDeletionInput{
		KeyId:               &service.kmsKeyID,
		PendingWindowInDays: aws.Int32(7),
	}); err != nil {
		return fmt.Errorf("failed to clean up KMS key ID %s: %w", service.kmsKeyID, err)
	}
	return nil
}

type awsKMSParameters struct {
	kmsEndpoint      string
	kmsKeyID         string
	needsKeyDeletion bool
}

func (a awsKMSParameters) KMSEndpoint() string {
	return a.kmsEndpoint
}

func (a awsKMSParameters) KMSKeyID() string {
	return a.kmsKeyID
}
