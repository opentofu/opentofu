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
			Name: "valid-config-with-table-name",
			Config: map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "valid-config-with-index-name",
			Config: map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "valid-config-with-table-name-and-index-name",
			Config: map[string]interface{}{
				"conn_str":    connStr,
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
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
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
			},
		},
		{
			Name: "conn-str-env-var",
			EnvVars: map[string]string{
				"PG_CONN_STR": connStr,
			},
			Config: map[string]interface{}{
				"schema_name": fmt.Sprintf("terraform_%s", t.Name()),
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
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
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
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
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
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
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
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
				"table_name":  fmt.Sprintf("terraform_%s", t.Name()),
				"index_name":  fmt.Sprintf("terraform_%s", t.Name()),
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

			schemaName := b.Config().Get("schema_name").(string)
			tableName := b.Config().Get("table_name").(string)
			indexName := b.Config().Get("index_name").(string)
			skipSchemaCreation := b.Config().Get("skip_schema_creation").(bool)
			skipTableCreation := b.Config().Get("skip_table_creation").(bool)
			skipIndexCreation := b.Config().Get("skip_index_creation").(bool)

			dbCleaner, err := sql.Open("postgres", connStr)
			if err != nil {
				t.Fatal(err)
			}
			defer dropSchema(t, dbCleaner, schemaName)

			// Make sure everything has been created
			if skipSchemaCreation {
				// Make sure schema exists
				var count int
				query := `select count(*) from information_schema.schemata where schema_name=$1`
				if err = b.db.QueryRow(query, schemaName).Scan(&count); err != nil {
					t.Fatal(err)
				}

				if count != 1 {
					t.Fatalf("The schema has not been created (%d)", count)
				}
			}

			if skipTableCreation {
				// Make sure that the index exists
				var count int

				query := `select count(*) from pg_catalog.pg_tables where schemaname=$1 and tablename=$2;`
				err = b.db.QueryRow(query, schemaName, tableName).Scan(&count)
				if err != nil {
					t.Fatal(err)
				}

				if count != 1 {
					t.Fatalf("The table has not been created (%d)", count)
				}
			}

			if skipIndexCreation {
				// Make sure that the index exists
				var count int

				query := `select count(*) from pg_indexes where schemaname=$1 and tablename=$2 and indexname=$3;`
				err = b.db.QueryRow(query, schemaName, tableName, indexName+"_name_key").Scan(&count)
				if err != nil {
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

			backend.TestBackendStates(t, b)
		})
	}

}

func TestBackendConfigSkipOptions(t *testing.T) {
	testACC(t)
	connStr := getDatabaseUrl()

	testCases := []struct {
		Name                string
		SkipSchemaCreation  bool
		SkipTableCreation   bool
		SkipIndexCreation   bool
		TestSchemaIsPresent bool
		TestTableIsPresent  bool
		TestIndexIsPresent  bool
		Setup               func(t *testing.T, db *sql.DB, schemaName string, tableName string, indexName string)
	}{
		{
			Name:                "skip_schema_creation",
			SkipSchemaCreation:  true,
			SkipTableCreation:   false,
			SkipIndexCreation:   false,
			TestSchemaIsPresent: true,
			TestTableIsPresent:  true,
			TestIndexIsPresent:  true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string, tableName string, indexName string) {
				// create the schema as a prerequisites
				query := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pq.QuoteIdentifier(schemaName))
				_, err := db.Exec(query)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:                "skip_table_creation",
			SkipSchemaCreation:  true,
			SkipTableCreation:   true,
			SkipIndexCreation:   false,
			TestSchemaIsPresent: true,
			TestTableIsPresent:  true,
			TestIndexIsPresent:  true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string, tableName string, indexName string) {
				// since the table needs to be already created the schema must be too
				query := fmt.Sprintf(`CREATE SCHEMA %s`, pq.QuoteIdentifier(schemaName))
				_, err := db.Exec(query)
				if err != nil {
					t.Fatal(err)
				}
				query = fmt.Sprintf(`CREATE TABLE %s.%s (
					id SERIAL PRIMARY KEY,
					name text UNIQUE,
					data TEXT
					)`, pq.QuoteIdentifier(schemaName), pq.QuoteIdentifier(tableName))
				_, err = db.Exec(query)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:                "skip_index_creation",
			SkipSchemaCreation:  true,
			SkipTableCreation:   true,
			SkipIndexCreation:   true,
			TestSchemaIsPresent: true,
			TestTableIsPresent:  true,
			TestIndexIsPresent:  true,
			Setup: func(t *testing.T, db *sql.DB, schemaName string, tableName string, indexName string) {
				// Everything need to exists for the index to be created
				query := fmt.Sprintf(`CREATE SCHEMA %s`, pq.QuoteIdentifier(schemaName))
				_, err := db.Exec(query)
				if err != nil {
					t.Fatal(err)
				}
				query = fmt.Sprintf(`CREATE TABLE %s.%s (
					id SERIAL PRIMARY KEY,
					name text UNIQUE,
					data TEXT
					)`, pq.QuoteIdentifier(schemaName), pq.QuoteIdentifier(tableName))
				_, err = db.Exec(query)
				if err != nil {
					t.Fatal(err)
				}
				query = fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s.%s (name)`, pq.QuoteIdentifier(indexName), pq.QuoteIdentifier(schemaName), pq.QuoteIdentifier(tableName))
				_, err = db.Exec(query)
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
			tableName := tc.Name
			indexName := tc.Name

			config := backend.TestWrapConfig(map[string]interface{}{
				"conn_str":             connStr,
				"schema_name":          schemaName,
				"table_name":           tableName,
				"index_name":           indexName,
				"skip_schema_creation": tc.SkipSchemaCreation,
				"skip_table_creation":  tc.SkipTableCreation,
				"skip_index_creation":  tc.SkipIndexCreation,
			})

			db, err := sql.Open("postgres", connStr)
			if err != nil {
				t.Fatal(err)
			}

			if tc.Setup != nil {
				tc.Setup(t, db, schemaName, tableName, indexName)
			}
			defer dropSchema(t, db, schemaName)

			unconfiguredBackend := New(encryption.StateEncryptionDisabled())
			b := backend.TestBackendConfig(t, unconfiguredBackend, config).(*Backend)

			if b == nil {
				t.Fatal("Backend could not be configured")
			}

			// Make sure everything has been created
			if tc.TestSchemaIsPresent {
				// Make sure schema exists
				var count int
				query := `select count(*) from information_schema.schemata where schema_name=$1`
				if err = b.db.QueryRow(query, schemaName).Scan(&count); err != nil {
					t.Fatal(err)
				}

				if count != 1 {
					t.Fatalf("The schema has not been created (%d)", count)
				}
			}

			if tc.TestTableIsPresent {
				// Make sure that the index exists
				var count int

				query := `select count(*) from pg_catalog.pg_tables where schemaname=$1 and tablename=$2;`
				err = b.db.QueryRow(query, schemaName, tableName).Scan(&count)
				if err != nil {
					t.Fatal(err)
				}

				if count != 1 {
					t.Fatalf("The table has not been created (%d)", count)
				}
			}

			if tc.TestIndexIsPresent {
				// Make sure that the index exists
				var count int

				query := `select count(*) from pg_indexes where schemaname=$1 and tablename=$2 and indexname=$3;`
				err = b.db.QueryRow(query, schemaName, tableName, indexName+"_name_key").Scan(&count)
				if err != nil {
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
			query := fmt.Sprintf(`INSERT INTO %s.%s VALUES (100, 'unique_name_test', '')`, pq.QuoteIdentifier(schemaName), pq.QuoteIdentifier(tableName))
			_, err = db.Exec(query)
			if err != nil {
				t.Fatal(err)
			}
			query = fmt.Sprintf(`INSERT INTO %s.%s VALUES (101, 'unique_name_test', '')`, pq.QuoteIdentifier(schemaName), pq.QuoteIdentifier(tableName))
			_, err = db.Exec(query)
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
	for _, testCaseName := range testCases {
		t.Run(testCaseName, func(t *testing.T) {
			schemaName := testCaseName
			tableName := testCaseName
			indexName := testCaseName
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

			backend.TestBackendStates(t, b)
		})
	}
}

func TestBackendStateLocks(t *testing.T) {
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

	getStateMgr := func(schemaName string, tableName string, indexName string) (statemgr.Full, *statemgr.LockInfo) {
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
		stateMgr, err := b.StateMgr(backend.DefaultStateName)
		if err != nil {
			t.Fatalf("Failed to get the state manager: %v", err)
		}

		info := statemgr.NewLockInfo()
		info.Operation = "test"
		info.Who = schemaName

		return stateMgr, info
	}

	firstSchema := fmt.Sprintf("terraform_%s_1", t.Name())
	firstTable := fmt.Sprintf("terraform_%s_1", t.Name())
	firstIndex := fmt.Sprintf("terraform_%s_1", t.Name())

	secondSchema := fmt.Sprintf("terraform_%s_2", t.Name())
	secondTable := fmt.Sprintf("terraform_%s_2", t.Name())
	secondIndex := fmt.Sprintf("terraform_%s_2", t.Name())

	defer dropSchema(t, dbCleaner, firstSchema)
	defer dropSchema(t, dbCleaner, secondSchema)

	s1, i1 := getStateMgr(firstSchema, firstTable, firstIndex)
	s2, i2 := getStateMgr(secondSchema, secondTable, secondIndex)

	// First we need to create the workspace as the lock for creating them is
	// global
	lockID1, err := s1.Lock(i1)
	if err != nil {
		t.Fatalf("failed to lock first state: %v", err)
	}

	if err = s1.PersistState(nil); err != nil {
		t.Fatalf("failed to persist state: %v", err)
	}

	if err = s1.Unlock(lockID1); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}

	lockID2, err := s2.Lock(i2)
	if err != nil {
		t.Fatalf("failed to lock second state: %v", err)
	}

	if err = s2.PersistState(nil); err != nil {
		t.Fatalf("failed to persist state: %v", err)
	}

	if err = s2.Unlock(lockID2); err != nil {
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

	if err = s1.Unlock(lockID1); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}

	if err = s2.Unlock(lockID2); err != nil {
		t.Fatalf("failed to unlock first state: %v", err)
	}
}

func getDatabaseUrl() string {
	return os.Getenv("DATABASE_URL")
}

func dropSchema(t *testing.T, db *sql.DB, schemaName string) {
	query := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", pq.QuoteIdentifier(schemaName))
	_, err := db.Exec(query)
	if err != nil {
		t.Fatal(err)
	}
}
