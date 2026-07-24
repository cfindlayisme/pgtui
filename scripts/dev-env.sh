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
# Written to also work when sourced from zsh (macOS's default shell),
# not just bash -- so it deliberately avoids bash-only $BASH_SOURCE
# introspection, which silently resolves to nothing under zsh.
#
# Author: Chuck Findlay <chuck@findlayis.me>
# License: LGPL v3.0

_dev_env_sourced=0
if [ -n "${ZSH_EVAL_CONTEXT:-}" ]; then
  case $ZSH_EVAL_CONTEXT in *:file*) _dev_env_sourced=1 ;; esac
elif [ -n "${BASH_VERSION:-}" ]; then
  (return 0 2>/dev/null) && _dev_env_sourced=1
fi

if [ "$_dev_env_sourced" -ne 1 ]; then
  echo "dev-env.sh must be sourced, not executed: source scripts/dev-env.sh" >&2
  exit 1
fi
unset _dev_env_sourced

_dev_env_root="$(git rev-parse --show-toplevel 2>/dev/null)"
if [ -z "$_dev_env_root" ]; then
  echo "dev-env.sh must be run from inside the pgtui git repo" >&2
  unset _dev_env_root
  return 1
fi

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
