package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithDatabase(t *testing.T) {
	cases := []struct {
		name   string
		dsn    string
		dbname string
		want   string
	}{
		{
			name:   "replaces path, keeps host and query",
			dsn:    "postgres://user:pass@localhost:5432/olddb?sslmode=disable",
			dbname: "newdb",
			want:   "postgres://user:pass@localhost:5432/newdb?sslmode=disable",
		},
		{
			name:   "no existing path",
			dsn:    "postgres://localhost:5432",
			dbname: "newdb",
			want:   "postgres://localhost:5432/newdb",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := WithDatabase(tc.dsn, tc.dbname)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWithDatabase_InvalidDSN(t *testing.T) {
	_, err := WithDatabase("postgres://%zz", "newdb")
	assert.Error(t, err)
}
