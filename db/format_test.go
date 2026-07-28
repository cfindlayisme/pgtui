package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil is NULL", nil, "NULL"},
		{"string passes through", "hello", "hello"},
		{"int", 42, "42"},
		{"float", 3.5, "3.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int32", int32(7), "7"},
		{"int64", int64(9000000000), "9000000000"},
		{"float32", float32(1.25), "1.25"},
		{"bytes", []byte("raw"), "raw"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, FormatValue(tc.in))
		})
	}
}
