// Package db handles the live Postgres connection: connecting, switching
// databases, and running queries. It has no knowledge of the terminal UI.
package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// pgxConn is the subset of *pgx.Conn this package depends on, narrow
// enough that tests can substitute a mock instead of a live connection.
type pgxConn interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Close(ctx context.Context) error
}

// connectFn opens a new connection. It's a variable, not a direct call to
// pgx.Connect, so tests can substitute a mock without touching the network.
var connectFn = func(ctx context.Context, dsn string) (pgxConn, error) {
	return pgx.Connect(ctx, dsn)
}

// Conn wraps a single live connection to a Postgres server/database.
type Conn struct {
	dsn  string
	conn pgxConn
	db   string
}

// Connect opens dsn, which is expected to already point at dbname (e.g.
// via Config.DSN()). dbname is recorded so SwitchDatabase can no-op when
// asked to switch to the database it's already on.
func Connect(ctx context.Context, dsn, dbname string) (*Conn, error) {
	conn, err := connectFn(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Conn{dsn: dsn, conn: conn, db: dbname}, nil
}

func (c *Conn) Close(ctx context.Context) {
	if c.conn != nil {
		c.conn.Close(ctx)
	}
}

// SwitchDatabase reconnects to dbname on the same server, since a single
// Postgres connection can never query across databases.
func (c *Conn) SwitchDatabase(ctx context.Context, dbname string) error {
	if c.db == dbname {
		return nil
	}
	newDSN, err := WithDatabase(c.dsn, dbname)
	if err != nil {
		return err
	}
	newConn, err := connectFn(ctx, newDSN)
	if err != nil {
		return err
	}
	if c.conn != nil {
		c.conn.Close(ctx)
	}
	c.conn = newConn
	c.dsn = newDSN
	c.db = dbname
	return nil
}

func (c *Conn) ListDatabases(ctx context.Context) ([]string, error) {
	return queryStrings(ctx, c.conn,
		`SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname`)
}

func (c *Conn) ListSchemas(ctx context.Context) ([]string, error) {
	return queryStrings(ctx, c.conn,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
		 ORDER BY schema_name`)
}

func (c *Conn) ListTables(ctx context.Context, schema string) ([]string, error) {
	return queryStrings(ctx, c.conn,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = $1 ORDER BY table_name`, schema)
}

// IndexInfo describes one index on a table.
type IndexInfo struct {
	Name       string
	Definition string
}

func (c *Conn) ListIndexes(ctx context.Context, schema, table string) ([]IndexInfo, error) {
	rows, err := c.conn.Query(ctx,
		`SELECT indexname, indexdef FROM pg_indexes
		 WHERE schemaname = $1 AND tablename = $2 ORDER BY indexname`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		if err := rows.Scan(&idx.Name, &idx.Definition); err != nil {
			return nil, err
		}
		out = append(out, idx)
	}
	return out, rows.Err()
}

func queryStrings(ctx context.Context, conn pgxConn, sql string, args ...any) ([]string, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// QueryResult is a flattened, display-ready result set.
type QueryResult struct {
	Columns []string
	Rows    [][]string
}

func (c *Conn) RunQuery(ctx context.Context, sql string) (*QueryResult, error) {
	rows, err := c.conn.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.Name
	}

	var result [][]string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			row[i] = FormatValue(v)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &QueryResult{Columns: cols, Rows: result}, nil
}
