// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// AWSDynamoDBTestService is a specialized extension to the AWSTestServiceBase containing DynamoDB-specific functions.
type AWSDynamoDBTestService interface {
	AWSTestServiceBase

	// DynamoDBEndpoint returns the endpoint for the DynamoDB service.
	DynamoDBEndpoint() string

	// DynamoDBTable returns a DynamoDB table suitable for testing. This table will contain an attribute called LockID
	// with the type of String and a key for this attribute. You may or may not be able to create additional tables.
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
	const maxDynamoDBTableNameLength = uint(255)
	const desiredDynamoDBTableNameSuffixLength = uint(12)
	prefix := strings.ReplaceAll(fmt.Sprintf("opentofu-test-%s-", service.t.Name()), ":", "")
	tableName := RandomIDPrefix(
		prefix,
		min(maxDynamoDBTableNameLength-uint(len(prefix)), desiredDynamoDBTableNameSuffixLength),
		CharacterRangeAlphaLower,
	)
	dynamoDBClient := dynamodb.NewFromConfig(service.Config())

	// TODO replace with variable if the config comes from env.
	const needsTableDeletion = true

	service.t.Logf("🌟 Creating DynamoDB table %s...", tableName)

	_, err := dynamoDBClient.CreateTable(service.ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("LockID"),
			KeyType:       types.KeyTypeHash,
		}},
		TableName:   &tableName,
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return fmt.Errorf("failed to create the DynamoDB table: %w", err)
	}
	service.awsDynamoDBParameters = awsDynamoDBParameters{
		dynamoDBEndpoint:   service.endpoint,
		dynamoDBTable:      tableName,
		needsTableDeletion: needsTableDeletion,
	}
	return nil
}

func (d dynamoDBServiceFixture) Teardown(service *awsTestService) error {
	if !service.awsDynamoDBParameters.needsTableDeletion {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	dynamoDBClient := dynamodb.NewFromConfig(service.Config())
	service.t.Logf("🗑️ Deleting DynamoDB table %s...", service.dynamoDBTable)
	if _, err := dynamoDBClient.DeleteTable(cleanupCtx, &dynamodb.DeleteTableInput{
		TableName: &service.dynamoDBTable,
	}); err != nil {
		return fmt.Errorf("failed to clean up DynamoDB table %s: %w", service.dynamoDBTable, err)
	}
	return nil
}

type awsDynamoDBParameters struct {
	dynamoDBEndpoint   string
	dynamoDBTable      string
	needsTableDeletion bool
}

func (a awsDynamoDBParameters) DynamoDBEndpoint() string {
	return a.dynamoDBEndpoint
}

func (a awsDynamoDBParameters) DynamoDBTable() string {
	return a.dynamoDBTable
}
