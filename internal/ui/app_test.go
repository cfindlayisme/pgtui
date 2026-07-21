package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cfindlayisme/pgtui/internal/browser"
	"github.com/cfindlayisme/pgtui/internal/db"
)

// fakeDB is the same tiny in-memory Postgres stand-in used by the browser
// package's tests, letting the UI's tree/table wiring be exercised without
// a live database or a real terminal screen.
type fakeDB struct {
	currentDB string

	databases []string
	schemas   map[string][]string
	tables    map[string]map[string][]string
	results   map[string]*db.QueryResult

	queryErr error
}

func (f *fakeDB) ListDatabases(ctx context.Context) ([]string, error) { return f.databases, nil }
func (f *fakeDB) ListSchemas(ctx context.Context) ([]string, error) {
	return f.schemas[f.currentDB], nil
}
func (f *fakeDB) ListTables(ctx context.Context, schema string) ([]string, error) {
	return f.tables[f.currentDB][schema], nil
}
func (f *fakeDB) RunQuery(ctx context.Context, sql string) (*db.QueryResult, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.results[sql], nil
}
func (f *fakeDB) SwitchDatabase(ctx context.Context, dbname string) error {
	f.currentDB = dbname
	return nil
}
func (f *fakeDB) Close(ctx context.Context) {}

func newTestApp(t *testing.T, fake *fakeDB) *App {
	t.Helper()
	a, err := newAppWithBrowser(context.Background(), browser.New(fake))
	require.NoError(t, err)
	return a
}

func TestNewApp_PopulatesDatabaseNodesFromBrowser(t *testing.T) {
	fake := &fakeDB{databases: []string{"alpha", "beta"}}

	a := newTestApp(t, fake)

	children := a.tree.GetRoot().GetChildren()
	require.Len(t, children, 2)
	assert.Equal(t, "alpha", children[0].GetText())
	assert.Equal(t, "beta", children[1].GetText())
}

func TestOnTreeSelect_DatabaseNodeLoadsSchemasOnce(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public", "app"}},
	}
	a := newTestApp(t, fake)
	dbNode := a.tree.GetRoot().GetChildren()[0]

	a.onTreeSelect(dbNode)

	schemaNodes := dbNode.GetChildren()
	require.Len(t, schemaNodes, 2)
	assert.Equal(t, "public", schemaNodes[0].GetText())
	assert.Equal(t, "app", schemaNodes[1].GetText())
	assert.True(t, dbNode.IsExpanded())

	// Selecting again should just toggle expansion, not reload/duplicate children.
	a.onTreeSelect(dbNode)
	assert.False(t, dbNode.IsExpanded())
	assert.Len(t, dbNode.GetChildren(), 2)
}

func TestOnTreeSelect_TableNodeRunsDefaultQueryAndFillsResults(t *testing.T) {
	want := &db.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]string{{"1", "bob"}, {"2", "alice"}},
	}
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		results:   map[string]*db.QueryResult{`SELECT * FROM "public"."users" LIMIT 100`: want},
	}
	a := newTestApp(t, fake)
	dbNode := a.tree.GetRoot().GetChildren()[0]
	a.onTreeSelect(dbNode) // expand -> loads schema

	schemaNode := dbNode.GetChildren()[0]
	a.onTreeSelect(schemaNode) // expand -> loads table

	tableNode := schemaNode.GetChildren()[0]
	a.onTreeSelect(tableNode) // run default query

	assert.Equal(t, `SELECT * FROM "public"."users" LIMIT 100`, a.queryBar.GetText())
	assert.Equal(t, " Results (2 rows) ", a.table.GetTitle())
	assert.Equal(t, "id", a.table.GetCell(0, 0).Text)
	assert.Equal(t, "bob", a.table.GetCell(1, 1).Text)
	assert.Equal(t, "alice", a.table.GetCell(2, 1).Text)
}

func TestOnTreeSelect_QueryErrorShowsInResultsPane(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		queryErr:  errors.New("permission denied"),
	}
	a := newTestApp(t, fake)
	dbNode := a.tree.GetRoot().GetChildren()[0]
	a.onTreeSelect(dbNode)
	schemaNode := dbNode.GetChildren()[0]
	a.onTreeSelect(schemaNode)
	tableNode := schemaNode.GetChildren()[0]

	a.onTreeSelect(tableNode)

	assert.Contains(t, a.table.GetCell(0, 0).Text, "permission denied")
}

func TestRunQuery_CustomSQLFromQueryBar(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"one"}, Rows: [][]string{{"1"}}}
	fake := &fakeDB{
		results: map[string]*db.QueryResult{"SELECT 1 AS one": want},
	}
	a := newTestApp(t, fake)

	a.runQuery("SELECT 1 AS one")

	assert.Equal(t, "SELECT 1 AS one", a.queryBar.GetText())
	assert.Equal(t, "one", a.table.GetCell(0, 0).Text)
	assert.Equal(t, "1", a.table.GetCell(1, 0).Text)
}

func TestRunQuery_BlankInputIsIgnored(t *testing.T) {
	fake := &fakeDB{}
	a := newTestApp(t, fake)
	a.queryBar.SetText("previous query")

	a.table.SetCell(0, 0, tview.NewTableCell("untouched"))
	a.runQuery("   ")

	assert.Equal(t, "previous query", a.queryBar.GetText())
	assert.Equal(t, "untouched", a.table.GetCell(0, 0).Text)
}
