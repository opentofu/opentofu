// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

type concurrentSigningStore struct {
	active  atomic.Int32
	maximum atomic.Int32
	calls   atomic.Int32
	entered chan struct{}
	release <-chan struct{}
}

func (s *concurrentSigningStore) sign() {
	active := s.active.Add(1)
	s.calls.Add(1)
	for previous := s.maximum.Load(); active > previous; previous = s.maximum.Load() {
		if s.maximum.CompareAndSwap(previous, active) {
			break
		}
	}
	if s.entered != nil {
		s.entered <- struct{}{}
	}
	if s.release != nil {
		<-s.release
	} else {
		// Model signing work that can overlap when distinct workspace clients
		// share the SDK. The assertion concerns simultaneous calls, not timing.
		time.Sleep(time.Millisecond)
	}
	s.active.Add(-1)
}

func (s *concurrentSigningStore) GetObject(context.Context, objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error) {
	s.sign()
	return objectstorage.GetObjectResponse{Content: io.NopCloser(strings.NewReader("state"))}, nil
}
func (s *concurrentSigningStore) PutObject(context.Context, objectstorage.PutObjectRequest) (objectstorage.PutObjectResponse, error) {
	s.sign()
	return objectstorage.PutObjectResponse{}, nil
}
func (s *concurrentSigningStore) DeleteObject(context.Context, objectstorage.DeleteObjectRequest) (objectstorage.DeleteObjectResponse, error) {
	s.sign()
	return objectstorage.DeleteObjectResponse{}, nil
}
func (s *concurrentSigningStore) ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	s.sign()
	return objectstorage.ListObjectsResponse{}, nil
}

func callSerializedOperation(ctx context.Context, client objectStorageClient, operation int) error {
	switch operation % 4 {
	case 0:
		response, err := client.GetObject(ctx, objectstorage.GetObjectRequest{})
		if response.Content != nil {
			response.Content.Close()
		}
		return err
	case 1:
		_, err := client.PutObject(ctx, objectstorage.PutObjectRequest{})
		return err
	case 2:
		_, err := client.DeleteObject(ctx, objectstorage.DeleteObjectRequest{})
		return err
	default:
		_, err := client.ListObjects(ctx, objectstorage.ListObjectsRequest{})
		return err
	}
}

func TestSDKClientSerializesMixedWorkspaceOperations(t *testing.T) {
	store := &concurrentSigningStore{}
	var factoryCalls atomic.Int32
	client := &serializedObjectStorageClient{newClient: func() objectStorageClient { factoryCalls.Add(1); return store }}
	const calls = 32
	start := make(chan struct{})
	results := make(chan error, calls)
	var workers sync.WaitGroup
	for operation := 0; operation < calls; operation++ {
		workers.Add(1)
		go func(operation int) {
			defer workers.Done()
			<-start
			results <- callSerializedOperation(t.Context(), client, operation)
		}(operation)
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if store.calls.Load() != calls || store.maximum.Load() != 1 || factoryCalls.Load() != calls {
		t.Fatalf("signing calls %d; concurrent calls %d", store.calls.Load(), store.maximum.Load())
	}
}

func TestSDKClientCanceledWhileWaitingDoesNotDelegate(t *testing.T) {
	for operation := 0; operation < 4; operation++ {
		t.Run([]string{"get", "put", "delete", "list"}[operation], func(t *testing.T) {
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseFirst()
			store := &concurrentSigningStore{entered: make(chan struct{}, 2), release: release}
			client := &serializedObjectStorageClient{client: store}
			first := make(chan error, 1)
			go func() { first <- callSerializedOperation(t.Context(), client, 0) }()
			<-store.entered
			ctx, cancel := context.WithCancel(t.Context())
			second := make(chan error, 1)
			waiting := make(chan struct{})
			go func() { close(waiting); second <- callSerializedOperation(ctx, client, operation) }()
			<-waiting
			cancel()
			select {
			case err := <-second:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("waiting cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled waiter did not return while the preceding SDK operation remained blocked")
			}
			if store.calls.Load() != 1 {
				t.Fatal("canceled waiter invoked the SDK")
			}
			releaseFirst()
			if err := <-first; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Model the SDK's behavior of returning a canceled operation while its signing
// goroutine continues. Every operation must retain a distinct factory instance.
type continuingSDKClient struct {
	objectStorageClient
	requestID       int
	started         chan struct{}
	continueSigning <-chan struct{}
	finished        chan<- int
}

func (c *continuingSDKClient) GetObject(ctx context.Context, _ objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error) {
	if c.requestID == 1 {
		go func() { <-c.continueSigning; c.finished <- c.requestID }()
		close(c.started)
		<-ctx.Done()
		return objectstorage.GetObjectResponse{}, ctx.Err()
	}
	return objectstorage.GetObjectResponse{Content: io.NopCloser(strings.NewReader("second request"))}, nil
}

func TestSDKClientFactoryIsolatesContinuingCanceledOperation(t *testing.T) {
	started := make(chan struct{})
	continueSigning := make(chan struct{})
	finished := make(chan int, 1)
	var instances []*continuingSDKClient
	adapter := &serializedObjectStorageClient{newClient: func() objectStorageClient {
		client := &continuingSDKClient{requestID: len(instances) + 1, started: started, continueSigning: continueSigning, finished: finished}
		instances = append(instances, client)
		return client
	}}
	ctx, cancel := context.WithCancel(t.Context())
	first := make(chan error, 1)
	go func() { _, err := adapter.GetObject(ctx, objectstorage.GetObjectRequest{}); first <- err }()
	<-started
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first operation: %v", err)
	}
	response, err := adapter.GetObject(t.Context(), objectstorage.GetObjectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Content.Close()
	if len(instances) != 2 || instances[0] == instances[1] || instances[0].requestID != 1 || instances[1].requestID != 2 {
		t.Fatal("each SDK operation must own a distinct factory instance")
	}
	close(continueSigning)
	if id := <-finished; id != 1 {
		t.Fatal("a continuing canceled operation lost its request-specific identity")
	}
	// A call canceled before entering the gate must not create another signer.
	if _, err := adapter.GetObject(ctx, objectstorage.GetObjectRequest{}); !errors.Is(err, context.Canceled) || len(instances) != 2 {
		t.Fatal("canceled call invoked the operation factory")
	}
}
