package ui

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func dumpScreen(t *testing.T, sim tcell.SimulationScreen, label string) {
	t.Helper()
	w, h := sim.Size()
	cells, _, _ := sim.GetContents()
	var out []byte
	for y := 0; y < h; y++ {
		line := make([]rune, w)
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				line[x] = c.Runes[0]
			} else {
				line[x] = ' '
			}
		}
		out = append(out, []byte(string(line))...)
		out = append(out, '\n')
	}
	fmt.Println("=====", label, "=====")
	fmt.Println(string(out))
}

// TestVisualSmoke drives the app against a real Postgres and renders it to
// a simulated terminal screen, printing each stage. It's the only way to
// catch pure-rendering bugs (glyph width corrupting tview's cursor math,
// tree text containing "[...]" being parsed as a color tag) that the
// logic-level tests above can't see. Skipped unless PGTUI_SMOKE_DSN is set.
func TestVisualSmoke(t *testing.T) {
	if os.Getenv("PGTUI_SMOKE_DSN") == "" {
		t.Skip("set PGTUI_SMOKE_DSN to run the visual smoke test against a real Postgres")
	}

	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}

	a, err := NewApp(context.Background(), os.Getenv("PGTUI_SMOKE_DSN"), os.Getenv("PGTUI_SMOKE_DB"))
	if err != nil {
		t.Fatal(err)
	}

	// SetScreen re-initializes the screen internally, which resets a
	// SimulationScreen back to tcell's 80x25 default -- so the intended
	// size has to be set after attaching it, not before, or it's silently
	// discarded and every rect/wrap computation below is measured wrong.
	a.tv.SetScreen(sim)
	sim.SetSize(110, 32)
	a.tv.SetRoot(a.pages, true).SetFocus(a.tree)
	a.tv.ForceDraw()
	dumpScreen(t, sim, "initial tree")

	var dbNode *tview.TreeNode
	for _, n := range a.tree.GetRoot().GetChildren() {
		if n.GetReference().(*nodeData).dbname == os.Getenv("PGTUI_SMOKE_DB") {
			dbNode = n
			break
		}
	}
	if dbNode == nil {
		t.Fatalf("database %q not found in tree", os.Getenv("PGTUI_SMOKE_DB"))
	}
	a.onTreeSelect(dbNode)
	a.tv.ForceDraw()
	dumpScreen(t, sim, "database expanded")

	for _, sn := range dbNode.GetChildren() {
		a.onTreeSelect(sn)
	}
	a.tv.ForceDraw()
	dumpScreen(t, sim, "schemas expanded")

	var tableNode *tview.TreeNode
	for _, sn := range dbNode.GetChildren() {
		if children := sn.GetChildren(); len(children) > 0 {
			tableNode = children[0]
			break
		}
	}
	if tableNode == nil {
		t.Fatal("no table node found")
	}
	a.onTreeSelect(tableNode)
	a.tv.ForceDraw()
	dumpScreen(t, sim, "table options modal")

	a.optionsList.GetItemSelectedFunc(0)()
	a.tv.ForceDraw()
	dumpScreen(t, sim, "after preview 100 rows")
}
