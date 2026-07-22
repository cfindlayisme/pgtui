package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- query methods, mocked via pgxmock -------------------------------

func newMockConn(t *testing.T) pgxmock.PgxConnIface {
	t.Helper()
	mock, err := pgxmock.NewConn()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close(context.Background()) })
	return mock
}

func TestConn_ListDatabases(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT datname FROM pg_database`).
		WillReturnRows(pgxmock.NewRows([]string{"datname"}).AddRow("alpha").AddRow("beta"))

	c := &Conn{conn: mock}
	got, err := c.ListDatabases(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestConn_ListDatabases_PropagatesQueryError(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT datname FROM pg_database`).
		WillReturnError(errors.New("connection reset"))

	c := &Conn{conn: mock}
	_, err := c.ListDatabases(context.Background())

	assert.EqualError(t, err, "connection reset")
}

func TestConn_ListSchemas_ExcludesSystemSchemas(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT schema_name FROM information_schema.schemata`).
		WillReturnRows(pgxmock.NewRows([]string{"schema_name"}).AddRow("public").AddRow("app"))

	c := &Conn{conn: mock}
	got, err := c.ListSchemas(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"public", "app"}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestConn_ListTables_PassesSchemaAsArg(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT table_name FROM information_schema.tables`).
		WithArgs("public").
		WillReturnRows(pgxmock.NewRows([]string{"table_name"}).AddRow("users").AddRow("orders"))

	c := &Conn{conn: mock}
	got, err := c.ListTables(context.Background(), "public")

	require.NoError(t, err)
	assert.Equal(t, []string{"users", "orders"}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestConn_ListIndexes_PassesSchemaAndTableAsArgs(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT indexname, indexdef FROM pg_indexes`).
		WithArgs("public", "users").
		WillReturnRows(pgxmock.NewRows([]string{"indexname", "indexdef"}).
			AddRow("users_pkey", "CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)").
			AddRow("users_email_idx", "CREATE INDEX users_email_idx ON public.users USING btree (email)"))

	c := &Conn{conn: mock}
	got, err := c.ListIndexes(context.Background(), "public", "users")

	require.NoError(t, err)
	assert.Equal(t, []IndexInfo{
		{Name: "users_pkey", Definition: "CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)"},
		{Name: "users_email_idx", Definition: "CREATE INDEX users_email_idx ON public.users USING btree (email)"},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestConn_ListIndexes_NoIndexes(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT indexname, indexdef FROM pg_indexes`).
		WithArgs("public", "unindexed").
		WillReturnRows(pgxmock.NewRows([]string{"indexname", "indexdef"}))

	c := &Conn{conn: mock}
	got, err := c.ListIndexes(context.Background(), "public", "unindexed")

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestConn_ListIndexes_PropagatesQueryError(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT indexname, indexdef FROM pg_indexes`).
		WithArgs("public", "users").
		WillReturnError(errors.New("permission denied"))

	c := &Conn{conn: mock}
	_, err := c.ListIndexes(context.Background(), "public", "users")

	assert.EqualError(t, err, "permission denied")
}

func TestConn_RunQuery_BuildsColumnsAndRows(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT id, name FROM users`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).
			AddRow(int32(1), "bob").
			AddRow(int32(2), nil))

	c := &Conn{conn: mock}
	got, err := c.RunQuery(context.Background(), "SELECT id, name FROM users")

	require.NoError(t, err)
	assert.Equal(t, []string{"id", "name"}, got.Columns)
	assert.Equal(t, [][]string{{"1", "bob"}, {"2", "NULL"}}, got.Rows)
}

func TestConn_RunQuery_NoRows(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT 1 WHERE false`).
		WillReturnRows(pgxmock.NewRows([]string{"?column?"}))

	c := &Conn{conn: mock}
	got, err := c.RunQuery(context.Background(), "SELECT 1 WHERE false")

	require.NoError(t, err)
	assert.Equal(t, []string{"?column?"}, got.Columns)
	assert.Empty(t, got.Rows)
}

func TestConn_RunQuery_PropagatesQueryError(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT \* FROM nope`).
		WillReturnError(errors.New(`relation "nope" does not exist`))

	c := &Conn{conn: mock}
	_, err := c.RunQuery(context.Background(), "SELECT * FROM nope")

	assert.EqualError(t, err, `relation "nope" does not exist`)
}

func TestConn_RunQuery_PropagatesRowError(t *testing.T) {
	mock := newMockConn(t)
	mock.ExpectQuery(`SELECT id FROM users`).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow(int32(1)).
			RowError(0, errors.New("boom")))

	c := &Conn{conn: mock}
	_, err := c.RunQuery(context.Background(), "SELECT id FROM users")

	assert.Error(t, err)
}

// --- Connect / SwitchDatabase / Close, via a hand-rolled fake ---------

// fakePgxConn is a minimal pgxConn implementation for exercising Conn's
// connection lifecycle (as opposed to query building, which the pgxmock
// tests above cover) without a real network connection. Its Query method
// is never expected to be called by these tests.
type fakePgxConn struct {
	closed bool
}

func (f *fakePgxConn) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	panic("fakePgxConn.Query should not be called in lifecycle tests")
}

func (f *fakePgxConn) Close(ctx context.Context) error {
	f.closed = true
	return nil
}

// stubConnectFn swaps the package-level connectFn for the duration of a
// test and returns a func to restore it.
func stubConnectFn(t *testing.T, fn func(ctx context.Context, dsn string) (pgxConn, error)) {
	t.Helper()
	orig := connectFn
	connectFn = fn
	t.Cleanup(func() { connectFn = orig })
}

func TestConn_Connect_StoresConnectionAndDatabaseName(t *testing.T) {
	spy := &fakePgxConn{}
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		assert.Equal(t, "postgres://host/mydb", dsn)
		return spy, nil
	})

	c, err := Connect(context.Background(), "postgres://host/mydb", "mydb")

	require.NoError(t, err)
	assert.Equal(t, "mydb", c.db)
	assert.Equal(t, "postgres://host/mydb", c.dsn)
	assert.Same(t, spy, c.conn)
}

func TestConn_Connect_PropagatesDialError(t *testing.T) {
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		return nil, errors.New("dial refused")
	})

	_, err := Connect(context.Background(), "postgres://host/mydb", "mydb")

	assert.EqualError(t, err, "dial refused")
}

func TestConn_SwitchDatabase_NoOpWhenAlreadyOnTargetDatabase(t *testing.T) {
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		t.Fatal("connectFn should not be called when the database is unchanged")
		return nil, nil
	})

	c := &Conn{db: "mydb"}
	err := c.SwitchDatabase(context.Background(), "mydb")

	assert.NoError(t, err)
}

func TestConn_SwitchDatabase_ReconnectsAndClosesOldConnection(t *testing.T) {
	old := &fakePgxConn{}
	next := &fakePgxConn{}
	var dialedDSN string
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		dialedDSN = dsn
		return next, nil
	})

	c := &Conn{conn: old, dsn: "postgres://host/old?sslmode=disable", db: "old"}
	err := c.SwitchDatabase(context.Background(), "new")

	require.NoError(t, err)
	assert.True(t, old.closed, "old connection should be closed after switching")
	assert.Same(t, next, c.conn)
	assert.Equal(t, "new", c.db)
	assert.Equal(t, "postgres://host/new?sslmode=disable", c.dsn)
	assert.Equal(t, "postgres://host/new?sslmode=disable", dialedDSN, "should dial the rewritten dsn pointing at the new db")
}

func TestConn_SwitchDatabase_PropagatesConnectErrorAndKeepsOldConnection(t *testing.T) {
	old := &fakePgxConn{}
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		return nil, errors.New("no such database")
	})

	c := &Conn{conn: old, dsn: "postgres://host/old", db: "old"}
	err := c.SwitchDatabase(context.Background(), "new")

	assert.EqualError(t, err, "no such database")
	assert.False(t, old.closed, "should keep the working connection if the reconnect failed")
	assert.Same(t, old, c.conn)
	assert.Equal(t, "old", c.db)
}

func TestConn_SwitchDatabase_InvalidDSNDoesNotDial(t *testing.T) {
	stubConnectFn(t, func(ctx context.Context, dsn string) (pgxConn, error) {
		t.Fatal("connectFn should not be called when the DSN can't be parsed")
		return nil, nil
	})

	c := &Conn{conn: &fakePgxConn{}, dsn: "postgres://%zz", db: "old"}
	err := c.SwitchDatabase(context.Background(), "new")

	assert.Error(t, err)
}

func TestConn_Close_ClosesUnderlyingConnection(t *testing.T) {
	spy := &fakePgxConn{}
	c := &Conn{conn: spy}

	c.Close(context.Background())

	assert.True(t, spy.closed)
}

func TestConn_Close_NilConnectionIsNoop(t *testing.T) {
	c := &Conn{}
	assert.NotPanics(t, func() { c.Close(context.Background()) })
}
