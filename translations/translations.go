// Package translations holds the message catalogs for pgtui's UI text.
// English is built in and always available; additional languages are
// added by writing a new catalog file (see en.go) and calling Register
// from that file's init.
package translations

import "fmt"

var catalogs = map[string]map[string]string{
	"en": en,
}

var active = catalogs["en"]

// Register adds or replaces a locale catalog, making it selectable via
// SetLocale. A new language file registers itself from an init() func
// (see en.go) rather than editing this file by hand.
func Register(lang string, catalog map[string]string) {
	catalogs[lang] = catalog
}

// SetLocale switches the active locale. Falls back to English if the
// requested locale is not available.
func SetLocale(lang string) {
	if cat, ok := catalogs[lang]; ok {
		active = cat
	} else {
		active = catalogs["en"]
	}
}

// T translates the given message key using the active locale catalog,
// formatting it with the provided arguments. If the key is not found,
// the key itself is returned as a fallback.
func T(key string, args ...interface{}) string {
	format, ok := active[key]
	if !ok {
		return key
	}
	return fmt.Sprintf(format, args...)
}
