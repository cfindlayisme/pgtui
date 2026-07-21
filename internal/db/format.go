package db

import "fmt"

// FormatValue renders a scanned Postgres value as display text for the
// results table.
func FormatValue(v any) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprintf("%v", v)
}
