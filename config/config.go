// Package config resolves how pgtui connects to Postgres, from CLI flags
// and standard PG* environment variables.
package config

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Config holds the discrete connection settings needed to reach Postgres.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
	Lang     string
}

// DSN renders cfg as a postgres:// connection string.
func (c Config) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   c.Host + ":" + c.Port,
		Path:   "/" + c.Database,
	}
	if c.SSLMode != "" {
		q := url.Values{}
		q.Set("sslmode", c.SSLMode)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// MissingFieldsError reports required connection settings that were not
// supplied by either a CLI flag or an environment variable.
type MissingFieldsError struct {
	Fields []string
}

func (e *MissingFieldsError) Error() string {
	return fmt.Sprintf("missing required connection settings: %s (set via flag or environment variable)",
		strings.Join(e.Fields, ", "))
}

// Load resolves connection settings from CLI flags (highest priority) and
// PG*-style environment variables. Host, port, user, and dbname are
// required; a missing one produces a MissingFieldsError. Password is left
// empty if unset so the caller can prompt for it interactively rather than
// failing outright.
func Load(args []string, getenv func(string) string) (Config, error) {
	fs := flag.NewFlagSet("pgtui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	host := fs.String("host", "", "Postgres host (or PGHOST)")
	port := fs.String("port", "", "Postgres port (or PGPORT)")
	user := fs.String("user", "", "Postgres user (or PGUSER)")
	password := fs.String("password", "", "Postgres password (or PGPASSWORD)")
	dbname := fs.String("dbname", "", "Postgres database name (or PGDATABASE)")
	sslmode := fs.String("sslmode", "", "Postgres sslmode (or PGSSLMODE)")
	lang := fs.String("lang", "", "UI language (or PGTUI_LANG); defaults to en")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Host:     firstNonEmpty(*host, getenv("PGHOST")),
		Port:     firstNonEmpty(*port, getenv("PGPORT")),
		User:     firstNonEmpty(*user, getenv("PGUSER")),
		Password: firstNonEmpty(*password, getenv("PGPASSWORD")),
		Database: firstNonEmpty(*dbname, getenv("PGDATABASE")),
		SSLMode:  firstNonEmpty(*sslmode, getenv("PGSSLMODE")),
		Lang:     firstNonEmpty(*lang, getenv("PGTUI_LANG"), "en"),
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "prefer"
	}

	var missing []string
	if cfg.Host == "" {
		missing = append(missing, "host (--host or PGHOST)")
	}
	if cfg.Port == "" {
		missing = append(missing, "port (--port or PGPORT)")
	}
	if cfg.User == "" {
		missing = append(missing, "user (--user or PGUSER)")
	}
	if cfg.Database == "" {
		missing = append(missing, "dbname (--dbname or PGDATABASE)")
	}
	if len(missing) > 0 {
		return Config{}, &MissingFieldsError{Fields: missing}
	}

	return cfg, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ResolvePassword returns cfg with Password filled in, invoking promptFn
// only when no password was supplied by flag or environment variable.
func ResolvePassword(cfg Config, promptFn func() (string, error)) (Config, error) {
	if cfg.Password != "" {
		return cfg, nil
	}
	pw, err := promptFn()
	if err != nil {
		return Config{}, err
	}
	cfg.Password = pw
	return cfg, nil
}
