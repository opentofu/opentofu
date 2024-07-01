// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

// AWSSTSTestService is a specialized extension to the AWSTestServiceBase containing STS-specific functions.
type AWSSTSTestService interface {
	AWSTestServiceBase

	// STSEndpoint returns the endpoint for the STS service.
	STSEndpoint() string
}

type stsServiceFixture struct {
}

func (s stsServiceFixture) Name() string {
	return "STS"
}

func (s stsServiceFixture) LocalStackID() string {
	return "sts"
}

func (s stsServiceFixture) Setup(service *awsTestService) error {
	service.awsSTSParameters = awsSTSParameters{
		stsEndpoint: service.endpoint,
	}
	return nil
}

func (s stsServiceFixture) Teardown(_ *awsTestService) error {
	return nil
}

type awsSTSParameters struct {
	stsEndpoint string
}

func (a awsSTSParameters) STSEndpoint() string {
	return a.stsEndpoint
}
