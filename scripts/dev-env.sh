#!/bin/bash
#
# Source this to point your shell at dev-postgres.sh's container and
# launch pgtui against it in one step:
#
#   source scripts/dev-env.sh
#
# Starts the dev container if it isn't already running, exports
# PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE for the rest of the shell
# session (pgtui reads these directly, and they also work as-is for
# PGTUI_SMOKE_DSN-style testing), then runs pgtui. The env vars stay
# exported after pgtui exits, so re-running `go run .` or reaching for
# `psql` afterward needs no extra setup.
#
# Author: Chuck Findlay <chuck@findlayis.me>
# License: LGPL v3.0

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "dev-env.sh must be sourced, not executed: source scripts/dev-env.sh" >&2
  exit 1
fi

_dev_env_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"$_dev_env_root/scripts/dev-postgres.sh" up

export PGHOST="localhost"
export PGPORT="${PGTUI_DEV_PORT:-5432}"
export PGUSER="postgres"
export PGPASSWORD="postgres"
export PGDATABASE="pgtui_dev"

echo "PG* env vars exported for the pgtui-dev-postgres container."
echo "Launching pgtui..."
(cd "$_dev_env_root" && go run .)

unset _dev_env_root
