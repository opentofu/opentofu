// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package kubernetes

////////////////////////////////////////////////////////////////////////////
// Most of the tests in this package want to interact with a real Kubernetes
// API server.
//
// One way to achieve that is to run minikube: https://minikube.sigs.k8s.io/
//
// Starting a cluster with minikube will, by default, write a "minikube"
// context into ~/.kube/config, and so you can set the following environment
// variable when running the tests to use the minikube cluster:
//    KUBE_CONFIG_PATHS=$HOME/.kube/config
//
// If that configuration file contains multiple "contexts" on your system,
// you can explicitly select the one added by minikube by setting:
//    KUBE_CTX=minikube
//
// As usual with acceptance tests in OpenTofu, you will also need to set
// either TF_ACC=1 or TF_K8S_TEST=1 to confirm that you want the test suite
// to try to connect to a Kubernetes cluster.
////////////////////////////////////////////////////////////////////////////

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sSchema "k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

const (
	secretSuffix = "test-state"
)

var namespace string

// verify that we are doing ACC tests or the k8s tests specifically
func testACC(t *testing.T) {
	skip := os.Getenv("TF_ACC") == "" && os.Getenv("TF_K8S_TEST") == ""
	if skip {
		t.Log("k8s backend tests require setting TF_ACC or TF_K8S_TEST")
		t.Skip()
	}

	ns := os.Getenv("KUBE_NAMESPACE")

	if ns != "" {
		namespace = ns
	} else {
		namespace = "default"
	}

	cleanupK8sResources(t)
}

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestBackend(t *testing.T) {
	testACC(t)
	defer cleanupK8sResources(t)

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"secret_suffix": secretSuffix,
	}))

	// Test
	backend.TestBackendStates(t, b1)
}

func TestBackendLocks(t *testing.T) {
	testACC(t)
	defer cleanupK8sResources(t)

	// Get the backend. We need two to test locking.
	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"secret_suffix": secretSuffix,
	}))

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"secret_suffix": secretSuffix,
	}))

	// Test
	backend.TestBackendStateLocks(t, b1, b2)
	backend.TestBackendStateForceUnlock(t, b1, b2)
}

func TestBackendLocksSoak(t *testing.T) {
	testACC(t)
	defer cleanupK8sResources(t)

	clientCount := 100
	lockAttempts := 100

	lockers := []statemgr.Locker{}
	for i := 0; i < clientCount; i++ {
		b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
			"secret_suffix": secretSuffix,
		}))

		s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
		if err != nil {
			t.Fatalf("Error creating state manager: %v", err)
		}

		lockers = append(lockers, s.(statemgr.Locker))
	}

	wg := sync.WaitGroup{}
	for i, l := range lockers {
		wg.Add(1)
		go func(locker statemgr.Locker, n int) {
			defer wg.Done()

			li := statemgr.NewLockInfo()
			li.Operation = "test"
			li.Who = fmt.Sprintf("client-%v", n)

			for i := 0; i < lockAttempts; i++ {
				id, err := locker.Lock(t.Context(), li)
				if err != nil {
					continue
				}

				// hold onto the lock for a little bit
				time.Sleep(time.Duration(rand.Intn(10)) * time.Microsecond)

				err = locker.Unlock(t.Context(), id)
				if err != nil {
					t.Errorf("failed to unlock: %v", err)
				}
			}
		}(l, i)
	}

	wg.Wait()
}

func cleanupK8sResources(t *testing.T) {
	ctx := context.Background()
	// Get a backend to use the k8s client
	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"secret_suffix": secretSuffix,
	}))

	b := b1.(*Backend)

	sClient, err := b.getKubernetesSecretClient()
	if err != nil {
		t.Fatal(err)
	}

	// Delete secrets
	opts := metav1.ListOptions{LabelSelector: tfstateKey + "=true"}
	secrets, err := sClient.List(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}

	delProp := metav1.DeletePropagationBackground
	delOps := metav1.DeleteOptions{PropagationPolicy: &delProp}
	var errs []error

	for _, secret := range secrets.Items {
		labels := secret.GetLabels()
		key, ok := labels[tfstateSecretSuffixKey]
		if !ok {
			continue
		}

		if key == secretSuffix {
			err = sClient.Delete(ctx, secret.GetName(), delOps)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	leaseClient, err := b.getKubernetesLeaseClient()
	if err != nil {
		t.Fatal(err)
	}

	// Delete leases
	leases, err := leaseClient.List(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}

	for _, lease := range leases.Items {
		labels := lease.GetLabels()
		key, ok := labels[tfstateSecretSuffixKey]
		if !ok {
			continue
		}

		if key == secretSuffix {
			err = leaseClient.Delete(ctx, lease.GetName(), delOps)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		t.Fatal(errs)
	}
}

func newTestFakeDynamicClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[k8sSchema.GroupVersionResource]string{
			secretResource: "SecretList",
		},
	)
}

func TestBackend_Workspaces_forbidden(t *testing.T) {
	fakeClient := newTestFakeDynamicClient()
	fakeClient.PrependReactor("list", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			k8sSchema.GroupResource{Group: "", Resource: "secrets"},
			"",
			errors.New("cannot list resource \"secrets\" in API group \"\" in the namespace \"default\""),
		)
	})

	b := &Backend{
		kubernetesSecretClient: fakeClient.Resource(secretResource).Namespace("default"),
		nameSuffix:             "test-state",
		namespace:              "default",
	}

	workspaces, err := b.Workspaces(context.Background())
	if err != nil {
		t.Fatalf("expected no error on forbidden list, got: %v", err)
	}

	expected := []string{backend.DefaultStateName}
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("expected workspaces %v, got %v", expected, workspaces)
	}
}

func TestBackend_Workspaces_error(t *testing.T) {
	fakeClient := newTestFakeDynamicClient()
	expectedErr := apierrors.NewInternalError(errors.New("internal server error"))
	fakeClient.PrependReactor("list", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, expectedErr
	})

	b := &Backend{
		kubernetesSecretClient: fakeClient.Resource(secretResource).Namespace("default"),
		nameSuffix:             "test-state",
		namespace:              "default",
	}

	workspaces, err := b.Workspaces(context.Background())
	if err == nil {
		t.Fatalf("expected error on internal server error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got: %v", expectedErr, err)
	}
	if workspaces != nil {
		t.Fatalf("expected nil workspaces on error, got: %v", workspaces)
	}
}

func TestBackend_Workspaces_genericError(t *testing.T) {
	fakeClient := newTestFakeDynamicClient()
	expectedErr := errors.New("connection refused")
	fakeClient.PrependReactor("list", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, expectedErr
	})

	b := &Backend{
		kubernetesSecretClient: fakeClient.Resource(secretResource).Namespace("default"),
		nameSuffix:             "test-state",
		namespace:              "default",
	}

	workspaces, err := b.Workspaces(context.Background())
	if err == nil {
		t.Fatalf("expected error on connection refused, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got: %v", expectedErr, err)
	}
	if workspaces != nil {
		t.Fatalf("expected nil workspaces on error, got: %v", workspaces)
	}
}

func TestBackend_Workspaces_success(t *testing.T) {
	fakeClient := newTestFakeDynamicClient()

	secretList := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "SecretList",
		},
		Items: []unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "tfstate-default-test-state",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:             "true",
							tfstateWorkspaceKey:    backend.DefaultStateName,
							tfstateSecretSuffixKey: "test-state",
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "tfstate-prod-test-state",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:             "true",
							tfstateWorkspaceKey:    "prod",
							tfstateSecretSuffixKey: "test-state",
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "tfstate-stage-test-state",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:             "true",
							tfstateWorkspaceKey:    "stage",
							tfstateSecretSuffixKey: "test-state",
						},
					},
				},
			},
			// Secret with different suffix - must be ignored
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "tfstate-other-other-suffix",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:             "true",
							tfstateWorkspaceKey:    "other",
							tfstateSecretSuffixKey: "other-suffix",
						},
					},
				},
			},
			// Secret missing workspace label - must be ignored
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "unrelated-secret",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:             "true",
							tfstateSecretSuffixKey: "test-state",
						},
					},
				},
			},
			// Secret missing suffix label - must be ignored
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "no-suffix-secret",
						"namespace": "default",
						"labels": map[string]interface{}{
							tfstateKey:          "true",
							tfstateWorkspaceKey: "dev",
						},
					},
				},
			},
		},
	}

	fakeClient.PrependReactor("list", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, secretList, nil
	})

	b := &Backend{
		kubernetesSecretClient: fakeClient.Resource(secretResource).Namespace("default"),
		nameSuffix:             "test-state",
		namespace:              "default",
	}

	workspaces, err := b.Workspaces(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expected := []string{"default", "prod", "stage"}
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("expected workspaces %v, got %v", expected, workspaces)
	}
}

func TestBackend_Workspaces_emptyList(t *testing.T) {
	fakeClient := newTestFakeDynamicClient()

	emptyList := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "SecretList",
		},
		Items: []unstructured.Unstructured{},
	}

	fakeClient.PrependReactor("list", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, emptyList, nil
	})

	b := &Backend{
		kubernetesSecretClient: fakeClient.Resource(secretResource).Namespace("default"),
		nameSuffix:             "test-state",
		namespace:              "default",
	}

	workspaces, err := b.Workspaces(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expected := []string{backend.DefaultStateName}
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("expected workspaces %v, got %v", expected, workspaces)
	}
}

