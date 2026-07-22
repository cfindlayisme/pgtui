package ui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cfindlayisme/pgtui/db"
	"github.com/cfindlayisme/pgtui/translations"
)

// fakeCatalog gives every key the UI package reads an unmistakable,
// locale-specific value, so a test can prove the UI is actually pulling
// text from the active catalog at render time rather than a hardcoded
// literal that merely happens to match the English catalog.
var fakeCatalog = map[string]string{
	"ui.tree_root":                "XX-ROOT",
	"ui.indexes_title":            " XX-IDX ",
	"ui.indexes_title_for":        " XX-IDXFOR:%s.%s ",
	"ui.no_indexes":               "XX-NONE",
	"ui.results_title":            " XX-RES ",
	"ui.results_title_rows":       " XX-ROWS(%d) ",
	"ui.query_title":              " XX-QUERY ",
	"ui.error_prefix":             "XX-ERR:",
	"ui.option.preview_100":       "XX-PREVIEW100",
	"ui.option.preview_100_desc":  "XX-DESC100",
	"ui.option.preview_1000":      "XX-PREVIEW1000",
	"ui.option.preview_1000_desc": "XX-DESC1000",
	"ui.option.row_count":         "XX-ROWCOUNT",
	"ui.option.row_count_desc":    "XX-ROWCOUNTDESC",
	"ui.option.columns":           "XX-COLUMNS",
	"ui.option.columns_desc":      "XX-COLUMNSDESC",
	"ui.option.cancel":            "XX-CANCEL",
}

func useFakeLocale(t *testing.T) {
	t.Helper()
	translations.Register("xx-wiring-test", fakeCatalog)
	translations.SetLocale("xx-wiring-test")
	t.Cleanup(func() { translations.SetLocale("en") })
}

// TestUIRendersFromActiveCatalog is the wiring test flagged as missing:
// every other UI test asserts against English strings that would still
// pass if a translations.T(...) call were reverted to a hardcoded
// literal, since the literal and the English catalog value are
// identical. Switching to a fake locale first means only genuine calls
// into the catalog can produce these values.
func TestUIRendersFromActiveCatalog(t *testing.T) {
	useFakeLocale(t)

	fake := &fakeDB{
		databases: []string{"alpha"},
		schemas:   map[string][]string{"alpha": {"public"}},
		tables:    map[string]map[string][]string{"alpha": {"public": {"users"}}},
		results: map[string]*db.QueryResult{
			`SELECT * FROM "public"."users" LIMIT 100`: {
				Columns: []string{"id"},
				Rows:    [][]string{{"1"}, {"2"}},
			},
		},
	}
	a := newTestApp(t, fake)

	assert.Equal(t, "XX-ROOT", a.tree.GetRoot().GetText())
	assert.Equal(t, " XX-IDX ", a.indexPanel.GetTitle())
	assert.Equal(t, " XX-RES ", a.table.GetTitle())
	assert.Equal(t, " XX-QUERY ", a.queryBar.GetTitle())

	tableNode := expandToTable(a)
	a.onTreeSelect(tableNode)

	assert.Equal(t, fmt.Sprintf(" XX-IDXFOR:%s.%s ", "public", "users"), a.indexPanel.GetTitle())
	assert.Contains(t, a.indexPanel.GetText(true), "XX-NONE")

	main, _ := a.optionsList.GetItemText(0)
	assert.Equal(t, "XX-PREVIEW100", main)
	main, _ = a.optionsList.GetItemText(4)
	assert.Equal(t, "XX-CANCEL", main)

	a.optionsList.GetItemSelectedFunc(0)() // "Preview 100 rows" in the fake locale

	assert.Equal(t, fmt.Sprintf(" XX-ROWS(%d) ", 2), a.table.GetTitle())
}

func TestUIErrorPrefixRendersFromActiveCatalog(t *testing.T) {
	useFakeLocale(t)

	fake := &fakeDB{}
	a := newTestApp(t, fake)

	a.showError(fmt.Errorf("boom"))

	assert.Equal(t, "XX-ERR:boom", a.table.GetCell(0, 0).Text)
}
