package db

import (
	"fmt"
	"strconv"
)

// FormatValue renders a scanned Postgres value as display text for the
// results table. The type switch covers the scalar kinds pgx returns for
// most columns, so formatting every cell of every result set doesn't have
// to go through fmt.Sprintf's reflection; anything less common falls back
// to %v.
func FormatValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case string:
		return t
	case []byte:
		return string(t)
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float32:
		return strconv.FormatFloat(float64(t), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}
