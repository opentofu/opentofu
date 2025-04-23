// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pg

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"os"
	"strconv"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
)

func defaultBoolFunc(k string, dv bool) schema.SchemaDefaultFunc {
	return func() (interface{}, error) {
		if v := os.Getenv(k); v != "" {
			return strconv.ParseBool(v)
		}

		return dv, nil
	}
}

// New creates a new backend for Postgres remote state.
func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"conn_str": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Postgres connection string; a `postgres://` URL",
				DefaultFunc: schema.EnvDefaultFunc("PG_CONN_STR", nil),
			},

			"schema_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Name of the automatically managed Postgres schema to store state",
				DefaultFunc: schema.EnvDefaultFunc("PG_SCHEMA_NAME", "terraform_remote_state"),
			},

			"skip_schema_creation": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "If set to `true`, OpenTofu won't try to create the Postgres schema",
				DefaultFunc: defaultBoolFunc("PG_SKIP_SCHEMA_CREATION", false),
			},

			"table_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Name of the automatically managed Postgres table to store state",
				DefaultFunc: schema.EnvDefaultFunc("PG_TABLE_NAME", "states"),
			},

			"skip_table_creation": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "If set to `true`, OpenTofu won't try to create the Postgres table",
				DefaultFunc: defaultBoolFunc("PG_SKIP_TABLE_CREATION", false),
			},

			"index_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Name of the automatically managed Postgres index used for stored state",
				DefaultFunc: schema.EnvDefaultFunc("PG_INDEX_NAME", "states_by_name"),
			},

			"skip_index_creation": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "If set to `true`, OpenTofu won't try to create the Postgres index",
				DefaultFunc: defaultBoolFunc("PG_SKIP_INDEX_CREATION", false),
			},
		},
	}

	result := &Backend{Backend: s, encryption: enc}
	result.Backend.ConfigureFunc = result.configure
	return result
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	// The fields below are set from configure
	db         *sql.DB
	configData *schema.ResourceData
	connStr    string
	schemaName string
	tableName  string
	indexName  string
}

func (b *Backend) configure(ctx context.Context) error {
	// Grab the resource data
	b.configData = schema.FromContextBackendConfig(ctx)
	data := b.configData

	b.connStr = data.Get("conn_str").(string)
	b.schemaName = data.Get("schema_name").(string)
	b.tableName = data.Get("table_name").(string)
	b.indexName = data.Get("index_name").(string)
	skipSchemaCreation := data.Get("skip_schema_creation").(bool)
	skipTableCreation := data.Get("skip_table_creation").(bool)
	skipIndexCreation := data.Get("skip_index_creation").(bool)

	db, err := sql.Open("postgres", b.connStr)
	if err != nil {
		return err
	}

	// Prepare database schema, tables, & indexes.
	var query string

	if !skipSchemaCreation {
		// list all schemas to see if it exists
		var count int
		query = `select count(1) from information_schema.schemata where schema_name = $1`
		if err = db.QueryRow(query, b.schemaName).Scan(&count); err != nil {
			return err
		}

		// skip schema creation if schema already exists
		// `CREATE SCHEMA IF NOT EXISTS` is to be avoided if ever
		// a user hasn't been granted the `CREATE SCHEMA` privilege
		if count < 1 {
			// tries to create the schema
			query = fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pq.QuoteIdentifier(b.schemaName))
			if _, err = db.Exec(query); err != nil {
				return err
			}
		}
	}

	if !skipTableCreation {
		query = "CREATE SEQUENCE IF NOT EXISTS public.global_states_id_seq AS bigint"
		if _, err = db.Exec(query); err != nil {
			return err
		}

		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.%s (
			id bigint NOT NULL DEFAULT nextval('public.global_states_id_seq') PRIMARY KEY,
			name text UNIQUE,
			data text
			)`, pq.QuoteIdentifier(b.schemaName), pq.QuoteIdentifier(b.tableName))

		if _, err = db.Exec(query); err != nil {
			return err
		}
	}

	if !skipIndexCreation {
		query = fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s.%s (name)`, pq.QuoteIdentifier(b.indexName), pq.QuoteIdentifier(b.schemaName), pq.QuoteIdentifier(b.tableName))
		if _, err = db.Exec(query); err != nil {
			return err
		}
	}

	// Assign db after its schema is prepared.
	b.db = db

	return nil
}
