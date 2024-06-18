// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const maxS3DeletionObjects = 1000

// AWSS3TestService is a specialized extension to the AWSTestServiceBase containing S3-specific functions.
type AWSS3TestService interface {
	AWSTestServiceBase

	// S3Endpoint returns the endpoint for the S3 service.
	S3Endpoint() string

	// S3Bucket returns an S3 bucket suitable for testing.
	S3Bucket() string

	// S3UsePathStyle returns true if the client should use a path-style access.
	S3UsePathStyle() bool
}

type s3ServiceFixture struct {
}

func (s s3ServiceFixture) Name() string {
	return "S3"
}

func (s s3ServiceFixture) LocalStackID() string {
	return "s3"
}

func (s s3ServiceFixture) Setup(service *awsTestService) error {
	const maxS3BucketNameLength = uint(63)
	const desiredS3BucketNameSuffixLength = uint(12)
	prefix := fmt.Sprintf(
		"opentofu-test-%s-",
		strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(service.t.Name(), "_", "-"), ":", "")),
	)
	bucketName := RandomIDPrefix(
		prefix,
		min(maxS3BucketNameLength-uint(len(prefix)), desiredS3BucketNameSuffixLength),
		CharacterRangeAlphaNumericLower,
	)

	// TODO replace with variable if the config comes from env.
	const pathStyle = true

	s3Connection := s3.NewFromConfig(service.Config(), func(options *s3.Options) {
		options.UsePathStyle = pathStyle
	})
	service.t.Logf("🌟 Creating S3 bucket %s for testing...", bucketName)
	_, err := s3Connection.CreateBucket(service.ctx, &s3.CreateBucketInput{
		Bucket: &bucketName,
	})
	bucketNeedsDeletion := true
	if err != nil {
		var bucketAlreadyExistsErr *types.BucketAlreadyExists
		if !errors.As(err, &bucketAlreadyExistsErr) {
			return fmt.Errorf("failed to create test bucket %s: %w", bucketName, err)
		}
		bucketNeedsDeletion = false
	}
	service.awsS3Parameters = awsS3Parameters{
		s3Endpoint:          service.endpoint,
		s3Bucket:            bucketName,
		s3PathStyle:         pathStyle,
		bucketNeedsDeletion: bucketNeedsDeletion,
	}
	return nil
}

func (s s3ServiceFixture) Teardown(service *awsTestService) error {
	if !service.bucketNeedsDeletion {
		return nil
	}
	cleanupContext, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	s3Connection := s3.NewFromConfig(service.Config(), func(options *s3.Options) {
		options.UsePathStyle = service.s3PathStyle
	})

	service.t.Logf("🗑️ Deleting all objects from S3 bucket %s...", service.s3Bucket)
	err := s.emptyS3Bucket(cleanupContext, service, s3Connection)
	if err != nil {
		return err
	}

	service.t.Logf("🗑️ Deleting S3 bucket %s...", service.s3Bucket)
	if _, err := s3Connection.DeleteBucket(service.ctx, &s3.DeleteBucketInput{
		Bucket: &service.s3Bucket,
	}); err != nil {
		return fmt.Errorf("failed to delete test bucket %s: %w", service.s3Bucket, err)
	}
	return nil
}

func (s s3ServiceFixture) emptyS3Bucket(cleanupContext context.Context, service *awsTestService, s3Connection *s3.Client) error {
	var marker *string
	for {
		listObjectsResult, err := s3Connection.ListObjects(cleanupContext, &s3.ListObjectsInput{
			Bucket: &service.s3Bucket,
			Marker: marker,
		})
		if err != nil {
			return fmt.Errorf("failed to clean up test bucket as the list objects call failed %s: %w", service.s3Bucket, err)
		}
		var objects []types.ObjectIdentifier
		deleteObjects := func() error {
			_, err := s3Connection.DeleteObjects(cleanupContext, &s3.DeleteObjectsInput{
				Bucket: &service.s3Bucket,
				Delete: &types.Delete{
					Objects: objects,
				},
			})
			return err
		}
		for _, object := range listObjectsResult.Contents {
			objects = append(objects, types.ObjectIdentifier{
				Key: object.Key,
			})
			if len(objects) == maxS3DeletionObjects {
				if err := deleteObjects(); err != nil {
					return fmt.Errorf("failed to clean up test bucket %s: %w", service.s3Bucket, err)
				}
			}
		}
		if len(objects) > 0 {
			if err := deleteObjects(); err != nil {
				return fmt.Errorf("failed to clean up test bucket %s: %w", service.s3Bucket, err)
			}
		}
		marker = listObjectsResult.NextMarker
		if marker == nil {
			break
		}
	}
	return nil
}

type awsS3Parameters struct {
	s3Endpoint          string
	s3Bucket            string
	s3PathStyle         bool
	bucketNeedsDeletion bool
}

func (a awsS3Parameters) S3Endpoint() string {
	return a.s3Endpoint
}

func (a awsS3Parameters) S3UsePathStyle() bool {
	return a.s3PathStyle
}

func (a awsS3Parameters) S3Bucket() string {
	return a.s3Bucket
}
