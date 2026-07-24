#!/bin/bash
#
# Spins up a throwaway Postgres in Docker, pre-loaded with sample data
# across two databases/schemas, so pgtui has something real to browse
# while developing without needing a live database on hand.
#
# Usage:
#   scripts/dev-postgres.sh [up|down|reset]
#
#   up (default)  Start the container if it isn't running, seeding it
#                 with sample data the first time it's created.
#   down          Stop and remove the container (data is not kept).
#   reset         down, then up again with a fresh seed.
#
# Author: Chuck Findlay <chuck@findlayis.me>
# License: LGPL v3.0

set -e

CONTAINER_NAME="pgtui-dev-postgres"
IMAGE="postgres:16-alpine"
PORT="${PGTUI_DEV_PORT:-5432}"
SUPERUSER="postgres"
SUPERPASS="postgres"
MAINTENANCE_DB="postgres"
PRIMARY_DB="pgtui_dev"
SECOND_DB="pgtui_dev2"

action="${1:-up}"

container_exists() {
  docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"
}

container_running() {
  docker ps --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"
}

wait_for_postgres() {
  echo "Waiting for Postgres to accept connections..."
  for _ in $(seq 1 30); do
    if docker exec "$CONTAINER_NAME" pg_isready -U "$SUPERUSER" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "Postgres never became ready" >&2
  exit 1
}

seed() {
  echo "Seeding sample data..."

  docker exec -i "$CONTAINER_NAME" psql -U "$SUPERUSER" -d "$PRIMARY_DB" -v ON_ERROR_STOP=1 <<'SQL'
create schema if not exists app;

create table public.users (
    id serial primary key,
    name text not null,
    email text not null unique,
    created_at timestamptz not null default now()
);

create table public.orders (
    id serial primary key,
    user_id integer not null references public.users(id),
    amount numeric(10,2) not null,
    status text not null,
    created_at timestamptz not null default now()
);
create index orders_user_id_idx on public.orders(user_id);
create index orders_status_idx on public.orders(status);

create table app.events (
    id serial primary key,
    user_id integer not null references public.users(id),
    event_type text not null,
    payload jsonb not null default '{}',
    created_at timestamptz not null default now()
);

insert into public.users (name, email)
select 'user_' || i, 'user' || i || '@example.com'
from generate_series(1, 50) as i;

insert into public.orders (user_id, amount, status)
select (floor(random() * 50) + 1)::int,
       round((random() * 500)::numeric, 2),
       (array['pending', 'paid', 'shipped', 'cancelled'])[(floor(random() * 4) + 1)::int]
from generate_series(1, 200);

insert into app.events (user_id, event_type, payload)
select (floor(random() * 50) + 1)::int,
       (array['login', 'logout', 'click', 'purchase'])[(floor(random() * 4) + 1)::int],
       jsonb_build_object('source', 'seed-script')
from generate_series(1, 100);
SQL

  docker exec "$CONTAINER_NAME" psql -U "$SUPERUSER" -d "$MAINTENANCE_DB" -v ON_ERROR_STOP=1 \
    -c "create database $SECOND_DB;"

  docker exec -i "$CONTAINER_NAME" psql -U "$SUPERUSER" -d "$SECOND_DB" -v ON_ERROR_STOP=1 <<'SQL'
create table public.products (
    id serial primary key,
    sku text not null unique,
    name text not null,
    price numeric(10,2) not null
);

insert into public.products (sku, name, price)
select 'SKU-' || i, 'Product ' || i, round((random() * 100)::numeric, 2)
from generate_series(1, 30) as i;
SQL

  echo "Seed complete."
}

print_connection_info() {
  cat <<INFO

Postgres is up on localhost:${PORT}.

  PGPASSWORD=${SUPERPASS} go run . --host localhost --port ${PORT} \\
    --user ${SUPERUSER} --dbname ${PRIMARY_DB}

A second database, "${SECOND_DB}", is also seeded -- switch to it from
the tree to see pgtui reconnect. Same connection info also works for
the opt-in visual smoke test:

  PGTUI_SMOKE_DSN="postgres://${SUPERUSER}:${SUPERPASS}@localhost:${PORT}/${PRIMARY_DB}?sslmode=disable" \\
  PGTUI_SMOKE_DB="${PRIMARY_DB}" \\
  go test ./ui/... -run TestVisualSmoke -v
INFO
}

up() {
  if container_running; then
    echo "$CONTAINER_NAME is already running."
    print_connection_info
    return
  fi

  if container_exists; then
    echo "Starting existing $CONTAINER_NAME container..."
    docker start "$CONTAINER_NAME" >/dev/null
    wait_for_postgres
    print_connection_info
    return
  fi

  echo "Starting a fresh $CONTAINER_NAME container..."
  docker run -d \
    --name "$CONTAINER_NAME" \
    -e POSTGRES_USER="$SUPERUSER" \
    -e POSTGRES_PASSWORD="$SUPERPASS" \
    -e POSTGRES_DB="$PRIMARY_DB" \
    -p "${PORT}:5432" \
    "$IMAGE" >/dev/null

  wait_for_postgres
  seed
  print_connection_info
}

down() {
  if container_exists; then
    echo "Removing $CONTAINER_NAME..."
    docker rm -f "$CONTAINER_NAME" >/dev/null
  else
    echo "$CONTAINER_NAME does not exist."
  fi
}

case "$action" in
  up) up ;;
  down) down ;;
  reset)
    down
    up
    ;;
  *)
    echo "Usage: $0 [up|down|reset]" >&2
    exit 1
    ;;
esac
