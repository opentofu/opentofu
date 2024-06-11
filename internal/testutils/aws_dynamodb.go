// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

// AWSDynamoDBTestService is a specialized extension to the AWSTestServiceBase containing DynamoDB-specific functions.
type AWSDynamoDBTestService interface {
	AWSTestServiceBase

	// DynamoDBEndpoint returns the endpoint for the DynamoDB service.
	DynamoDBEndpoint() string

	// DynamoDBTable returns a DynamoDB table suitable for testing.
	DynamoDBTable() string
}

type dynamoDBServiceFixture struct {
}

func (d dynamoDBServiceFixture) Name() string {
	return "DynamoDB"
}

func (d dynamoDBServiceFixture) LocalStackID() string {
	return "dynamodb"
}

func (d dynamoDBServiceFixture) Setup(service *awsTestService) error {
	return nil
}

func (d dynamoDBServiceFixture) Teardown(service *awsTestService) error {
	return nil
}

type awsDynamoDBParameters struct {
	dynamoDBEndpoint string
	dynamoDBTable    string
}

func (a awsDynamoDBParameters) DynamoDBEndpoint() string {
	return a.dynamoDBEndpoint
}

func (a awsDynamoDBParameters) DynamoDBTable() string {
	return a.dynamoDBTable
}
