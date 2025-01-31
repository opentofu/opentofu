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
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/lib/pq"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Function to skip a test unless in ACCeptance test mode.
//
// A running Postgres server identified by env variable
// DATABASE_URL is required for acceptance tests.
func testACC(t *testing.T) (connectionURI *url.URL) {
	skip := os.Getenv("TF_ACC") == "" && os.Getenv("TF_PG_TEST") == ""
	if skip {
		t.Log("pg backend tests requires setting TF_ACC or TF_PG_TEST")
		t.Skip()
	}
	databaseUrl, found := os.LookupEnv("DATABASE_URL")
	if !found {
		t.Fatal("pg backend tests require setting DATABASE_URL")
	}

	u, err := url.Parse(databaseUrl)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestBackendConfig(t *testing.T) {
	connectionURI := testACC(t)
	connStr := os.Getenv("DATABASE_URL")

	user := connectionURI.User.Username()
	password, _ := connectionURI.User.Password()
	databaseName := connectionURI.Path[1:]

	connectionURIObfuscated := connectionURI
	connectionURIObfuscated.User = nil

	testCases := []struct {
		Name                     string
		EnvVars                  map[string]string
		Config                   map[string]interface{}
		ExpectConfigurationError string
		ExpectConnectionError    string
	}{
		{
			Name: "valid-config",
			Config: map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "missing-conn_str-defaults-to-localhost",
			EnvVars: map[string]string{
				"PGSSLMODE":  "disable",
				"PGDATABASE": databaseName,
				"PGUSER":     user,
				"PGPASSWORD": password,
			},
			Config: map[string]interface{}{
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "conn-str-env-var",
			EnvVars: map[string]string{
				"PG_CONN_STR": connStr,
			},
			Config: map[string]interface{}{
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "setting-credentials-using-env-vars",
			EnvVars: map[string]string{
				"PGUSER":     "baduser",
				"PGPASSWORD": "badpassword",
			},
			Config: map[string]interface{}{
				"conn_str":    connectionURIObfuscated.String(),
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
			ExpectConnectionError: `authentication failed for user "baduser"`,
		},
		{
			Name: "host-in-env-vars",
			EnvVars: map[string]string{
				"PGHOST": "hostthatdoesnotexist",
			},
			Config: map[string]interface{}{
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
			ExpectConnectionError: `no such host`,
		},
		{
			Name: "boolean-env-vars",
			EnvVars: map[string]string{
				"PGSSLMODE":               "disable",
				"PG_SKIP_SCHEMA_CREATION": "f",
				"PG_SKIP_TABLE_CREATION":  "f",
				"PG_SKIP_INDEX_CREATION":  "f",
				"PGDATABASE":              databaseName,
			},
			Config: map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "wrong-boolean-env-vars",
			EnvVars: map[string]string{
				"PGSSLMODE":               "disable",
				"PG_SKIP_SCHEMA_CREATION": "foo",
				"PGDATABASE":              databaseName,
			},
			Config: map[string]interface{}{
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
			},
			ExpectConfigurationError: `error getting default for "skip_schema_creation"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			for k, v := range tc.EnvVars {
				t.Setenv(k, v)
			}

			config := backend.TestWrapConfig(tc.Config)
			schemaName := pq.QuoteIdentifier(tc.Config["schema_name"].(string))

			dbCleaner, err := sql.Open("postgres", connStr)
			if err != nil {
				t.Fatal(err)
			}
			defer dropSchemaByQuotedName(t, dbCleaner, schemaName)

			var diags tfdiags.Diagnostics
			b := New(encryption.StateEncryptionDisabled()).(*Backend)
			schema := b.ConfigSchema()
			spec := schema.DecoderSpec()
			obj, decDiags := hcldec.Decode(config, spec, nil)
			diags = diags.Append(decDiags)

			newObj, valDiags := b.PrepareConfig(obj)
			diags = diags.Append(valDiags.InConfigBody(config, ""))

			if tc.ExpectConfigurationError != "" {
				if !diags.HasErrors() {
					t.Fatal("error expected but got none")
				}
				if !strings.Contains(diags.ErrWithWarnings().Error(), tc.ExpectConfigurationError) {
					t.Fatalf("failed to find %q in %s", tc.ExpectConfigurationError, diags.ErrWithWarnings())
				}
				return
			} else if diags.HasErrors() {
				t.Fatal(diags.ErrWithWarnings())
			}

			obj = newObj

			confDiags := b.Configure(obj)
			if tc.ExpectConnectionError != "" {
				err := confDiags.InConfigBody(config, "").ErrWithWarnings()
				if err == nil {
					t.Fatal("error expected but got none")
				}
				if !strings.Contains(err.Error(), tc.ExpectConnectionError) {
					t.Fatalf("failed to find %q in %s", tc.ExpectConnectionError, err)
				}
				return
			} else if len(confDiags) != 0 {
				confDiags = confDiags.InConfigBody(config, "")
				t.Fatal(confDiags.ErrWithWarnings())
			}

			if b == nil {
				t.Fatal("Backend could not be configured")
			}

			_, err = b.db.Query(fmt.Sprintf("SELECT name, data FROM %s.%s LIMIT 1", schemaName, statesTableName))
			if err != nil {
				t.Fatal(err)
			}

			_, err = b.StateMgr(backend.DefaultStateName)
			if err != nil {
				t.Fatal(err)
			}

			s, err := b.StateMgr(backend.DefaultStateName)
			if err != nil {
				t.Fatal(err)
			}
			c := s.(*remote.State).Client.(*RemoteClient)
			if c.Name != backend.DefaultStateName {
				t.Fatal("RemoteClient name is not configured")
			}

			backend.TestBackendStates(t, b)
		})
	}

}

func TestBackendConfigSkipOptions(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()

	testCases := []struct {
		Name               string
		SkipSchemaCreation bool
		SkipTableCreation  bool
		SkipIndexCreation  bool
		TestIndexIsPresent bool
		Setup              func(t *testing.T, db *sql.DB, schemaName string)
	}{
		{
			Name:               "skip_schema_creation",
			SkipSchemaCreation: true,
			TestIndexIsPresent: true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string) {
				// create the schema as a prerequisites
				_, err := db.Query(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schemaName))
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:               "skip_table_creation",
			SkipTableCreation:  true,
			TestIndexIsPresent: true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string) {
				// since the table needs to be already created the schema must be too
				_, err := db.Query(fmt.Sprintf(`CREATE SCHEMA %s`, schemaName))
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Query(fmt.Sprintf(`CREATE TABLE %s.%s (
					id SERIAL PRIMARY KEY,
					name TEXT,
					data TEXT
					)`, schemaName, statesTableName))
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:               "skip_index_creation",
			SkipIndexCreation:  true,
			TestIndexIsPresent: true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string) {
				// Everything need to exists for the index to be created
				_, err := db.Query(fmt.Sprintf(`CREATE SCHEMA %s`, schemaName))
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Query(fmt.Sprintf(`CREATE TABLE %s.%s (
					id SERIAL PRIMARY KEY,
					name TEXT,
					data TEXT
					)`, schemaName, statesTableName))
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s.%s (name)`, statesIndexName, schemaName, statesTableName))
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:              "missing_index",
			SkipIndexCreation: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			schemaName := tc.Name

			config := backend.TestWrapConfig(map[string]interface{}{
				"conn_str":             connStr,
				"schema_name":          schemaName,
				"skip_schema_creation": tc.SkipSchemaCreation,
				"skip_table_creation":  tc.SkipTableCreation,
				"skip_index_creation":  tc.SkipIndexCreation,
			})
			schemaName = pq.QuoteIdentifier(schemaName)
			db, err := sql.Open("postgres", connStr)
			if err != nil {
				t.Fatal(err)
			}

			if tc.Setup != nil {
				tc.Setup(t, db, schemaName)
			}
			defer dropSchemaByQuotedName(t, db, schemaName)

			b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

			if b == nil {
				t.Fatal("Backend could not be configured")
			}

			// Make sure everything has been created

			// This tests that both the schema and the table have been created
			_, err = b.db.Query(fmt.Sprintf("SELECT name, data FROM %s.%s LIMIT 1", schemaName, statesTableName))
			if err != nil {
				t.Fatal(err)
			}
			if tc.TestIndexIsPresent {
				// Make sure that the index exists
				query := `select count(*) from pg_indexes where schemaname=$1 and tablename=$2 and indexname=$3;`
				var count int
				if err := b.db.QueryRow(query, tc.Name, statesTableName, statesIndexName).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("The index has not been created (%d)", count)
				}
			}

			_, err = b.StateMgr(backend.DefaultStateName)
			if err != nil {
				t.Fatal(err)
			}

			s, err := b.StateMgr(backend.DefaultStateName)
			if err != nil {
				t.Fatal(err)
			}
			c := s.(*remote.State).Client.(*RemoteClient)
			if c.Name != backend.DefaultStateName {
				t.Fatal("RemoteClient name is not configured")
			}

			// Make sure that all workspace must have a unique name
			_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s.%s VALUES (100, 'unique_name_test', '')`, schemaName, statesTableName))
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s.%s VALUES (101, 'unique_name_test', '')`, schemaName, statesTableName))
			if err == nil {
				t.Fatal("Creating two workspaces with the same name did not raise an error")
			}
		})
	}

}

func TestBackendStates(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()

	testCases := []string{
		fmt.Sprintf("terraform_%s", t.Name()),
		fmt.Sprintf("test with spaces: %s", t.Name()),
	}
	for _, schemaName := range testCases {
		t.Run(schemaName, func(t *testing.T) {
			dbCleaner, err := sql.Open("postgres", connStr)
			if err != nil {
				t.Fatal(err)
			}
			defer dropSchema(t, dbCleaner, schemaName)

			config := backend.TestWrapConfig(map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": schemaName,
			})
			b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

			if b == nil {
				t.Fatal("Backend could not be configured")
			}

			backend.TestBackendStates(t, b)
		})
	}
}

func TestBackendStateLocks(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	schemaName := fmt.Sprintf("terraform_%s", t.Name())
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer dropSchema(t, dbCleaner, schemaName)

	config := backend.TestWrapConfig(map[string]interface{}{
		"conn_str":    connStr,
		"schema_name": schemaName,
	})
	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

	if b == nil {
		t.Fatal("Backend could not be configured")
	}

	bb := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

	if bb == nil {
		t.Fatal("Backend could not be configured")
	}

	backend.TestBackendStateLocks(t, b, bb)
}

func TestBackendConcurrentLock(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()
	dbCleaner, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatal(err)
	}

	getStateMgr := func(schemaName string) (statemgr.Full, *statemgr.LockInfo) {
		config := backend.TestWrapConfig(map[string]interface{}{
			"conn_str":    connStr,
			"schema_name": schemaName,
		})
		b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)

		if b == nil {
			t.Fatal("Backend could not be configured")
		}
		stateMgr, err := b.StateMgr(backend.DefaultStateName)
		if err != nil {
			t.Fatalf("Failed to get the state manager: %v", err)
		}

		info := statemgr.NewLockInfo()
		info.Operation = "test"
		info.Who = schemaName

		return stateMgr, info
	}

	firstSchema, secondSchema := fmt.Sprintf("terraform_%s_1", t.Name()), fmt.Sprintf("terraform_%s_2", t.Name())
	defer dropSchema(t, dbCleaner, firstSchema)
	defer dropSchema(t, dbCleaner, secondSchema)

	s1, i1 := getStateMgr(firstSchema)
	s2, i2 := getStateMgr(secondSchema)

	// First we need to create the workspace as the lock for creating them is
	// global
	lockID1, err := s1.Lock(i1)
	if err != nil {
		t.Fatalf("failed to lock first state: %v", err)
	}

	if err = s1.PersistState(nil); err != nil {
		t.Fatalf("failed to persist state: %v", err)
	}

	if err := s1.Unlock(lockID1); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}

	lockID2, err := s2.Lock(i2)
	if err != nil {
		t.Fatalf("failed to lock second state: %v", err)
	}

	if err = s2.PersistState(nil); err != nil {
		t.Fatalf("failed to persist state: %v", err)
	}

	if err := s2.Unlock(lockID2); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}

	// Now we can test concurrent lock
	lockID1, err = s1.Lock(i1)
	if err != nil {
		t.Fatalf("failed to lock first state: %v", err)
	}

	lockID2, err = s2.Lock(i2)
	if err != nil {
		t.Fatalf("failed to lock second state: %v", err)
	}

	if err := s1.Unlock(lockID1); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}

	if err := s2.Unlock(lockID2); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}
}

func getDatabaseUrl() string {
	return os.Getenv("DATABASE_URL")
}

func dropSchema(t *testing.T, db *sql.DB, schemaName string) {
	dropSchemaByQuotedName(t, db, pq.QuoteIdentifier(schemaName))
}

func dropSchemaByQuotedName(t *testing.T, db *sql.DB, quotedSchemaName string) {
	rows, err := db.Query(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchemaName))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}
