// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

// AWSKMSTestService holds the functions to access an AWS KMS from a test service.
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
	return nil
}

func (k kmsServiceFixture) Teardown(service *awsTestService) error {
	return nil
}

type awsKMSParameters struct {
	kmsEndpoint string
	kmsKeyID    string
}

func (a awsKMSParameters) KMSEndpoint() string {
	return a.kmsEndpoint
}

func (a awsKMSParameters) KMSKeyID() string {
	return a.kmsKeyID
}
