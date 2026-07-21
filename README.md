# pgtui

A very small terminal UI for browsing Postgres, in the spirit of Midnight
Commander / pgAdmin: a database → schema → table tree on the left, query
results on the right, and an always-visible query bar at the bottom.

## Build

```sh
go build -o pgtui ./cmd/pgtui
```

## Connecting

Connection settings come from CLI flags or the standard `PG*` environment
variables (flags win if both are set). `host`, `port`, `user`, and `dbname`
are required — pgtui exits with an error naming whichever are missing.
`password` is the one exception: if it's not supplied either way, pgtui
prompts for it interactively (without echoing) before launching.

| Setting | Flag         | Env var      |
|---------|--------------|--------------|
| Host    | `--host`     | `PGHOST`     |
| Port    | `--port`     | `PGPORT`     |
| User    | `--user`     | `PGUSER`     |
| Password| `--password` | `PGPASSWORD` |
| Database| `--dbname`   | `PGDATABASE` |
| SSL mode| `--sslmode`  | `PGSSLMODE`  (default `prefer`) |

```sh
./pgtui --host localhost --port 5432 --user postgres --dbname postgres
# or
PGHOST=localhost PGPORT=5432 PGUSER=postgres PGDATABASE=postgres ./pgtui
```

## Using it

- Left panel: tree of databases → schemas → tables. `Enter` on a database
  or schema loads and expands its children; `Enter` on a table runs a
  default `SELECT * ... LIMIT 100` and shows the results on the right.
- Right panel: results of the last query.
- Bottom bar: always shows the query that produced the current results.
  Press `:` to focus it, type any SQL, and press `Enter` to run it.
- `Tab` cycles focus between the tree, results table, and query bar.
- `Esc` in the query bar returns focus to the tree.
- `q` or `Ctrl+C` quits.

Selecting a database that differs from the currently connected one
transparently reconnects, since a single Postgres connection can't query
across databases.

## Layout

- `cmd/pgtui` — entrypoint, wires config → UI together.
- `internal/config` — CLI flag / env var resolution and password prompting.
- `internal/db` — the live Postgres connection (via `pgx`): connecting,
  switching databases, listing catalogs, running queries.
- `internal/browser` — feature/orchestration layer between `db` and `ui`,
  built against a small `DB` interface so it's unit-testable with a fake.
- `internal/ui` — the `tview` terminal UI, built against `browser.Browser`.

## Testing

```sh
go test ./...
```

Unit tests cover the pure utilities (`config.Load`, `db.WithDatabase`,
`db.FormatValue`, `browser.QuoteIdent`/`TableQuery`). `internal/db`'s
actual Postgres-facing methods (`ListDatabases`/`ListSchemas`/
`ListTables`/`RunQuery`/`Connect`/`SwitchDatabase`) are tested against a
mocked `pgx` connection — SQL/args assertions and row data via
[`pgxmock`](https://github.com/pashagolub/pgxmock) for query building, and
a small hand-rolled fake for the connect/reconnect/close lifecycle — so
none of it needs a live database. Functional tests cover
`browser.Browser`'s orchestration against a fake `DB`; and the `ui`
package tests exercise the tree/table/query-bar wiring end-to-end against
the same kind of fake, without needing a live database or terminal.
