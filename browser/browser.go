// Package browser is the feature layer between the Postgres connection
// and the terminal UI: it decides what to fetch and what query to run,
// but knows nothing about tview or terminals. That keeps it unit-testable
// with a fake DB and no live database or screen.
package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/cfindlayisme/pgtui/db"
)

// DB is the subset of *db.Conn the browser depends on, so tests can
// supply a fake implementation instead of a live Postgres connection.
type DB interface {
	ListDatabases(ctx context.Context) ([]string, error)
	ListSchemas(ctx context.Context) ([]string, error)
	ListTables(ctx context.Context, schema string) ([]string, error)
	ListIndexes(ctx context.Context, schema, table string) ([]db.IndexInfo, error)
	RunQuery(ctx context.Context, sql string) (*db.QueryResult, error)
	SwitchDatabase(ctx context.Context, dbname string) error
	Close(ctx context.Context)
}

type Browser struct {
	DB DB
}

func New(d DB) *Browser {
	return &Browser{DB: d}
}

// Databases lists every database on the currently connected server.
func (b *Browser) Databases(ctx context.Context) ([]string, error) {
	return b.DB.ListDatabases(ctx)
}

// Schemas switches to dbname and lists its schemas.
func (b *Browser) Schemas(ctx context.Context, dbname string) ([]string, error) {
	if err := b.DB.SwitchDatabase(ctx, dbname); err != nil {
		return nil, err
	}
	return b.DB.ListSchemas(ctx)
}

// Tables switches to dbname and lists the tables in schema.
func (b *Browser) Tables(ctx context.Context, dbname, schema string) ([]string, error) {
	if err := b.DB.SwitchDatabase(ctx, dbname); err != nil {
		return nil, err
	}
	return b.DB.ListTables(ctx, schema)
}

// Indexes switches to dbname and lists the indexes on schema.table.
func (b *Browser) Indexes(ctx context.Context, dbname, schema, table string) ([]db.IndexInfo, error) {
	if err := b.DB.SwitchDatabase(ctx, dbname); err != nil {
		return nil, err
	}
	return b.DB.ListIndexes(ctx, schema, table)
}

// RunQuery switches to dbname and executes sql.
func (b *Browser) RunQuery(ctx context.Context, dbname, sql string) (*db.QueryResult, error) {
	if err := b.DB.SwitchDatabase(ctx, dbname); err != nil {
		return nil, err
	}
	return b.DB.RunQuery(ctx, sql)
}

// PreviewQuery builds a SELECT * ... LIMIT query for previewing a table's rows.
func PreviewQuery(schema, table string, limit int) string {
	return fmt.Sprintf("SELECT * FROM %s.%s LIMIT %d", QuoteIdent(schema), QuoteIdent(table), limit)
}

// TableQuery is the default preview query run when a table is selected in
// the tree without picking a specific option.
func TableQuery(schema, table string) string {
	return PreviewQuery(schema, table, 100)
}

// CountQuery builds a query counting all rows in a table.
func CountQuery(schema, table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", QuoteIdent(schema), QuoteIdent(table))
}

// ColumnsQuery builds a query listing a table's columns, types, and
// nullability, in declaration order.
func ColumnsQuery(schema, table string) string {
	return fmt.Sprintf(
		`SELECT column_name, data_type, is_nullable, column_default
		 FROM information_schema.columns
		 WHERE table_schema = %s AND table_name = %s
		 ORDER BY ordinal_position`,
		quoteLiteral(schema), quoteLiteral(table))
}

// QuoteIdent double-quotes a Postgres identifier, escaping embedded quotes.
func QuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// quoteLiteral single-quotes a Postgres string literal, escaping embedded quotes.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
