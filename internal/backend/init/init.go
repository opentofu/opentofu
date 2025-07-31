// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package init contains the list of backends that can be initialized and
// basic helper functions for initializing those backends.
package init

import (
	"sync"

	"github.com/opentofu/svchost/disco"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/backend"
	backendLocal "github.com/opentofu/opentofu/internal/backend/local"
	backendRemote "github.com/opentofu/opentofu/internal/backend/remote"
	backendAzure "github.com/opentofu/opentofu/internal/backend/remote-state/azure"
	backendConsul "github.com/opentofu/opentofu/internal/backend/remote-state/consul"
	backendCos "github.com/opentofu/opentofu/internal/backend/remote-state/cos"
	backendGCS "github.com/opentofu/opentofu/internal/backend/remote-state/gcs"
	backendHTTP "github.com/opentofu/opentofu/internal/backend/remote-state/http"
	backendInmem "github.com/opentofu/opentofu/internal/backend/remote-state/inmem"
	backendKubernetes "github.com/opentofu/opentofu/internal/backend/remote-state/kubernetes"
	backendOSS "github.com/opentofu/opentofu/internal/backend/remote-state/oss"
	backendPg "github.com/opentofu/opentofu/internal/backend/remote-state/pg"
	backendS3 "github.com/opentofu/opentofu/internal/backend/remote-state/s3"
	backendCloud "github.com/opentofu/opentofu/internal/cloud"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// backends is the list of available backends. This is a global variable
// because backends are currently hardcoded into OpenTofu and can't be
// modified without recompilation.
//
// To read an available backend, use the Backend function. This ensures
// safe concurrent read access to the list of built-in backends.
//
// Backends are hardcoded into OpenTofu because the API for backends uses
// complex structures and supporting that over the plugin system is currently
// prohibitively difficult. For those wanting to implement a custom backend,
// they can do so with recompilation.
var backends map[string]backend.InitFn
var backendsLock sync.Mutex

// RemovedBackends is a record of previously supported backends which have
// since been deprecated and removed.
var RemovedBackends map[string]string

// Init initializes the backends map with all our hardcoded backends.
func Init(services *disco.Disco) {
	backendsLock.Lock()
	defer backendsLock.Unlock()

	// NOTE: Underscore-prefixed named are reserved for unit testing use via
	// the RegisterTemp function. Do not add any underscore-prefixed names
	// to the following table.

	backends = map[string]backend.InitFn{
		"local":  func(enc encryption.StateEncryption) backend.Backend { return backendLocal.New(enc) },
		"remote": func(enc encryption.StateEncryption) backend.Backend { return backendRemote.New(services, enc) },

		// Remote State backends.
		"azurerm":    func(enc encryption.StateEncryption) backend.Backend { return backendAzure.New(enc) },
		"consul":     func(enc encryption.StateEncryption) backend.Backend { return backendConsul.New(enc) },
		"cos":        func(enc encryption.StateEncryption) backend.Backend { return backendCos.New(enc) },
		"gcs":        func(enc encryption.StateEncryption) backend.Backend { return backendGCS.New(enc) },
		"http":       func(enc encryption.StateEncryption) backend.Backend { return backendHTTP.New(enc) },
		"inmem":      func(enc encryption.StateEncryption) backend.Backend { return backendInmem.New(enc) },
		"kubernetes": func(enc encryption.StateEncryption) backend.Backend { return backendKubernetes.New(enc) },
		"oss":        func(enc encryption.StateEncryption) backend.Backend { return backendOSS.New(enc) },
		"pg":         func(enc encryption.StateEncryption) backend.Backend { return backendPg.New(enc) },
		"s3":         func(enc encryption.StateEncryption) backend.Backend { return backendS3.New(enc) },

		// Terraform Cloud 'backend'
		// This is an implementation detail only, used for the cloud package
		"cloud": func(enc encryption.StateEncryption) backend.Backend { return backendCloud.New(services, enc) },
	}

	RemovedBackends = map[string]string{
		"artifactory": `The "artifactory" backend is not supported in OpenTofu v1.3 or later.`,
		"azure":       `The "azure" backend name has been removed, please use "azurerm".`,
		"etcd":        `The "etcd" backend is not supported in OpenTofu v1.3 or later.`,
		"etcdv3":      `The "etcdv3" backend is not supported in OpenTofu v1.3 or later.`,
		"manta":       `The "manta" backend is not supported in OpenTofu v1.3 or later.`,
		"swift":       `The "swift" backend is not supported in OpenTofu v1.3 or later.`,
	}
}

// Backend returns the initialization factory for the given backend, or
// nil if none exists.
func Backend(name string) backend.InitFn {
	backendsLock.Lock()
	defer backendsLock.Unlock()
	return backends[name]
}

// Set sets a new backend in the list of backends. If f is nil then the
// backend will be removed from the map. If this backend already exists
// then it will be overwritten.
//
// This method sets this backend globally and care should be taken to do
// this only before OpenTofu is executing to prevent odd behavior of backends
// changing mid-execution.
//
// NOTE: Underscore-prefixed named are reserved for unit testing use via
// the RegisterTemp function. Do not add any underscore-prefixed names
// using this function.
func Set(name string, f backend.InitFn) {
	backendsLock.Lock()
	defer backendsLock.Unlock()

	if f == nil {
		delete(backends, name)
		return
	}

	backends[name] = f
}

// deprecatedBackendShim is used to wrap a backend and inject a deprecation
// warning into the Validate method.
type deprecatedBackendShim struct {
	backend.Backend
	Message string
}

// PrepareConfig delegates to the wrapped backend to validate its config
// and then appends shim's deprecation warning.
func (b deprecatedBackendShim) PrepareConfig(obj cty.Value) (cty.Value, tfdiags.Diagnostics) {
	newObj, diags := b.Backend.PrepareConfig(obj)
	return newObj, diags.Append(tfdiags.SimpleWarning(b.Message))
}

// DeprecateBackend can be used to wrap a backend to return a deprecation
// warning during validation.
func deprecateBackend(b backend.Backend, message string) backend.Backend {
	// Since a Backend wrapped by deprecatedBackendShim can no longer be
	// asserted as an Enhanced or Local backend, disallow those types here
	// entirely.  If something other than a basic backend.Backend needs to be
	// deprecated, we can add that functionality to schema.Backend or the
	// backend itself.
	if _, ok := b.(backend.Enhanced); ok {
		panic("cannot use DeprecateBackend on an Enhanced Backend")
	}

	if _, ok := b.(backend.Local); ok {
		panic("cannot use DeprecateBackend on a Local Backend")
	}

	return deprecatedBackendShim{
		Backend: b,
		Message: message,
	}
}
