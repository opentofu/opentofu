// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pg

// Create the test database: createdb terraform_backend_pg_test
// TF_ACC=1 GO111MODULE=on go test -v -mod=vendor -timeout=2m -parallel=4 github.com/opentofu/opentofu/backend/remote-state/pg

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func TestRemoteClient_impl(t *testing.T) {
	var _ remote.Client = new(RemoteClient)
	var _ remote.ClientLocker = new(RemoteClient)
}

func TestRemoteClient(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	schemaName := fmt.Sprintf("terraform_%s", t.Name())
	tableName := fmt.Sprintf("terraform_%s", t.Name())
	indexName := fmt.Sprintf("terraform_%s", t.Name())
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer dropSchema(t, dbCleaner, schemaName)

	config := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": schemaName,
		"table_name":  tableName,
		"index_name":  indexName,
	})
	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

	if b == nil {
		t.Fatal("Backend could not be configured")
	}

	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, s.(*remote.State).Client)
}

func TestRemoteLocks(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	schemaName := fmt.Sprintf("terraform_%s", t.Name())
	tableName := fmt.Sprintf("terraform_%s", t.Name())
	indexName := fmt.Sprintf("terraform_%s", t.Name())
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer dropSchema(t, dbCleaner, schemaName)

	config := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": schemaName,
		"table_name":  tableName,
		"index_name":  indexName,
	})

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)
	s1, err := b1.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)
	s2, err := b2.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}

// TestConcurrentCreationLocksInDifferentSchemas tests whether backends with different schemas
// affect each other while taking global workspace creation locks.
func TestConcurrentCreationLocksInDifferentSchemas(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}

	firstSchema := fmt.Sprintf("terraform_%s_1", t.Name())
	firstTable := fmt.Sprintf("terraform_%s_1", t.Name())
	firstIndex := fmt.Sprintf("terraform_%s_1", t.Name())

	secondSchema := fmt.Sprintf("terraform_%s_2", t.Name())
	secondTable := fmt.Sprintf("terraform_%s_2", t.Name())
	secondIndex := fmt.Sprintf("terraform_%s_2", t.Name())

	defer dropSchema(t, dbCleaner, firstSchema)
	defer dropSchema(t, dbCleaner, secondSchema)

	firstConfig := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": firstSchema,
		"table_name":  firstTable,
		"index_name":  firstIndex,
	})

	secondConfig := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": secondSchema,
		"table_name":  secondTable,
		"index_name":  secondIndex,
	})

	//nolint:errcheck // this is a test, I am fine with panic here
	firstBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), firstConfig).(*Backend)

	//nolint:errcheck // this is a test, I am fine with panic here
	secondBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), secondConfig).(*Backend)

	//nolint:errcheck // this is a test, I am fine with panic here
	thirdBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), secondConfig).(*Backend)

	// We operate on remote clients instead of state managers to simulate the
	// first call to backend.StateMgr(), which creates an empty state in default
	// workspace.
	firstClient := &RemoteClient{
		Client:     firstBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: firstBackend.schemaName,
		TableName:  firstBackend.tableName,
		IndexName:  firstBackend.indexName,
	}

	secondClient := &RemoteClient{
		Client:     secondBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: secondBackend.schemaName,
		TableName:  secondBackend.tableName,
		IndexName:  secondBackend.indexName,
	}

	thirdClient := &RemoteClient{
		Client:     thirdBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: thirdBackend.schemaName,
		TableName:  thirdBackend.tableName,
		IndexName:  thirdBackend.indexName,
	}

	// It doesn't matter what lock info to supply for workspace creation.
	lock := &statemgr.LockInfo{
		ID:        "1",
		Operation: "test",
		Info:      "This needs to lock for workspace creation",
		Who:       "me",
		Version:   "1",
		Created:   time.Date(1999, 8, 19, 0, 0, 0, 0, time.UTC),
	}

	// Those calls with empty database must think they are locking
	// for workspace creation, both of them must succeed since they
	// are operating on different schemas.
	if _, err = firstClient.Lock(lock); err != nil {
		t.Fatal(err)
	}
	if _, err = secondClient.Lock(lock); err != nil {
		t.Fatal(err)
	}

	// This call must fail since we are trying to acquire the same
	// lock as the first client. We need to make this call from a
	// separate session, since advisory locks are okay to be re-acquired
	// during the same session.
	if _, err = thirdClient.Lock(lock); err == nil {
		t.Fatal("Expected an error to be thrown on a second lock attempt")
	} else if lockErr := err.(*statemgr.LockError); lockErr.Info != lock && //nolint:errcheck,errorlint // this is a test, I am fine with panic here
		lockErr.Err.Error() != "Already locked for workspace creation: default" {
		t.Fatalf("Unexpected error thrown on a second lock attempt: %v", err)
	}
}

// TestConcurrentCreationLocksInDifferentTables tests whether backends with different tables
// affect each other while taking global workspace creation locks.
func TestConcurrentCreationLocksInDifferentTables(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("terraform_%s", t.Name())

	firstTable := fmt.Sprintf("terraform_%s_1", t.Name())
	firstIndex := fmt.Sprintf("terraform_%s_1", t.Name())

	secondTable := fmt.Sprintf("terraform_%s_2", t.Name())
	secondIndex := fmt.Sprintf("terraform_%s_2", t.Name())

	defer dropSchema(t, dbCleaner, schema)

	firstConfig := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": schema,
		"table_name":  firstTable,
		"index_name":  firstIndex,
	})

	secondConfig := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": schema,
		"table_name":  secondTable,
		"index_name":  secondIndex,
	})

	//nolint:errcheck // this is a test, I am fine with panic here
	firstBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), firstConfig).(*Backend)

	//nolint:errcheck // this is a test, I am fine with panic here
	secondBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), secondConfig).(*Backend)

	//nolint:errcheck // this is a test, I am fine with panic here
	thirdBackend := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), secondConfig).(*Backend)

	// We operate on remote clients instead of state managers to simulate the
	// first call to backend.StateMgr(), which creates an empty state in default
	// workspace.
	firstClient := &RemoteClient{
		Client:     firstBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: firstBackend.schemaName,
		TableName:  firstBackend.tableName,
		IndexName:  firstBackend.indexName,
	}

	secondClient := &RemoteClient{
		Client:     secondBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: secondBackend.schemaName,
		TableName:  secondBackend.tableName,
		IndexName:  secondBackend.indexName,
	}

	thirdClient := &RemoteClient{
		Client:     thirdBackend.db,
		Name:       backend.DefaultStateName,
		SchemaName: thirdBackend.schemaName,
		TableName:  thirdBackend.tableName,
		IndexName:  thirdBackend.indexName,
	}

	// It doesn't matter what lock info to supply for workspace creation.
	lock := &statemgr.LockInfo{
		ID:        "1",
		Operation: "test",
		Info:      "This needs to lock for workspace creation",
		Who:       "me",
		Version:   "1",
		Created:   time.Date(1999, 8, 19, 0, 0, 0, 0, time.UTC),
	}

	// Those calls with empty database must think they are locking
	// for workspace creation, both of them must succeed since they
	// are operating on different schemas.
	if _, err = firstClient.Lock(lock); err != nil {
		t.Fatal(err)
	}
	if _, err = secondClient.Lock(lock); err != nil {
		t.Fatal(err)
	}

	// This call must fail since we are trying to acquire the same
	// lock as the first client. We need to make this call from a
	// separate session, since advisory locks are okay to be re-acquired
	// during the same session.
	if _, err = thirdClient.Lock(lock); err == nil {
		t.Fatal("Expected an error to be thrown on a second lock attempt")
	} else if lockErr := err.(*statemgr.LockError); lockErr.Info != lock && //nolint:errcheck // this is a test, I am fine with panic here
		lockErr.Err.Error() != "Already locked for workspace creation: default" {
		t.Fatalf("Unexpected error thrown on a second lock attempt: %v", err)
	}
}
