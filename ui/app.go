// Package ui is the tview presentation layer: a database/schema/table
// tree on the left (with an index panel underneath), a results table on
// the right, and an always-visible query bar at the bottom. All Postgres
// decisions are delegated to browser; this package only renders
// and wires up input.
package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/cfindlayisme/pgtui/browser"
	"github.com/cfindlayisme/pgtui/db"
	"github.com/cfindlayisme/pgtui/translations"
)

// highlightSwitchDelay is how long the tree cursor must sit still on a
// database node before its SwitchDatabase reconnect actually fires. Without
// this, holding an arrow key down to scroll past several databases fired a
// live reconnect per node passed over, stalling the UI thread with a
// network round-trip on every step. The delay is short enough that a
// single, deliberate move still feels immediate.
const highlightSwitchDelay = 150 * time.Millisecond

// Forced to pure black rather than left at the terminal's default, in the
// spirit of k9s's always-black chrome -- keeps the header/border colors
// consistent regardless of the terminal theme it runs in.
func init() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorBlack
	tview.Styles.ContrastBackgroundColor = tcell.ColorBlack
	tview.Styles.MoreContrastBackgroundColor = tcell.ColorBlack
}

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
	tv           *tview.Application
	pages        *tview.Pages
	header       *tview.Flex
	headerInfo   *tview.TextView
	headerLegend *tview.TextView
	headerLogo   *tview.TextView
	tree         *tview.TreeView
	indexPanel   *tview.TextView
	resultsPages *tview.Pages
	table        *tview.Table
	resultsText  *tview.TextView
	queryBar     *tview.InputField
	optionsList  *tview.List
	layout       *tview.Flex

	optionsOpen bool
	wrapped     bool

	// Base titles for the panels that get a scroll indicator appended
	// (see refreshScrollIndicators) -- kept separate from what's actually
	// on screen so appending "^"/"v" never has to be undone.
	treeTitle    string
	indexTitle   string
	resultsTitle string

	host     string
	user     string
	database string

	// highlightGen is bumped on every tree highlight change and captured
	// by the pending debounce timer (see highlightSwitchDelay); a timer
	// that fires after being superseded by a newer highlight compares its
	// captured value against the current one and no-ops if they differ.
	highlightGen int

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
	a, err := newAppWithBrowser(ctx, browser.New(conn), database)
	if err != nil {
		conn.Close(ctx)
		return nil, err
	}
	// Parsed only for the header's Host/User display -- never touches the
	// password, so nothing sensitive ends up on screen.
	if u, parseErr := url.Parse(dsn); parseErr == nil {
		a.host = u.Host
		if u.User != nil {
			a.user = u.User.Username()
		}
		a.updateHeaderInfo()
	}
	return a, nil
}

// newAppWithBrowser builds the application against an already-constructed
// Browser, so tests can inject a fake DB without a live connection.
func newAppWithBrowser(ctx context.Context, br *browser.Browser, database string) (*App, error) {
	a := &App{
		tv:       tview.NewApplication(),
		ctx:      ctx,
		br:       br,
		database: database,
	}

	a.buildHeader()
	a.buildTree()
	a.buildIndexPanel()
	a.buildTable()
	a.buildResultsText()
	a.buildQueryBar()
	a.buildOptionsList()
	a.buildLayout()
	a.setKeybindings()
	a.tv.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		// tview does not clear the screen itself between draws (see
		// Application.SetBeforeDrawFunc's docs); since our titles change
		// length as their "^"/"v" scroll suffix comes and goes, a shorter
		// title left stale characters from the longer one behind it
		// without this, corrupting the display after enough scrolling.
		screen.Clear()
		a.refreshScrollIndicators()
		return false
	})

	if err := a.loadDatabases(); err != nil {
		return nil, err
	}

	return a, nil
}

// pgtuiLogo is a block-letter ASCII wordmark, k9s-style, shown in the
// header's top-right corner. Plain "#"/space blocks rather than k9s's
// slashes -- easier to keep pixel-aligned across five straight-stroke
// letters, and still ASCII-only per the width-safety rule above.
const pgtuiLogo = `####   ###  ##### #   # #####
#   # #       #   #   #   #
####  #  ##   #   #   #   #
#     #   #   #   #   #   #
#      ###    #    ###  #####`

// buildHeader sets up the k9s-style strip shown above everything else:
// connection context on the left, a keybinding legend in the middle, and
// a wordmark/copyright/link block on the right. All three are populated
// by updateHeaderInfo and the code below.
func (a *App) buildHeader() {
	a.headerInfo = tview.NewTextView().SetDynamicColors(true)
	a.headerLegend = tview.NewTextView().SetDynamicColors(true)
	// Wrap disabled: the ASCII art below is pre-aligned to exact columns,
	// and word-wrap would reflow it mid-line and destroy that alignment.
	a.headerLogo = tview.NewTextView().SetDynamicColors(true).SetWrap(false)

	legend := []struct{ key, desc string }{
		{"Tab", translations.T("ui.legend.switch_panel")},
		{"Shift+Tab", translations.T("ui.legend.switch_panel_back")},
		{":", translations.T("ui.legend.focus_query")},
		{"Enter", translations.T("ui.legend.run_select")},
		{"Esc", translations.T("ui.legend.cancel")},
		{"w", translations.T("ui.legend.toggle_wrap")},
		{"q", translations.T("ui.legend.quit")},
	}
	var b strings.Builder
	for _, l := range legend {
		fmt.Fprintf(&b, "[cyan]<%s>[white] %s\n", l.key, l.desc)
	}
	a.headerLegend.SetText(b.String())

	a.headerLogo.SetText(fmt.Sprintf(
		"[fuchsia]%s[white]\nCopyright 2026 Chuck Findlay\ngithub.com/cfindlayisme/pgtui",
		pgtuiLogo,
	))

	a.header = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.headerInfo, 0, 1, false).
		AddItem(a.headerLegend, 0, 1, false).
		AddItem(a.headerLogo, 0, 1, false)

	a.updateHeaderInfo()
}

// updateHeaderInfo redraws the header's context column. Called at
// startup and again whenever the tree switches which database is active.
func (a *App) updateHeaderInfo() {
	var b strings.Builder
	fmt.Fprintf(&b, "[orange]%-10s[white]%s\n", translations.T("ui.header.host")+":", a.host)
	fmt.Fprintf(&b, "[orange]%-10s[white]%s\n", translations.T("ui.header.user")+":", a.user)
	fmt.Fprintf(&b, "[orange]%-10s[white]%s\n", translations.T("ui.header.database")+":", a.database)
	a.headerInfo.SetText(b.String())
}

func (a *App) buildTree() {
	root := tview.NewTreeNode(translations.T("ui.tree_root"))
	a.tree = tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root).
		SetTopLevel(1)
	a.tree.SetBorder(true)
	a.setTreeTitle(" pgtui ")
	a.tree.SetSelectedFunc(a.onTreeSelect)
	a.tree.SetChangedFunc(a.onTreeHighlightChanged)
}

func (a *App) buildIndexPanel() {
	a.indexPanel = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	a.indexPanel.SetBorder(true)
	a.setIndexTitle(translations.T("ui.indexes_title"))
}

func (a *App) buildTable() {
	a.table = tview.NewTable().SetFixed(1, 0)
	a.table.SetSelectable(true, false)
	a.table.SetBorder(true)
}

// buildResultsText is the "w"-toggled alternative to the results table:
// tview's Table can't wrap a cell across multiple lines, so wide/long
// values there always end up truncated with an ellipsis. This renders
// the same result set as one "column: value" block per row instead,
// word-wrapped to the pane width, trading the grid layout for not
// losing any data off the edge of the screen.
func (a *App) buildResultsText() {
	a.resultsText = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	a.resultsText.SetBorder(true)
	a.setResultsTitle(translations.T("ui.results_title"))
}

// setTreeTitle/setIndexTitle/setResultsTitle record the "real" title
// text and apply it immediately (so it's visible without waiting for the
// next draw). refreshScrollIndicators reapplies these same base strings
// with a "^"/"v" suffix on every draw, so nothing here needs to know
// about scroll state.
func (a *App) setTreeTitle(title string) {
	a.treeTitle = title
	a.tree.SetTitle(title)
}

func (a *App) setIndexTitle(title string) {
	a.indexTitle = title
	a.indexPanel.SetTitle(title)
}

func (a *App) setResultsTitle(title string) {
	a.resultsTitle = title
	a.table.SetTitle(title)
	a.resultsText.SetTitle(title)
}

// scrollSuffix reports, for a pane currently showing "visible" rows/lines
// starting at "offset" out of "total", whether there's more content above
// and/or below. ASCII only ("^"/"v"), never Unicode arrows or "[...]" --
// both have already caused rendering bugs elsewhere in this tree (see the
// icon constants above), so panel chrome sticks to plain characters too.
func scrollSuffix(offset, visible, total int) string {
	canUp := offset > 0
	canDown := offset+visible < total
	switch {
	case canUp && canDown:
		return " ^v"
	case canUp:
		return " ^"
	case canDown:
		return " v"
	default:
		return ""
	}
}

// refreshScrollIndicators recomputes the ^/v suffix on every panel that
// can scroll, using each widget's own offset/size introspection. Hooked
// up via Application.SetBeforeDrawFunc so it stays correct regardless of
// what caused the redraw (arrow keys, PgUp/PgDn, new query results, a
// terminal resize) without having to intercept every one of those individually.
func (a *App) refreshScrollIndicators() {
	_, _, _, h := a.tree.GetInnerRect()
	a.tree.SetTitle(a.treeTitle + scrollSuffix(a.tree.GetScrollOffset(), h, a.tree.GetRowCount()))

	_, _, _, h = a.indexPanel.GetInnerRect()
	row, _ := a.indexPanel.GetScrollOffset()
	a.indexPanel.SetTitle(a.indexTitle + scrollSuffix(row, h, a.indexPanel.GetWrappedLineCount()))

	// The table always has exactly one fixed header row (see buildTable);
	// GetOffset/GetRowCount count it too, so it's subtracted out of the
	// visible/total figures fed to scrollSuffix.
	const tableFixedRows = 1
	_, _, _, h = a.table.GetInnerRect()
	rowOffset, _ := a.table.GetOffset()
	a.table.SetTitle(a.resultsTitle + scrollSuffix(rowOffset, h-tableFixedRows, a.table.GetRowCount()-tableFixedRows))

	_, _, _, h = a.resultsText.GetInnerRect()
	row, _ = a.resultsText.GetScrollOffset()
	a.resultsText.SetTitle(a.resultsTitle + scrollSuffix(row, h, a.resultsText.GetWrappedLineCount()))
}

func (a *App) buildQueryBar() {
	a.queryBar = tview.NewInputField()
	a.updateQueryBarLabel()
	a.queryBar.SetBorder(true).SetTitle(translations.T("ui.query_title"))
	a.queryBar.SetDoneFunc(a.onQueryBarDone)
}

// updateQueryBarLabel keeps the query bar's own label naming the
// database it's about to run against, right where you're typing --
// belt-and-suspenders alongside the header's Database: line, since
// that's easy to miss while focused on the query bar itself.
func (a *App) updateQueryBarLabel() {
	a.queryBar.SetLabel(translations.T("ui.query_label", a.database))
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

	a.resultsPages = tview.NewPages().
		AddPage("table", a.table, true, true).
		AddPage("text", a.resultsText, true, false)

	top := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 40, 1, true).
		AddItem(a.resultsPages, 0, 3, false)

	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 8, 0, false).
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
		if event.Key() == tcell.KeyBacktab {
			a.cycleFocusBack()
			return nil
		}
		if a.tv.GetFocus() != a.queryBar {
			switch event.Rune() {
			case ':':
				a.tv.SetFocus(a.queryBar)
				return nil
			case 'w':
				a.toggleWrap()
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
		a.tv.SetFocus(a.resultsFocusTarget())
	case a.resultsFocusTarget():
		a.tv.SetFocus(a.indexPanel)
	case a.indexPanel:
		a.tv.SetFocus(a.queryBar)
	default:
		a.tv.SetFocus(a.tree)
	}
}

// cycleFocusBack is Shift+Tab: the exact reverse of cycleFocus.
func (a *App) cycleFocusBack() {
	switch a.tv.GetFocus() {
	case a.queryBar:
		a.tv.SetFocus(a.indexPanel)
	case a.indexPanel:
		a.tv.SetFocus(a.resultsFocusTarget())
	case a.resultsFocusTarget():
		a.tv.SetFocus(a.tree)
	default:
		a.tv.SetFocus(a.queryBar)
	}
}

// resultsFocusTarget is whichever of the table/wrapped-text results view
// is currently visible, so focus-cycling and post-query focus always land
// on the one actually on screen.
func (a *App) resultsFocusTarget() tview.Primitive {
	if a.wrapped {
		return a.resultsText
	}
	return a.table
}

// toggleWrap is "w": swap the results pane between the grid table (fast
// to scan, truncates long values) and the wrapped text view (shows every
// value in full, one row per block). Both are kept populated with the
// same data at all times, so switching is instant.
func (a *App) toggleWrap() {
	hadResultsFocus := a.tv.GetFocus() == a.table || a.tv.GetFocus() == a.resultsText

	a.wrapped = !a.wrapped
	if a.wrapped {
		a.resultsPages.SwitchToPage("text")
	} else {
		a.resultsPages.SwitchToPage("table")
	}

	if hadResultsFocus {
		a.tv.SetFocus(a.resultsFocusTarget())
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

// onTreeHighlightChanged fires as the highlighted tree node changes --
// e.g. arrow-key navigation -- rather than only on Enter/click. Any node
// under a database (the database node itself, or an already-expanded
// schema/table beneath it) reconnects the live session once the cursor
// settles on it (see highlightSwitchDelay), so the header and query bar
// end up reflecting whichever database the cursor is currently sitting
// in, without needing to re-open that database's own node first.
func (a *App) onTreeHighlightChanged(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*nodeData)
	if !ok || ref.dbname == "" || ref.dbname == a.database {
		return
	}
	a.highlightGen++
	gen := a.highlightGen
	dbname := ref.dbname
	time.AfterFunc(highlightSwitchDelay, func() {
		a.tv.QueueUpdateDraw(func() {
			if gen != a.highlightGen {
				return // superseded by a later highlight change
			}
			if err := a.br.DB.SwitchDatabase(a.ctx, dbname); err != nil {
				a.showError(err)
				return
			}
			a.database = dbname
			a.updateHeaderInfo()
			a.updateQueryBarLabel()
		})
	})
}

func (a *App) onTreeSelect(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*nodeData)
	if !ok {
		return
	}

	if ref.dbname != "" && ref.dbname != a.database {
		a.database = ref.dbname
		a.updateHeaderInfo()
		a.updateQueryBarLabel()
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
	a.setIndexTitle(translations.T("ui.indexes_title_for", ref.schema, ref.table))

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
		a.tv.SetFocus(a.resultsFocusTarget())
		return
	}
	a.setResults(result)
	a.tv.SetFocus(a.resultsFocusTarget())
}

// onQueryBarDone handles Enter/Esc in the query bar: Enter runs whatever
// SQL is typed and jumps focus to the results; anything else (Esc) just
// returns focus to the tree without running anything.
func (a *App) onQueryBarDone(key tcell.Key) {
	if key == tcell.KeyEnter {
		a.runQuery(a.queryBar.GetText())
		a.tv.SetFocus(a.resultsFocusTarget())
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
	a.table.ScrollToBeginning()

	a.renderResultsText(result)

	a.setResultsTitle(translations.T("ui.results_title_rows", len(result.Rows)))
}

// renderResultsText fills in the wrapped-text alternative to the results
// table: one "column: value" line per cell, grouped by row, so nothing
// gets truncated regardless of how long a value is.
func (a *App) renderResultsText(result *db.QueryResult) {
	var b strings.Builder
	for r, row := range result.Rows {
		if r > 0 {
			b.WriteString("\n")
		}
		for c, val := range row {
			fmt.Fprintf(&b, "[yellow]%s:[white] %s\n", tview.Escape(result.Columns[c]), tview.Escape(val))
		}
	}
	a.resultsText.SetText(b.String())
	a.resultsText.ScrollToBeginning()
}

func (a *App) showError(err error) {
	msg := translations.T("ui.error_prefix") + err.Error()

	a.table.Clear()
	a.table.SetCell(0, 0, tview.NewTableCell(msg).SetTextColor(tcell.ColorRed))

	a.resultsText.SetText("[red]" + tview.Escape(msg))
	a.resultsText.ScrollToBeginning()

	a.setResultsTitle(translations.T("ui.results_title"))
}
