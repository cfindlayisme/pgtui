# pgtui

<p align="center">
  <img src="docs/screenshot.png" alt="pgtui screenshot" width="800">
</p>

A very small terminal UI for administering a postgres database in the terminal, focused on simple tasks a software developer would perform. Inspired by Midnight Commander and k9s in look and feel

Wrote it due to RAM considerations of running pgadmin on a MacBook Neo (8GB RAM) - so designed to be very lightweight and efficient.

## Install

**macOS (Homebrew):**

```sh
brew tap cfindlayisme/tap
brew install pgtui
```

**Linux or macOS (prebuilt binary):** grab `pgtui-linux-amd64`,
`pgtui-linux-arm64`, `pgtui-darwin-amd64`, or `pgtui-darwin-arm64` from the
[latest release](https://github.com/cfindlayisme/pgtui/releases/latest),
then:

```sh
chmod +x pgtui-*
mv pgtui-* /usr/local/bin/pgtui
```

**From source:** see [Build](#build) below.

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

## Language

UI text defaults to English. Select another installed locale with
`--lang` or `PGTUI_LANG` (flag wins if both are set):

```sh
./pgtui --lang fr ...
# or
PGTUI_LANG=fr ./pgtui ...
```

An unrecognized locale silently falls back to English. To add a new one,
drop a catalog file next to `translations/en.go` and register it from
that file's `init()` via `translations.Register("fr", fr)` -- see
`translations/translations.go`.

## Using it

- Header (top): connection context on the left (host/user/currently
  active database), a keybinding legend in the middle, and the pgtui
  wordmark/copyright/repo link on the right -- k9s-inspired, always-black
  background regardless of the terminal's theme.
- Top-left panel: tree of databases → schemas → tables, tagged `DB:` /
  `SCHEMA:` / `TABLE:` (each also its own color) so the hierarchy is
  unambiguous at a glance. `Enter` on a database or schema loads and
  expands its children. Just highlighting a database node (arrow keys,
  no `Enter` needed) reconnects to it immediately, so the header and
  query bar always reflect whichever database the cursor is on.
- Bottom-left panel: indexes on whichever table you last selected (name +
  full definition, via `pg_indexes`).
- `Enter` on a table pops up a menu of common queries instead of guessing
  which one you want: preview 100 rows, preview 1000 rows, row count, or
  column list (name/type/nullable/default). Pick one with `1`-`4`/arrows+
  `Enter` and focus jumps straight to the results; `Esc`/`Cancel` backs
  out to the tree without running anything.
- Right panel: results of the last query, as a grid. Press `w` to toggle
  a wrapped view instead -- tview's grid can't wrap a cell across lines,
  so wide/long values (JSON blobs, timestamps) there get truncated with
  an ellipsis; the wrapped view shows every value in full as one
  `column: value` block per row. `w` again switches back.
- Bottom bar: always shows the query that produced the current results.
  Press `:` to focus it, type any SQL, and press `Enter` to run it --
  focus jumps to the results afterward here too.
- Any panel that has more content than fits shows a `^`/`v` in its title
  for whether you can scroll up/down from where you are.
- `Tab` cycles focus forward between the tree, results, index panel, and
  query bar; `Shift+Tab` cycles the same ring backward.
- `Esc` in the query bar returns focus to the tree.
- `q` or `Ctrl+C` quits.

## Layout

- `main.go` — entrypoint, wires config → UI together.
- `config` — CLI flag / env var resolution and password prompting.
- `db` — the live Postgres connection (via `pgx`): connecting,
  switching databases, listing catalogs, running queries.
- `browser` — feature/orchestration layer between `db` and `ui`,
  built against a small `DB` interface so it's unit-testable with a fake.
- `ui` — the `tview` terminal UI, built against `browser.Browser`.
- `translations` — message catalogs for UI text; English is built in,
  additional locales register themselves (see Language above).

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
