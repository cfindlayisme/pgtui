// Package ui is the tview presentation layer: a database/schema/table
// tree on the left, a results table on the right, and an always-visible
// query bar at the bottom. All Postgres decisions are delegated to
// internal/browser; this package only renders and wires up input.
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/cfindlayisme/pgtui/internal/browser"
	"github.com/cfindlayisme/pgtui/internal/db"
)

type nodeKind int

const (
	kindDatabase nodeKind = iota
	kindSchema
	kindTable
)

// nodeData is stashed on each tree node via SetReference so selection
// handlers know what the node represents and whether it's been expanded
// (and thus queried) before.
type nodeData struct {
	kind   nodeKind
	dbname string
	schema string
	table  string
	loaded bool
}

type App struct {
	tv       *tview.Application
	tree     *tview.TreeView
	table    *tview.Table
	queryBar *tview.InputField
	layout   *tview.Flex

	ctx context.Context
	br  *browser.Browser
}

// NewApp connects to dsn and builds the full application.
func NewApp(ctx context.Context, dsn string) (*App, error) {
	conn, err := db.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	a, err := newAppWithBrowser(ctx, browser.New(conn))
	if err != nil {
		conn.Close(ctx)
		return nil, err
	}
	return a, nil
}

// newAppWithBrowser builds the application against an already-constructed
// Browser, so tests can inject a fake DB without a live connection.
func newAppWithBrowser(ctx context.Context, br *browser.Browser) (*App, error) {
	a := &App{
		tv:  tview.NewApplication(),
		ctx: ctx,
		br:  br,
	}

	a.buildTree()
	a.buildTable()
	a.buildQueryBar()
	a.buildLayout()
	a.setKeybindings()

	if err := a.loadDatabases(); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *App) buildTree() {
	root := tview.NewTreeNode("Databases")
	a.tree = tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root).
		SetTopLevel(1)
	a.tree.SetBorder(true).SetTitle(" pgtui ")
	a.tree.SetSelectedFunc(a.onTreeSelect)
}

func (a *App) buildTable() {
	a.table = tview.NewTable().SetFixed(1, 0)
	a.table.SetSelectable(true, false)
	a.table.SetBorder(true).SetTitle(" Results ")
}

func (a *App) buildQueryBar() {
	a.queryBar = tview.NewInputField().SetLabel("SQL> ")
	a.queryBar.SetBorder(true).SetTitle(" Query  [Enter: run  Esc: cancel  ':' to focus  Tab: switch panel  q: quit] ")
	a.queryBar.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			a.runQuery(a.queryBar.GetText())
		}
		a.tv.SetFocus(a.tree)
	})
}

func (a *App) buildLayout() {
	top := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.tree, 40, 1, true).
		AddItem(a.table, 0, 3, false)

	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(top, 0, 1, true).
		AddItem(a.queryBar, 3, 0, false)
}

func (a *App) setKeybindings() {
	a.tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			a.tv.Stop()
			return nil
		case tcell.KeyTab:
			a.cycleFocus()
			return nil
		}
		if a.tv.GetFocus() != a.queryBar {
			switch event.Rune() {
			case ':':
				a.tv.SetFocus(a.queryBar)
				return nil
			case 'q':
				a.tv.Stop()
				return nil
			}
		}
		return event
	})
}

func (a *App) cycleFocus() {
	switch a.tv.GetFocus() {
	case a.tree:
		a.tv.SetFocus(a.table)
	case a.table:
		a.tv.SetFocus(a.queryBar)
	default:
		a.tv.SetFocus(a.tree)
	}
}

// Run starts the terminal event loop. It blocks until the user quits.
func (a *App) Run() error {
	defer a.br.DB.Close(a.ctx)
	return a.tv.SetRoot(a.layout, true).SetFocus(a.tree).Run()
}

func (a *App) loadDatabases() error {
	dbs, err := a.br.Databases(a.ctx)
	if err != nil {
		return err
	}
	root := a.tree.GetRoot()
	for _, name := range dbs {
		node := tview.NewTreeNode(name).
			SetReference(&nodeData{kind: kindDatabase, dbname: name}).
			SetColor(tcell.ColorYellow)
		root.AddChild(node)
	}
	return nil
}

func (a *App) onTreeSelect(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*nodeData)
	if !ok {
		return
	}

	switch ref.kind {
	case kindDatabase:
		if ref.loaded {
			node.SetExpanded(!node.IsExpanded())
			return
		}
		if err := a.loadSchemas(node, ref); err != nil {
			a.showError(err)
			return
		}
		// A freshly loaded node has no meaningful prior expansion state
		// (tview's default is "expanded" even with zero children), so
		// force it open rather than toggling.
		node.SetExpanded(true)

	case kindSchema:
		if ref.loaded {
			node.SetExpanded(!node.IsExpanded())
			return
		}
		if err := a.loadTables(node, ref); err != nil {
			a.showError(err)
			return
		}
		node.SetExpanded(true)

	case kindTable:
		a.runTableQuery(ref)
	}
}

func (a *App) loadSchemas(node *tview.TreeNode, ref *nodeData) error {
	schemas, err := a.br.Schemas(a.ctx, ref.dbname)
	if err != nil {
		return err
	}
	for _, s := range schemas {
		child := tview.NewTreeNode(s).
			SetReference(&nodeData{kind: kindSchema, dbname: ref.dbname, schema: s}).
			SetColor(tcell.ColorGreen)
		node.AddChild(child)
	}
	ref.loaded = true
	return nil
}

func (a *App) loadTables(node *tview.TreeNode, ref *nodeData) error {
	tables, err := a.br.Tables(a.ctx, ref.dbname, ref.schema)
	if err != nil {
		return err
	}
	for _, t := range tables {
		child := tview.NewTreeNode(t).
			SetReference(&nodeData{kind: kindTable, dbname: ref.dbname, schema: ref.schema, table: t})
		node.AddChild(child)
	}
	ref.loaded = true
	return nil
}

func (a *App) runTableQuery(ref *nodeData) {
	q := browser.TableQuery(ref.schema, ref.table)
	a.queryBar.SetText(q)
	result, err := a.br.RunQuery(a.ctx, ref.dbname, q)
	if err != nil {
		a.showError(err)
		return
	}
	a.setResults(result)
}

// runQuery executes q against whichever database is currently connected
// and renders the result in the right-hand table. The query bar always
// reflects the query that produced what's on screen.
func (a *App) runQuery(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	a.queryBar.SetText(q)

	result, err := a.br.DB.RunQuery(a.ctx, q)
	if err != nil {
		a.showError(err)
		return
	}
	a.setResults(result)
}

func (a *App) setResults(result *db.QueryResult) {
	a.table.Clear()
	for c, col := range result.Columns {
		cell := tview.NewTableCell(col).
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetAttributes(tcell.AttrBold)
		a.table.SetCell(0, c, cell)
	}
	for r, row := range result.Rows {
		for c, val := range row {
			a.table.SetCell(r+1, c, tview.NewTableCell(val))
		}
	}
	a.table.SetTitle(fmt.Sprintf(" Results (%d rows) ", len(result.Rows)))
	a.table.ScrollToBeginning()
}

func (a *App) showError(err error) {
	a.table.Clear()
	a.table.SetTitle(" Results ")
	a.table.SetCell(0, 0, tview.NewTableCell("Error: "+err.Error()).SetTextColor(tcell.ColorRed))
}
