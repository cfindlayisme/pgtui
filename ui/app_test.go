package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cfindlayisme/pgtui/browser"
	"github.com/cfindlayisme/pgtui/db"
)

// fakeDB is the same tiny in-memory Postgres stand-in used by the browser
// package's tests, letting the UI's tree/table wiring be exercised without
// a live database or a real terminal screen.
type fakeDB struct {
	currentDB string

	databases []string
	schemas   map[string][]string
	tables    map[string]map[string][]string
	indexes   map[string]map[string]map[string][]db.IndexInfo
	results   map[string]*db.QueryResult

	queryErr   error
	indexesErr error
}

func (f *fakeDB) ListDatabases(ctx context.Context) ([]string, error) { return f.databases, nil }
func (f *fakeDB) ListSchemas(ctx context.Context) ([]string, error) {
	return f.schemas[f.currentDB], nil
}
func (f *fakeDB) ListTables(ctx context.Context, schema string) ([]string, error) {
	return f.tables[f.currentDB][schema], nil
}
func (f *fakeDB) ListIndexes(ctx context.Context, schema, table string) ([]db.IndexInfo, error) {
	if f.indexesErr != nil {
		return nil, f.indexesErr
	}
	return f.indexes[f.currentDB][schema][table], nil
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
	a, err := newAppWithBrowser(context.Background(), browser.New(fake), "")
	require.NoError(t, err)
	return a
}

// expandToTable drives the tree down to a single table node: expand the
// (only) database, then its (only) schema, returning the table node.
func expandToTable(a *App) *tview.TreeNode {
	dbNode := a.tree.GetRoot().GetChildren()[0]
	a.onTreeSelect(dbNode)
	schemaNode := dbNode.GetChildren()[0]
	a.onTreeSelect(schemaNode)
	return schemaNode.GetChildren()[0]
}

func TestNewApp_PopulatesDatabaseNodesFromBrowser(t *testing.T) {
	fake := &fakeDB{databases: []string{"alpha", "beta"}}

	a := newTestApp(t, fake)

	children := a.tree.GetRoot().GetChildren()
	require.Len(t, children, 2)
	assert.Equal(t, databaseIcon+" alpha", children[0].GetText())
	assert.Equal(t, databaseIcon+" beta", children[1].GetText())
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
	assert.Equal(t, schemaIcon+" public", schemaNodes[0].GetText())
	assert.Equal(t, schemaIcon+" app", schemaNodes[1].GetText())
	assert.True(t, dbNode.IsExpanded())

	// Selecting again should just toggle expansion, not reload/duplicate children.
	a.onTreeSelect(dbNode)
	assert.False(t, dbNode.IsExpanded())
	assert.Len(t, dbNode.GetChildren(), 2)
}

func TestOnTreeSelect_TableNodeOpensOptionsAndLoadsIndexes(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		indexes: map[string]map[string]map[string][]db.IndexInfo{
			"alpha": {"public": {"users": {
				{Name: "users_pkey", Definition: "CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)"},
			}}},
		},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	require.Equal(t, tableIcon+" users", tableNode.GetText())

	a.onTreeSelect(tableNode)

	assert.True(t, a.optionsOpen, "selecting a table should open the query-options prompt")
	assert.Equal(t, " public.users ", a.optionsList.GetTitle())
	assert.Equal(t, 5, a.optionsList.GetItemCount(), "preview 100/1000, count, columns, cancel")
	assert.Contains(t, a.indexPanel.GetText(true), "users_pkey")
	assert.Contains(t, a.indexPanel.GetText(true), "CREATE UNIQUE INDEX")

	// Nothing should have been queried yet -- that only happens once an
	// option is picked.
	assert.Empty(t, a.queryBar.GetText())
}

func TestOnTreeSelect_TableNodeWithNoIndexes(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)

	a.onTreeSelect(tableNode)

	assert.Contains(t, a.indexPanel.GetText(true), "no indexes")
}

func TestOnTreeSelect_IndexLoadErrorShowsInIndexPanel(t *testing.T) {
	fake := &fakeDB{
		databases:  []string{"alpha"},
		schemas:    map[string][]string{"alpha": {"public"}},
		tables:     map[string]map[string][]string{"alpha": {"public": {"users"}}},
		indexesErr: errors.New("permission denied"),
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)

	a.onTreeSelect(tableNode)

	assert.Contains(t, a.indexPanel.GetText(true), "permission denied")
}

func TestTableOptions_Preview100RowsRunsDefaultQuery(t *testing.T) {
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
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(0)() // "Preview 100 rows"

	assert.False(t, a.optionsOpen, "picking an option should close the prompt")
	assert.Equal(t, `SELECT * FROM "public"."users" LIMIT 100`, a.queryBar.GetText())
	assert.Equal(t, " Results (2 rows) ", a.table.GetTitle())
	assert.Equal(t, "id", a.table.GetCell(0, 0).Text)
	assert.Equal(t, "bob", a.table.GetCell(1, 1).Text)
	assert.Equal(t, "alice", a.table.GetCell(2, 1).Text)
	assert.Same(t, a.table, a.tv.GetFocus(), "should jump straight to the results after picking an option")
}

func TestTableOptions_Preview1000Rows(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		results:   map[string]*db.QueryResult{`SELECT * FROM "public"."users" LIMIT 1000`: want},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(1)() // "Preview 1000 rows"

	assert.Equal(t, `SELECT * FROM "public"."users" LIMIT 1000`, a.queryBar.GetText())
}

func TestTableOptions_RowCount(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"count"}, Rows: [][]string{{"42"}}}
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		results:   map[string]*db.QueryResult{`SELECT COUNT(*) FROM "public"."users"`: want},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(2)() // "Row count"

	assert.Equal(t, `SELECT COUNT(*) FROM "public"."users"`, a.queryBar.GetText())
	assert.Equal(t, "42", a.table.GetCell(1, 0).Text)
}

func TestTableOptions_Columns(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"column_name"}, Rows: [][]string{{"id"}}}
	query := browser.ColumnsQuery("public", "users")
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		results:   map[string]*db.QueryResult{query: want},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(3)() // "Columns"

	assert.Equal(t, query, a.queryBar.GetText())
	assert.Equal(t, "id", a.table.GetCell(1, 0).Text)
}

func TestTableOptions_CancelClosesPromptWithoutQuerying(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(4)() // "Cancel"

	assert.False(t, a.optionsOpen)
	assert.Empty(t, a.queryBar.GetText())
	assert.Same(t, a.tree, a.tv.GetFocus(), "cancelling should return focus to the tree, not the results")
}

func TestTableOptions_QueryErrorShowsInResultsPane(t *testing.T) {
	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		queryErr:  errors.New("permission denied"),
	}
	a := newTestApp(t, fake)
	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	a.optionsList.GetItemSelectedFunc(0)()

	assert.Contains(t, a.table.GetCell(0, 0).Text, "permission denied")
	assert.Same(t, a.table, a.tv.GetFocus(), "should still land on the results even when the query errored")
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

func TestOnQueryBarDone_EnterRunsQueryAndFocusesResults(t *testing.T) {
	want := &db.QueryResult{Columns: []string{"one"}, Rows: [][]string{{"1"}}}
	fake := &fakeDB{
		results: map[string]*db.QueryResult{"SELECT 1 AS one": want},
	}
	a := newTestApp(t, fake)
	a.queryBar.SetText("SELECT 1 AS one")

	a.onQueryBarDone(tcell.KeyEnter)

	assert.Equal(t, "one", a.table.GetCell(0, 0).Text)
	assert.Same(t, a.table, a.tv.GetFocus())
}

func TestOnQueryBarDone_EscReturnsFocusToTreeWithoutRunning(t *testing.T) {
	fake := &fakeDB{}
	a := newTestApp(t, fake)
	a.queryBar.SetText("SELECT should not run")

	a.onQueryBarDone(tcell.KeyEsc)

	assert.Same(t, a.tree, a.tv.GetFocus())
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
