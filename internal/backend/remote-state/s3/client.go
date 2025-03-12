// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	multierror "github.com/hashicorp/go-multierror"
	uuid "github.com/hashicorp/go-uuid"

	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// Store the last saved serial in dynamo with this suffix for consistency checks.
const (
	s3EncryptionAlgorithm  = "AES256"
	stateIDSuffix          = "-md5"
	lockFileSuffix         = ".tflock"
	s3ErrCodeInternalError = "InternalError"

	contentTypeJSON = "application/json"
)

type RemoteClient struct {
	s3Client              *s3.Client
	dynClient             *dynamodb.Client
	bucketName            string
	path                  string
	serverSideEncryption  bool
	customerEncryptionKey []byte
	acl                   string
	kmsKeyID              string
	ddbTable              string

	skipS3Checksum bool

	useLockfile bool
}

var (
	// The amount of time we will retry a state waiting for it to match the
	// expected checksum.
	consistencyRetryTimeout = 10 * time.Second

	// delay when polling the state
	consistencyRetryPollInterval = 2 * time.Second
)

// test hook called when checksums don't match
var testChecksumHook func()

func (c *RemoteClient) Get() (payload *remote.Payload, err error) {
	ctx := context.TODO()
	deadline := time.Now().Add(consistencyRetryTimeout)

	// If we have a checksum, and the returned payload doesn't match, we retry
	// up until deadline.
	for {
		payload, err = c.get(ctx)
		if err != nil {
			return nil, err
		}

		// If the remote state was manually removed the payload will be nil,
		// but if there's still a digest entry for that state we will still try
		// to compare the MD5 below.
		var digest []byte
		if payload != nil {
			digest = payload.MD5
		}

		// verify that this state is what we expect
		if expected, err := c.getMD5(ctx); err != nil {
			log.Printf("[WARN] failed to fetch state md5: %s", err)
		} else if len(expected) > 0 && !bytes.Equal(expected, digest) {
			log.Printf("[WARN] state md5 mismatch: expected '%x', got '%x'", expected, digest)

			if testChecksumHook != nil {
				testChecksumHook()
			}

			if time.Now().Before(deadline) {
				time.Sleep(consistencyRetryPollInterval)
				log.Println("[INFO] retrying S3 RemoteClient.Get...")
				continue
			}

			return nil, fmt.Errorf(errBadChecksumFmt, digest)
		}

		break
	}

	return payload, err
}

func (c *RemoteClient) get(ctx context.Context) (*remote.Payload, error) {
	var output *s3.GetObjectOutput
	var err error

	ctx, _ = attachLoggerToContext(ctx)

	inputHead := &s3.HeadObjectInput{
		Bucket: &c.bucketName,
		Key:    &c.path,
	}

	if c.serverSideEncryption && c.customerEncryptionKey != nil {
		inputHead.SSECustomerKey = aws.String(base64.StdEncoding.EncodeToString(c.customerEncryptionKey))
		inputHead.SSECustomerAlgorithm = aws.String(s3EncryptionAlgorithm)
		inputHead.SSECustomerKeyMD5 = aws.String(c.getSSECustomerKeyMD5())
	}

	// Head works around some s3 compatible backends not handling missing GetObject requests correctly (ex: minio Get returns Missing Bucket)
	_, err = c.s3Client.HeadObject(ctx, inputHead)
	if err != nil {
		var nb *types.NoSuchBucket
		if errors.As(err, &nb) {
			return nil, fmt.Errorf(errS3NoSuchBucket, err)
		}

		var nk *types.NotFound
		if errors.As(err, &nk) {
			return nil, nil
		}

		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: &c.bucketName,
		Key:    &c.path,
	}

	if c.serverSideEncryption && c.customerEncryptionKey != nil {
		input.SSECustomerKey = aws.String(base64.StdEncoding.EncodeToString(c.customerEncryptionKey))
		input.SSECustomerAlgorithm = aws.String(s3EncryptionAlgorithm)
		input.SSECustomerKeyMD5 = aws.String(c.getSSECustomerKeyMD5())
	}

	output, err = c.s3Client.GetObject(ctx, input)
	if err != nil {
		var nb *types.NoSuchBucket
		if errors.As(err, &nb) {
			return nil, fmt.Errorf(errS3NoSuchBucket, err)
		}

		var nk *types.NoSuchKey
		if errors.As(err, &nk) {
			return nil, nil
		}

		return nil, err
	}

	defer output.Body.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, output.Body); err != nil {
		return nil, fmt.Errorf("Failed to read remote state: %w", err)
	}

	sum := md5.Sum(buf.Bytes())
	payload := &remote.Payload{
		Data: buf.Bytes(),
		MD5:  sum[:],
	}

	// If there was no data, then return nil
	if len(payload.Data) == 0 {
		return nil, nil
	}

	return payload, nil
}

func (c *RemoteClient) Put(data []byte) error {
	contentLength := int64(len(data))

	i := &s3.PutObjectInput{
		ContentType:   aws.String(contentTypeJSON),
		ContentLength: aws.Int64(contentLength),
		Body:          bytes.NewReader(data),
		Bucket:        &c.bucketName,
		Key:           &c.path,
	}

	if !c.skipS3Checksum {
		i.ChecksumAlgorithm = types.ChecksumAlgorithmSha256

		// There is a conflict in the aws-go-sdk-v2 that prevents it from working with many s3 compatible services
		// Since we can pre-compute the hash here, we can work around it.
		// ref: https://github.com/aws/aws-sdk-go-v2/issues/1689
		algo := sha256.New()
		algo.Write(data)
		sum64str := base64.StdEncoding.EncodeToString(algo.Sum(nil))
		i.ChecksumSHA256 = &sum64str
	}

	if c.serverSideEncryption {
		if c.kmsKeyID != "" {
			i.SSEKMSKeyId = &c.kmsKeyID
			i.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		} else if c.customerEncryptionKey != nil {
			i.SSECustomerKey = aws.String(base64.StdEncoding.EncodeToString(c.customerEncryptionKey))
			i.SSECustomerAlgorithm = aws.String(string(s3EncryptionAlgorithm))
			i.SSECustomerKeyMD5 = aws.String(c.getSSECustomerKeyMD5())
		} else {
			i.ServerSideEncryption = s3EncryptionAlgorithm
		}
	}

	if c.acl != "" {
		i.ACL = types.ObjectCannedACL(c.acl)
	}

	log.Printf("[DEBUG] Uploading remote state to S3: %#v", i)

	ctx := context.TODO()
	ctx, _ = attachLoggerToContext(ctx)

	_, err := c.s3Client.PutObject(ctx, i)
	if err != nil {
		return fmt.Errorf("failed to upload state: %w", err)
	}

	sum := md5.Sum(data)
	if err := c.putMD5(ctx, sum[:]); err != nil {
		// if this errors out, we unfortunately have to error out altogether,
		// since the next Get will inevitably fail.
		return fmt.Errorf("failed to store state MD5: %w", err)

	}

	return nil
}

func (c *RemoteClient) Delete() error {
	ctx := context.TODO()
	ctx, _ = attachLoggerToContext(ctx)

	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &c.bucketName,
		Key:    &c.path,
	})

	if err != nil {
		return err
	}

	if err := c.deleteMD5(ctx); err != nil {
		log.Printf("error deleting state md5: %s", err)
	}

	return nil
}

func (c *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	if !c.IsLockingEnabled() {
		return "", nil
	}
	if info.ID == "" {
		lockID, err := uuid.GenerateUUID()
		if err != nil {
			return "", err
		}

		info.ID = lockID
	}
	info.Path = c.lockPath()

	if err := c.s3Lock(info); err != nil {
		return "", err
	}
	if err := c.dynamoDBLock(info); err != nil {
		// when the second lock fails from getting acquired, release the initially acquired one
		if uErr := c.s3Unlock(info.ID); uErr != nil {
			log.Printf("[WARN] failed to release the S3 lock after failed to acquire the dynamoDD lock: %v", uErr)
		}
		return "", err
	}
	return info.ID, nil
}

// dynamoDBLock expects the statemgr.LockInfo#ID to be filled already
func (c *RemoteClient) dynamoDBLock(info *statemgr.LockInfo) error {
	if c.ddbTable == "" {
		return nil
	}

	putParams := &dynamodb.PutItemInput{
		Item: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath()},
			"Info":   &dtypes.AttributeValueMemberS{Value: string(info.Marshal())},
		},
		TableName:           aws.String(c.ddbTable),
		ConditionExpression: aws.String("attribute_not_exists(LockID)"),
	}

	ctx := context.TODO()
	_, err := c.dynClient.PutItem(ctx, putParams)
	if err != nil {
		lockInfo, infoErr := c.getLockInfoFromDynamoDB(ctx)
		if infoErr != nil {
			err = multierror.Append(err, infoErr)
		}

		lockErr := &statemgr.LockError{
			Err:  err,
			Info: lockInfo,
		}
		return lockErr
	}

	return nil
}

// s3Lock expects the statemgr.LockInfo#ID to be filled already
func (c *RemoteClient) s3Lock(info *statemgr.LockInfo) error {
	if !c.useLockfile {
		return nil
	}

	lInfo := info.Marshal()
	putParams := &s3.PutObjectInput{
		ContentType:   aws.String(contentTypeJSON),
		ContentLength: aws.Int64(int64(len(lInfo))),
		Bucket:        aws.String(c.bucketName),
		Key:           aws.String(c.lockFilePath()),
		Body:          bytes.NewReader(lInfo),
		IfNoneMatch:   aws.String("*"),
	}

	ctx := context.TODO()
	ctx, _ = attachLoggerToContext(ctx)
	_, err := c.s3Client.PutObject(ctx, putParams)
	if err != nil {
		lockInfo, infoErr := c.getLockInfoFromS3(ctx)
		if infoErr != nil {
			err = multierror.Append(err, infoErr)
		}

		lockErr := &statemgr.LockError{
			Err:  err,
			Info: lockInfo,
		}
		return lockErr
	}

	return nil
}

func (c *RemoteClient) getMD5(ctx context.Context) ([]byte, error) {
	if c.ddbTable == "" {
		return nil, nil
	}

	getParams := &dynamodb.GetItemInput{
		Key: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath() + stateIDSuffix},
		},
		ProjectionExpression: aws.String("LockID, Digest"),
		TableName:            aws.String(c.ddbTable),
		ConsistentRead:       aws.Bool(true),
	}

	resp, err := c.dynClient.GetItem(ctx, getParams)
	if err != nil {
		return nil, err
	}

	var val string
	if v, ok := resp.Item["Digest"]; ok {
		if v, ok := v.(*dtypes.AttributeValueMemberS); ok {
			val = v.Value
		}
	}

	sum, err := hex.DecodeString(val)
	if err != nil || len(sum) != md5.Size {
		return nil, errors.New("invalid md5")
	}

	return sum, nil
}

// store the hash of the state so that clients can check for stale state files.
func (c *RemoteClient) putMD5(ctx context.Context, sum []byte) error {
	if c.ddbTable == "" {
		return nil
	}

	if len(sum) != md5.Size {
		return errors.New("invalid payload md5")
	}

	putParams := &dynamodb.PutItemInput{
		Item: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath() + stateIDSuffix},
			"Digest": &dtypes.AttributeValueMemberS{Value: hex.EncodeToString(sum)},
		},
		TableName: aws.String(c.ddbTable),
	}
	_, err := c.dynClient.PutItem(ctx, putParams)
	if err != nil {
		log.Printf("[WARN] failed to record state serial in dynamodb: %s", err)
	}

	return nil
}

// remove the hash value for a deleted state
func (c *RemoteClient) deleteMD5(ctx context.Context) error {
	if c.ddbTable == "" {
		return nil
	}

	params := &dynamodb.DeleteItemInput{
		Key: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath() + stateIDSuffix},
		},
		TableName: aws.String(c.ddbTable),
	}
	if _, err := c.dynClient.DeleteItem(ctx, params); err != nil {
		return err
	}
	return nil
}

func (c *RemoteClient) getLockInfoFromDynamoDB(ctx context.Context) (*statemgr.LockInfo, error) {
	getParams := &dynamodb.GetItemInput{
		Key: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath()},
		},
		ProjectionExpression: aws.String("LockID, Info"),
		TableName:            aws.String(c.ddbTable),
		ConsistentRead:       aws.Bool(true),
	}

	resp, err := c.dynClient.GetItem(ctx, getParams)
	if err != nil {
		return nil, err
	}

	if len(resp.Item) == 0 {
		return nil, fmt.Errorf("no lock info found for: %q within the DynamoDB table: %s", c.lockPath(), c.ddbTable)
	}

	var infoData string
	if v, ok := resp.Item["Info"]; ok {
		if v, ok := v.(*dtypes.AttributeValueMemberS); ok {
			infoData = v.Value
		}
	}

	lockInfo := &statemgr.LockInfo{}
	err = json.Unmarshal([]byte(infoData), lockInfo)
	if err != nil {
		return nil, err
	}

	return lockInfo, nil
}

func (c *RemoteClient) getLockInfoFromS3(ctx context.Context) (*statemgr.LockInfo, error) {
	getParams := &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(c.lockFilePath()),
	}

	resp, err := c.s3Client.GetObject(ctx, getParams)
	if err != nil {
		var nb *types.NoSuchBucket
		if errors.As(err, &nb) {
			return nil, fmt.Errorf(errS3NoSuchBucket, err)
		}

		return nil, err
	}

	lockInfo := &statemgr.LockInfo{}
	err = json.NewDecoder(resp.Body).Decode(lockInfo)
	if err != nil {
		return nil, fmt.Errorf("unable to json parse the lock info %q from bucket %q: %w", c.lockFilePath(), c.bucketName, err)
	}

	return lockInfo, nil
}

func (c *RemoteClient) Unlock(id string) error {
	// Attempt to release the lock from both sources.
	// We want to do so to be sure that we are leaving no locks unhandled
	s3Err := c.s3Unlock(id)
	dynamoDBErr := c.dynamoDBUnlock(id)
	switch {
	case s3Err != nil && dynamoDBErr != nil:
		s3Err.Err = multierror.Append(s3Err.Err, dynamoDBErr.Err)
		return s3Err
	case s3Err != nil:
		if c.ddbTable != "" {
			return fmt.Errorf("dynamoDB lock released but s3 failed: %w", s3Err)
		}
		return s3Err
	case dynamoDBErr != nil:
		if c.useLockfile {
			return fmt.Errorf("s3 lock released but dynamoDB failed: %w", dynamoDBErr)
		}
		return dynamoDBErr
	}
	return nil
}

func (c *RemoteClient) s3Unlock(id string) *statemgr.LockError {
	if !c.useLockfile {
		return nil
	}
	lockErr := &statemgr.LockError{}
	ctx := context.TODO()
	ctx, _ = attachLoggerToContext(ctx)

	lockInfo, err := c.getLockInfoFromS3(ctx)
	if err != nil {
		lockErr.Err = fmt.Errorf("failed to retrieve s3 lock info: %w", err)
		return lockErr
	}
	lockErr.Info = lockInfo

	if lockInfo.ID != id {
		lockErr.Err = fmt.Errorf("lock id %q from s3 does not match existing lock", id)
		return lockErr
	}

	params := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(c.lockFilePath()),
	}

	_, err = c.s3Client.DeleteObject(ctx, params)
	if err != nil {
		lockErr.Err = err
		return lockErr
	}
	return nil
}

func (c *RemoteClient) dynamoDBUnlock(id string) *statemgr.LockError {
	if c.ddbTable == "" {
		return nil
	}

	lockErr := &statemgr.LockError{}
	ctx := context.TODO()

	lockInfo, err := c.getLockInfoFromDynamoDB(ctx)
	if err != nil {
		lockErr.Err = fmt.Errorf("failed to retrieve lock info: %w", err)
		return lockErr
	}
	lockErr.Info = lockInfo

	if lockInfo.ID != id {
		lockErr.Err = fmt.Errorf("lock id %q does not match existing lock", id)
		return lockErr
	}

	// Use a condition expression to ensure both the lock info and lock ID match
	params := &dynamodb.DeleteItemInput{
		Key: map[string]dtypes.AttributeValue{
			"LockID": &dtypes.AttributeValueMemberS{Value: c.lockPath()},
		},
		TableName:           aws.String(c.ddbTable),
		ConditionExpression: aws.String("Info = :info"),
		ExpressionAttributeValues: map[string]dtypes.AttributeValue{
			":info": &dtypes.AttributeValueMemberS{Value: string(lockInfo.Marshal())},
		},
	}
	_, err = c.dynClient.DeleteItem(ctx, params)

	if err != nil {
		lockErr.Err = err
		return lockErr
	}
	return nil
}

func (c *RemoteClient) lockPath() string {
	return fmt.Sprintf("%s/%s", c.bucketName, c.path)
}

func (c *RemoteClient) getSSECustomerKeyMD5() string {
	b := md5.Sum(c.customerEncryptionKey)
	return base64.StdEncoding.EncodeToString(b[:])
}

func (c *RemoteClient) IsLockingEnabled() bool {
	return c.ddbTable != "" || c.useLockfile
}

func (c *RemoteClient) lockFilePath() string {
	return fmt.Sprintf("%s%s", c.path, lockFileSuffix)
}

const errBadChecksumFmt = `state data in S3 does not have the expected content.

This may be caused by unusually long delays in S3 processing a previous state
update.  Please wait for a minute or two and try again. If this problem
persists, and neither S3 nor DynamoDB are experiencing an outage, you may need
to manually verify the remote state and update the Digest value stored in the
DynamoDB table to the following value: %x
`

const errS3NoSuchBucket = `S3 bucket does not exist.

The referenced S3 bucket must have been previously created. If the S3 bucket
was created within the last minute, please wait for a minute or two and try
again.

Error: %w
`
