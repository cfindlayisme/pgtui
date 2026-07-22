package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cfindlayisme/pgtui/internal/db"
)

// fakeDB is an in-memory stand-in for a live Postgres connection, letting
// Browser's orchestration logic be tested without a real database.
type fakeDB struct {
	currentDB string

	databases []string
	schemas   map[string][]string                             // dbname -> schemas
	tables    map[string]map[string][]string                  // dbname -> schema -> tables
	indexes   map[string]map[string]map[string][]db.IndexInfo // dbname -> schema -> table -> indexes
	results   map[string]*db.QueryResult                      // sql -> result

	switchErr error
	queryErr  error

	switchCalls []string
	queryCalls  []string
}

func (f *fakeDB) ListDatabases(ctx context.Context) ([]string, error) {
	return f.databases, nil
}

func (f *fakeDB) ListSchemas(ctx context.Context) ([]string, error) {
	return f.schemas[f.currentDB], nil
}

func (f *fakeDB) ListTables(ctx context.Context, schema string) ([]string, error) {
	return f.tables[f.currentDB][schema], nil
}

func (f *fakeDB) ListIndexes(ctx context.Context, schema, table string) ([]db.IndexInfo, error) {
	return f.indexes[f.currentDB][schema][table], nil
}

func (f *fakeDB) RunQuery(ctx context.Context, sql string) (*db.QueryResult, error) {
	f.queryCalls = append(f.queryCalls, sql)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.results[sql], nil
}

func (f *fakeDB) SwitchDatabase(ctx context.Context, dbname string) error {
	f.switchCalls = append(f.switchCalls, dbname)
	if f.switchErr != nil {
		return f.switchErr
	}
	f.currentDB = dbname
	return nil
}

func (f *fakeDB) Close(ctx context.Context) {}

func TestBrowser_Databases(t *testing.T) {
	fake := &fakeDB{databases: []string{"alpha", "beta"}}
	b := New(fake)

	got, err := b.Databases(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, got)
}

func TestBrowser_Schemas_SwitchesDatabaseFirst(t *testing.T) {
	fake := &fakeDB{
		schemas: map[string][]string{"alpha": {"public", "app"}},
	}
	b := New(fake)

	got, err := b.Schemas(context.Background(), "alpha")

	require.NoError(t, err)
	assert.Equal(t, []string{"public", "app"}, got)
	assert.Equal(t, []string{"alpha"}, fake.switchCalls)
}

func TestBrowser_Schemas_PropagatesSwitchError(t *testing.T) {
	fake := &fakeDB{switchErr: errors.New("connection refused")}
	b := New(fake)

	_, err := b.Schemas(context.Background(), "alpha")

	assert.EqualError(t, err, "connection refused")
}

func TestBrowser_Tables_SwitchesDatabaseFirst(t *testing.T) {
	fake := &fakeDB{
		tables: map[string]map[string][]string{
			"alpha": {"public": {"users", "orders"}},
		},
	}
	b := New(fake)

	got, err := b.Tables(context.Background(), "alpha", "public")

	require.NoError(t, err)
	assert.Equal(t, []string{"users", "orders"}, got)
	assert.Equal(t, []string{"alpha"}, fake.switchCalls)
}

func TestBrowser_Indexes_SwitchesDatabaseFirst(t *testing.T) {
	want := []db.IndexInfo{{Name: "users_pkey", Definition: "CREATE UNIQUE INDEX ..."}}
	fake := &fakeDB{
		indexes: map[string]map[string]map[string][]db.IndexInfo{
			"alpha": {"public": {"users": want}},
		},
	}
	b := New(fake)

	got, err := b.Indexes(context.Background(), "alpha", "public", "users")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, []string{"alpha"}, fake.switchCalls)
}

func TestBrowser_Indexes_PropagatesSwitchError(t *testing.T) {
	fake := &fakeDB{switchErr: errors.New("connection refused")}
	b := New(fake)

	_, err := b.Indexes(context.Background(), "alpha", "public", "users")

	assert.EqualError(t, err, "connection refused")
}

func TestBrowser_RunQuery_SwitchesDatabaseFirst(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
	fake := &fakeDB{
		results: map[string]*db.QueryResult{"SELECT 1": want},
	}
	b := New(fake)

	got, err := b.RunQuery(context.Background(), "alpha", "SELECT 1")

	require.NoError(t, err)
	assert.Same(t, want, got)
	assert.Equal(t, []string{"alpha"}, fake.switchCalls)
	assert.Equal(t, []string{"SELECT 1"}, fake.queryCalls)
}

func TestBrowser_RunQuery_SwitchFailureShortCircuits(t *testing.T) {
	fake := &fakeDB{switchErr: errors.New("no such database")}
	b := New(fake)

	_, err := b.RunQuery(context.Background(), "ghost", "SELECT 1")

	assert.EqualError(t, err, "no such database")
	assert.Empty(t, fake.queryCalls, "should not run the query if switching databases failed")
}

func TestQuoteIdent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple identifier", "users", `"users"`},
		{"embedded quote is escaped", `weird"name`, `"weird""name"`},
		{"empty string", "", `""`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, QuoteIdent(tc.in))
		})
	}
}

func TestTableQuery(t *testing.T) {
	got := TableQuery("public", "users")
	assert.Equal(t, `SELECT * FROM "public"."users" LIMIT 100`, got)
}

func TestTableQuery_EscapesIdentifiers(t *testing.T) {
	got := TableQuery(`sch"ema`, "table")
	assert.Equal(t, `SELECT * FROM "sch""ema"."table" LIMIT 100`, got)
}

func TestPreviewQuery(t *testing.T) {
	got := PreviewQuery("public", "users", 1000)
	assert.Equal(t, `SELECT * FROM "public"."users" LIMIT 1000`, got)
}

func TestTableQuery_MatchesPreviewQueryAt100(t *testing.T) {
	assert.Equal(t, PreviewQuery("public", "users", 100), TableQuery("public", "users"))
}

func TestCountQuery(t *testing.T) {
	got := CountQuery("public", "users")
	assert.Equal(t, `SELECT COUNT(*) FROM "public"."users"`, got)
}

func TestColumnsQuery(t *testing.T) {
	got := ColumnsQuery("public", "users")
	assert.Contains(t, got, "FROM information_schema.columns")
	assert.Contains(t, got, "WHERE table_schema = 'public' AND table_name = 'users'")
	assert.Contains(t, got, "ORDER BY ordinal_position")
}

func TestColumnsQuery_EscapesLiterals(t *testing.T) {
	got := ColumnsQuery(`sch'ema`, "table")
	assert.Contains(t, got, `table_schema = 'sch''ema'`)
}
