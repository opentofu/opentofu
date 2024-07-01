// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// AWSTestServiceBase is an interface all AWS-related test services should embed.
type AWSTestServiceBase interface {
	// AccessKey returns the access key to use for authentication.
	AccessKey() string

	// SecretKey returns the secret key to use for authentication.
	SecretKey() string

	// Region returns the AWS region to use.
	Region() string

	// CACert returns the CA certificate the service will present. This may be empty if the endpoint is not an
	// https endpoint.
	CACert() []byte

	// CACertFile returns a file on the local disk containing the CA certificate.
	CACertFile() string

	// Config creates an AWS Go SDK v2-compatible configuration.
	Config() awsv2.Config
}

// AWSTestService is a top interface for all AWS-related test services.
type AWSTestService interface {
	AWSIAMTestService
	AWSSTSTestService
	AWSS3TestService
	AWSDynamoDBTestService
	AWSKMSTestService
}

// AWS creates a locally-emulated AWS stack or attaches to an already-existing AWS configuration from the environment.
//
// Important: if you are using this tool, please make sure to also use testutils.Context in order to give this tool
// enough time to tear down the test infrastructure.
func AWS(t *testing.T) AWSTestService {
	return newAWSTestService(t, []awsServiceFixture{
		&iamServiceFixture{},
		&stsServiceFixture{},
		&s3ServiceFixture{},
		&dynamoDBServiceFixture{},
		&kmsServiceFixture{},
	})
}

func newAWSTestService(t *testing.T, services []awsServiceFixture) AWSTestService {
	t.Logf("🚧 Configuring AWS test service...")
	ctx := Context(t)

	ca := CA(t)
	pair := ca.CreateLocalhostServerCert()
	tempDir := t.TempDir()
	if err := os.WriteFile(path.Join(tempDir, "server.pem"), append(pair.Certificate, pair.PrivateKey...), permAll); err != nil {
		t.Skipf("Cannot write to test directory %s: %v", tempDir, err)
	}

	caCertFile := path.Join(tempDir, "cacert.pem")
	if err := os.WriteFile(caCertFile, ca.GetPEMCACert(), permAll); err != nil {
		t.Skipf("Cannot write to test directory %s: %v", tempDir, err)
	}

	var ids []string
	for _, service := range services {
		ids = append(ids, service.LocalStackID())
	}

	const localStackPort = 4566
	natPort := fmt.Sprintf("%d/tcp", localStackPort)

	request := testcontainers.ContainerRequest{
		HostAccessPorts: nil,
		Image:           "localstack/localstack",
		Env: map[string]string{
			"LOCALSTACK_HOST": fmt.Sprintf("localhost:%d", localStackPort),
			"SERVICES":        strings.Join(ids, ","),
			// Eager loading is on because we need to provision test fixtures anyway.
			"EAGER_SERVICE_LOADING":  "1",
			"SKIP_SSL_CERT_DOWNLOAD": "1",
			"CUSTOM_SSL_CERT_PATH":   "/opt/certs/server.pem",
		},
		ExposedPorts: []string{
			natPort,
		},
		Name: t.Name(),
		HostConfigModifier: func(config *container.HostConfig) {
			config.Binds = []string{
				fmt.Sprintf("%s:/opt/certs", tempDir),
				"/var/run/docker.sock:/var/run/docker.sock",
			}
		},
		WaitingFor: wait.ForLog("Ready."),
	}
	localStackContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
		Logger:           newAdapter(t),
	})
	if err != nil {
		t.Skipf("❌ Failed to start LocalStack backend: %v", err)
		return nil
	}
	t.Cleanup(func() {
		if err := localStackContainer.Terminate(ctx); err != nil { //nolint:govet // This shadowing of err is intentional
			t.Logf("❌ Failed to stop LocalStack container %s: %v", localStackContainer.GetContainerID(), err)
		}
	})

	mappedPort, err := localStackContainer.MappedPort(ctx, nat.Port(natPort))
	if err != nil {
		t.Skipf("❌ Failed to get mapped port for LocalStack instance (%v)", err)
	}
	host, err := localStackContainer.Host(ctx)
	if err != nil {
		t.Skipf("❌ Failed to get host for LocalStack instance (%v)", err)
	}

	svc := &awsTestService{
		t:           t,
		ctx:         ctx,
		ca:          ca,
		caCertFile:  caCertFile,
		endpoint:    "https://" + host + ":" + strconv.Itoa(mappedPort.Int()),
		region:      "us-east-1",
		accessKeyID: "test",
		secretKeyID: "test",
	}
	for _, service := range services {
		service := service
		if err := service.Setup(svc); err != nil {
			t.Skipf("❌ Failed to initialize %s: %v", service.Name(), err)
			return nil
		}
		t.Cleanup(func() {
			if err := service.Teardown(svc); err != nil {
				t.Errorf("❌ Failed to tear down service %s: %v", service.Name(), err)
			}
		})
	}
	t.Logf("✅ AWS test service is ready for use.")
	return svc
}

type awsServiceFixture interface {
	Name() string
	LocalStackID() string
	Setup(service *awsTestService) error
	Teardown(service *awsTestService) error
}

type awsTestService struct {
	t           *testing.T
	ctx         context.Context
	ca          CertificateAuthority
	caCertFile  string
	endpoint    string
	region      string
	accessKeyID string
	secretKeyID string

	awsIAMParameters
	awsSTSParameters
	awsS3Parameters
	awsDynamoDBParameters
	awsKMSParameters
}

func (a awsTestService) CACertFile() string {
	return a.caCertFile
}

func (a awsTestService) ConfigV1() awsv1.Config {
	return awsv1.Config{
		Credentials: credentials.NewCredentials(
			&credentials.StaticProvider{
				Value: credentials.Value{
					AccessKeyID:     a.accessKeyID,
					SecretAccessKey: a.secretKeyID,
				},
			},
		),
		Endpoint:         awsv1.String(a.endpoint),
		Region:           awsv1.String(a.region),
		S3ForcePathStyle: awsv1.Bool(a.s3PathStyle),
	}
}

func (a awsTestService) Config() awsv2.Config {
	return awsv2.Config{
		Region: a.region,
		Credentials: awsv2.CredentialsProviderFunc(func(_ context.Context) (awsv2.Credentials, error) {
			return awsv2.Credentials{
				AccessKeyID:     a.accessKeyID,
				SecretAccessKey: a.secretKeyID,
			}, nil
		}),
		BaseEndpoint: awsv2.String(a.endpoint),
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: a.ca.GetClientTLSConfig(),
			},
		},
	}
}

func (a awsTestService) AccessKey() string {
	return a.accessKeyID
}

func (a awsTestService) SecretKey() string {
	return a.secretKeyID
}

func (a awsTestService) Region() string {
	return a.region
}

func (a awsTestService) CACert() []byte {
	return a.ca.GetPEMCACert()
}
