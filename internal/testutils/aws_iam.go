// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

// AWSIAMTestService is a specialized extension to the AWSTestServiceBase containing IAM-specific functions.
type AWSIAMTestService interface {
	AWSTestServiceBase

	// IAMEndpoint returns the endpoint for the IAM service.
	IAMEndpoint() string
}

type iamServiceFixture struct {
}

func (s iamServiceFixture) Name() string {
	return "IAM"
}

func (s iamServiceFixture) LocalStackID() string {
	return "iam"
}

func (s iamServiceFixture) Setup(service *awsTestService) error {
	service.awsIAMParameters = awsIAMParameters{
		iamEndpoint: service.endpoint,
	}
	return nil
}

func (s iamServiceFixture) Teardown(_ *awsTestService) error {
	return nil
}

type awsIAMParameters struct {
	iamEndpoint string
}

func (a awsIAMParameters) IAMEndpoint() string {
	return a.iamEndpoint
}
