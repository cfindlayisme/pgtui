package translations

import "testing"

func TestTFormatsEnglish(t *testing.T) {
	SetLocale("en")

	got := T("ui.no_indexes")
	want := "no indexes"
	if got != want {
		t.Errorf("T(ui.no_indexes) = %q, want %q", got, want)
	}
}

func TestTMultipleArgs(t *testing.T) {
	SetLocale("en")

	got := T("ui.indexes_title_for", "public", "users")
	want := " Indexes: public.users "
	if got != want {
		t.Errorf("T(ui.indexes_title_for) = %q, want %q", got, want)
	}
}

func TestTUnknownKeyReturnsFallback(t *testing.T) {
	got := T("nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("T(unknown key) = %q, want %q", got, "nonexistent.key")
	}
}

func TestAllEnglishKeysNonEmpty(t *testing.T) {
	for key, val := range en {
		if val == "" {
			t.Errorf("English catalog key %q has empty value", key)
		}
	}
}

func TestSetLocaleUnsupportedFallsBackToEnglish(t *testing.T) {
	SetLocale("zz")

	got := T("ui.tree_root")
	want := "Databases"
	if got != want {
		t.Errorf("after SetLocale(zz), T() = %q, want %q", got, want)
	}
}

func TestRegisterMakesANewLocaleSelectable(t *testing.T) {
	t.Cleanup(func() { SetLocale("en") })

	Register("xx", map[string]string{"ui.tree_root": "Fake"})
	SetLocale("xx")

	got := T("ui.tree_root")
	if got != "Fake" {
		t.Errorf("T(ui.tree_root) after Register+SetLocale(xx) = %q, want %q", got, "Fake")
	}
}
