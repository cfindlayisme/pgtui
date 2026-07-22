// Package translations holds the message catalogs for pgtui's UI text.
// English is built in and always available; additional languages are
// added by writing a new catalog file (see en.go) and registering it in
// catalogs below.
package translations

import "fmt"

var catalogs = map[string]map[string]string{
	"en": en,
}

var active = catalogs["en"]

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
