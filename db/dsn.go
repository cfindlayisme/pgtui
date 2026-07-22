package db

import "net/url"

// WithDatabase returns dsn with its path replaced so it points at dbname
// instead. Used to reconnect to a sibling database on the same server,
// since a single Postgres connection can never query across databases.
func WithDatabase(dsn, dbname string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbname
	return u.String(), nil
}
