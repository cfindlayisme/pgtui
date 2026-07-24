# pgtui

A very small terminal UI for browsing Postgres, in the spirit of Midnight
Commander / pgAdmin: a database → schema → table tree on the left, query
results on the right, and an always-visible query bar at the bottom.

## Build

```sh
go build .
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

- Top-left panel: tree of databases → schemas → tables, tagged `DB:` /
  `SCHEMA:` / `TABLE:` (each also its own color) so the hierarchy is
  unambiguous at a glance. `Enter` on a database or schema loads and
  expands its children.
- Bottom-left panel: indexes on whichever table you last selected (name +
  full definition, via `pg_indexes`).
- `Enter` on a table pops up a menu of common queries instead of guessing
  which one you want: preview 100 rows, preview 1000 rows, row count, or
  column list (name/type/nullable/default). Pick one with `1`-`4`/arrows+
  `Enter` and focus jumps straight to the results; `Esc`/`Cancel` backs
  out to the tree without running anything.
- Right panel: results of the last query.
- Bottom bar: always shows the query that produced the current results.
  Press `:` to focus it, type any SQL, and press `Enter` to run it --
  focus jumps to the results afterward here too.
- `Tab` cycles focus between the tree, results table, index panel, and
  query bar.
- `Esc` in the query bar returns focus to the tree.
- `q` or `Ctrl+C` quits.

Selecting a database that differs from the currently connected one
transparently reconnects, since a single Postgres connection can't query
across databases.

## Layout

- `main.go` — entrypoint, wires config → UI together.
- `config` — CLI flag / env var resolution and password prompting.
- `db` — the live Postgres connection (via `pgx`): connecting,
  switching databases, listing catalogs, running queries.
- `browser` — feature/orchestration layer between `db` and `ui`,
  built against a small `DB` interface so it's unit-testable with a fake.
- `ui` — the `tview` terminal UI, built against `browser.Browser`.

## Development database

`scripts/dev-postgres.sh` spins up a throwaway Postgres in Docker (two
databases, a couple of schemas, and enough sample rows/indexes to make the
tree and results pane worth looking at) so there's something real to point
pgtui at while developing:

```sh
scripts/dev-postgres.sh [up|down|reset]   # up is the default
```

To skip typing out connection flags, `source` the companion script instead
-- it starts the container if needed, exports `PG*` env vars for the rest
of the shell session, and launches pgtui:

```sh
source scripts/dev-env.sh
```

## Testing

```sh
go test ./...
```

Unit tests cover the pure utilities (`config.Load`, `db.WithDatabase`,
`db.FormatValue`, `browser.QuoteIdent`/`TableQuery`/`PreviewQuery`/
`CountQuery`/`ColumnsQuery`). `db`'s actual Postgres-facing
methods (`ListDatabases`/`ListSchemas`/`ListTables`/`ListIndexes`/
`RunQuery`/`Connect`/`SwitchDatabase`) are tested against a mocked `pgx`
connection — SQL/args assertions and row data via
[`pgxmock`](https://github.com/pashagolub/pgxmock) for query building, and
a small hand-rolled fake for the connect/reconnect/close lifecycle — so
none of it needs a live database. Functional tests cover
`browser.Browser`'s orchestration against a fake `DB`; and the `ui`
package tests exercise the tree/table/options-modal/query-bar wiring
end-to-end against the same kind of fake, without needing a live database
or terminal.

`ui` also has an opt-in visual smoke test
(`TestVisualSmoke`, skipped by default) that drives the real app against
an actual Postgres and renders it to a simulated terminal screen — the
only way to catch pure-rendering bugs (glyph-width issues throwing off
tview's cursor math, tree text being misparsed as color tags) that
logic-level tests can't see. Point it at a scratch database and run it
explicitly:

```sh
PGTUI_SMOKE_DSN="postgres://user:pass@localhost:5432/somedb?sslmode=disable" \
PGTUI_SMOKE_DB="somedb" \
go test ./ui/... -run TestVisualSmoke -v
```
