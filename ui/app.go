// Package ui is the tview presentation layer: a database/schema/table
// tree on the left (with an index panel underneath), a results table on
// the right, and an always-visible query bar at the bottom. All Postgres
// decisions are delegated to browser; this package only renders
// and wires up input.
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/cfindlayisme/pgtui/browser"
	"github.com/cfindlayisme/pgtui/db"
	"github.com/cfindlayisme/pgtui/translations"
)

type nodeKind int

const (
	kindDatabase nodeKind = iota
	kindSchema
	kindTable
)

// Tags prefixed onto tree node text so database/schema/table nodes are
// visually distinguishable at a glance, on top of their distinct colors.
// Plain ASCII, no square brackets: emoji/symbol glyphs are ambiguous-width
// in many terminals (corrupts tview's cursor math), and tview parses
// "[...]" in node text as color/region tags rather than literal text --
// both confirmed by actually rendering the tree during development.
const (
	databaseIcon = "DB:"
	schemaIcon   = "SCHEMA:"
	tableIcon    = "TABLE:"
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
	tv          *tview.Application
	pages       *tview.Pages
	tree        *tview.TreeView
	indexPanel  *tview.TextView
	table       *tview.Table
	queryBar    *tview.InputField
	optionsList *tview.List
	layout      *tview.Flex

	optionsOpen bool

	ctx context.Context
	br  *browser.Browser
}

// NewApp connects to dsn (which must already point at database) and
// builds the full application.
func NewApp(ctx context.Context, dsn, database string) (*App, error) {
	conn, err := db.Connect(ctx, dsn, database)
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
	a.buildIndexPanel()
	a.buildTable()
	a.buildQueryBar()
	a.buildOptionsList()
	a.buildLayout()
	a.setKeybindings()

	if err := a.loadDatabases(); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *App) buildTree() {
	root := tview.NewTreeNode(translations.T("ui.tree_root"))
	a.tree = tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root).
		SetTopLevel(1)
	a.tree.SetBorder(true).SetTitle(" pgtui ")
	a.tree.SetSelectedFunc(a.onTreeSelect)
}

func (a *App) buildIndexPanel() {
	a.indexPanel = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	a.indexPanel.SetBorder(true).SetTitle(translations.T("ui.indexes_title"))
}

func (a *App) buildTable() {
	a.table = tview.NewTable().SetFixed(1, 0)
	a.table.SetSelectable(true, false)
	a.table.SetBorder(true).SetTitle(translations.T("ui.results_title"))
}

func (a *App) buildQueryBar() {
	a.queryBar = tview.NewInputField().SetLabel("SQL> ")
	a.queryBar.SetBorder(true).SetTitle(translations.T("ui.query_title"))
	a.queryBar.SetDoneFunc(a.onQueryBarDone)
}

func (a *App) buildOptionsList() {
	a.optionsList = tview.NewList().ShowSecondaryText(true)
	a.optionsList.SetBorder(true)
	a.optionsList.SetDoneFunc(a.cancelTableOptions)
}

func (a *App) buildLayout() {
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.tree, 0, 3, true).
		AddItem(a.indexPanel, 0, 1, false)

	top := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 40, 1, true).
		AddItem(a.table, 0, 3, false)

	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(top, 0, 1, true).
		AddItem(a.queryBar, 3, 0, false)

	a.pages = tview.NewPages().
		AddPage("main", a.layout, true, true).
		AddPage("options", modalCenter(a.optionsList, 60, 12), true, false)
}

// modalCenter wraps p in nested Flexes so it renders as a fixed-size box
// centered over whatever's beneath it in a Pages stack.
func modalCenter(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func (a *App) setKeybindings() {
	a.tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			a.tv.Stop()
			return nil
		}
		// While the table-options modal is open, leave every other key to
		// the list itself (digit shortcuts, arrows, Enter, Esc-to-cancel).
		if a.optionsOpen {
			return event
		}
		if event.Key() == tcell.KeyTab {
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
		a.tv.SetFocus(a.indexPanel)
	case a.indexPanel:
		a.tv.SetFocus(a.queryBar)
	default:
		a.tv.SetFocus(a.tree)
	}
}

// Run starts the terminal event loop. It blocks until the user quits.
func (a *App) Run() error {
	defer a.br.DB.Close(a.ctx)
	return a.tv.SetRoot(a.pages, true).SetFocus(a.tree).Run()
}

func (a *App) loadDatabases() error {
	dbs, err := a.br.Databases(a.ctx)
	if err != nil {
		return err
	}
	root := a.tree.GetRoot()
	for _, name := range dbs {
		node := tview.NewTreeNode(databaseIcon + " " + name).
			SetReference(&nodeData{kind: kindDatabase, dbname: name}).
			SetColor(tcell.ColorYellow).
			SetTextStyle(tcell.StyleDefault.Bold(true))
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
		a.selectTable(ref)
	}
}

func (a *App) loadSchemas(node *tview.TreeNode, ref *nodeData) error {
	schemas, err := a.br.Schemas(a.ctx, ref.dbname)
	if err != nil {
		return err
	}
	for _, s := range schemas {
		child := tview.NewTreeNode(schemaIcon + " " + s).
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
		child := tview.NewTreeNode(tableIcon + " " + t).
			SetReference(&nodeData{kind: kindTable, dbname: ref.dbname, schema: ref.schema, table: t}).
			SetColor(tcell.ColorAqua)
		node.AddChild(child)
	}
	ref.loaded = true
	return nil
}

// selectTable is what Enter on a table node does: it refreshes the index
// panel and prompts for which query to run, rather than guessing.
func (a *App) selectTable(ref *nodeData) {
	a.loadIndexes(ref)
	a.showTableOptions(ref)
}

func (a *App) loadIndexes(ref *nodeData) {
	a.indexPanel.SetTitle(translations.T("ui.indexes_title_for", ref.schema, ref.table))

	indexes, err := a.br.Indexes(a.ctx, ref.dbname, ref.schema, ref.table)
	if err != nil {
		a.indexPanel.SetText("[red]" + tview.Escape(err.Error()))
		return
	}
	if len(indexes) == 0 {
		a.indexPanel.SetText("[gray]" + translations.T("ui.no_indexes"))
		return
	}

	var b strings.Builder
	for i, idx := range indexes {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[yellow]%s[white]\n%s", tview.Escape(idx.Name), tview.Escape(idx.Definition))
	}
	a.indexPanel.SetText(b.String())
}

// showTableOptions pops up a small menu of common queries for the
// selected table instead of guessing which one the user wants.
func (a *App) showTableOptions(ref *nodeData) {
	a.optionsList.Clear()
	a.optionsList.SetTitle(fmt.Sprintf(" %s.%s ", ref.schema, ref.table))

	a.optionsList.AddItem(translations.T("ui.option.preview_100"), translations.T("ui.option.preview_100_desc"), '1', func() {
		a.runQueryForTable(ref, browser.PreviewQuery(ref.schema, ref.table, 100))
	})
	a.optionsList.AddItem(translations.T("ui.option.preview_1000"), translations.T("ui.option.preview_1000_desc"), '2', func() {
		a.runQueryForTable(ref, browser.PreviewQuery(ref.schema, ref.table, 1000))
	})
	a.optionsList.AddItem(translations.T("ui.option.row_count"), translations.T("ui.option.row_count_desc"), '3', func() {
		a.runQueryForTable(ref, browser.CountQuery(ref.schema, ref.table))
	})
	a.optionsList.AddItem(translations.T("ui.option.columns"), translations.T("ui.option.columns_desc"), '4', func() {
		a.runQueryForTable(ref, browser.ColumnsQuery(ref.schema, ref.table))
	})
	a.optionsList.AddItem(translations.T("ui.option.cancel"), "", 'c', a.cancelTableOptions)

	a.optionsOpen = true
	a.pages.ShowPage("options")
	a.tv.SetFocus(a.optionsList)
}

// hideTableOptions closes the modal without deciding where focus goes
// next -- callers do that, since it differs between cancelling (back to
// the tree) and picking an option (on to the results).
func (a *App) hideTableOptions() {
	a.optionsOpen = false
	a.pages.HidePage("options")
}

// cancelTableOptions is what Esc or "Cancel" in the options modal does:
// close it and return focus to the tree, without running anything.
func (a *App) cancelTableOptions() {
	a.hideTableOptions()
	a.tv.SetFocus(a.tree)
}

func (a *App) runQueryForTable(ref *nodeData, query string) {
	a.hideTableOptions()
	a.queryBar.SetText(query)
	result, err := a.br.RunQuery(a.ctx, ref.dbname, query)
	if err != nil {
		a.showError(err)
		a.tv.SetFocus(a.table)
		return
	}
	a.setResults(result)
	a.tv.SetFocus(a.table)
}

// onQueryBarDone handles Enter/Esc in the query bar: Enter runs whatever
// SQL is typed and jumps focus to the results; anything else (Esc) just
// returns focus to the tree without running anything.
func (a *App) onQueryBarDone(key tcell.Key) {
	if key == tcell.KeyEnter {
		a.runQuery(a.queryBar.GetText())
		a.tv.SetFocus(a.table)
		return
	}
	a.tv.SetFocus(a.tree)
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
	a.table.SetTitle(translations.T("ui.results_title_rows", len(result.Rows)))
	a.table.ScrollToBeginning()
}

func (a *App) showError(err error) {
	a.table.Clear()
	a.table.SetTitle(translations.T("ui.results_title"))
	a.table.SetCell(0, 0, tview.NewTableCell(translations.T("ui.error_prefix")+err.Error()).SetTextColor(tcell.ColorRed))
}
