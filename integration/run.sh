#!/usr/bin/env bash
# Runs the checks against real PostgreSQL servers in Docker: 18 (with reserved_connections) and 15 (without),
# then check-postgres-amcheck on a PostgreSQL 18 server, run by the sensu user through sudo.
set -euo pipefail
cd "$(dirname "$0")"

id=$$
image=check-postgres-integration
amcheck_image=check-postgres-amcheck-integration
network=check-postgres-$id
socket=check-postgres-socket-$id
server=check-postgres-server-$id
amcheck=check-postgres-amcheck-$id
runner=check-postgres-runner-$id
pg_images=(postgres:18.1-trixie postgres:15.15-trixie)

docker build -q -t "$image" . >/dev/null
docker build -q -t "$amcheck_image" -f amcheck.Dockerfile . >/dev/null
docker network create "$network" >/dev/null
docker volume create "$socket" >/dev/null
cleanup() {
  docker rm -f "$runner" "$server" "$amcheck" >/dev/null 2>&1 || true
  docker volume rm "$socket" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# The runner shares the server Unix socket directory, like a local agent.
docker run -d --name "$runner" --network "$network" -v "$socket:/var/run/postgresql" "$image" >/dev/null

failures=0
# Container and system user running the checks.
target=$runner runas=nobody
expect() {
  local want=$1 desc=$2 match=$3; shift 3
  local out rc=0
  # Run as an unprivileged user, like sensu-agent does.
  out=$(docker exec -e HOME=/tmp ${envs[@]+"${envs[@]}"} "$target" runuser -u "$runas" -- "$@" 2>&1) || rc=$?
  # Last line, or the whole output when only it holds the match (multi-line check outputs).
  local last; last=$(tail -1 <<<"$out")
  [[ $last != *"$match"* && $out == *"$match"* ]] || out=$last
  out=${out//$'\n'/ | }
  if [[ $rc -eq $want && $out == *"$match"* ]]; then
    echo "ok   [$rc] $desc -> $out"
  else
    echo "FAIL [$rc, want $want, match '$match'] $desc -> $out"
    failures=$((failures + 1))
  fi
}
envs=()

psql() { docker exec "$server" psql -qtAX -U postgres -d postgres -c "$1"; }

# Opens n idle client connections as the "app" role, then waits until they are counted.
hold() {
  local n=$1 i want
  want=$(($(psql "SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend' AND usename = 'app'") + n))
  for i in $(seq "$n"); do docker exec -d "$server" psql -U app -d postgres -c 'SELECT pg_sleep(600)'; done
  for _ in $(seq 30); do
    [[ $(psql "SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend' AND usename = 'app'") -eq $want ]] && return
    sleep 1
  done
  echo "timeout waiting for $want app connections" >&2
  exit 1
}

for pg_image in "${pg_images[@]}"; do
  echo "--- $pg_image"
  docker rm -f "$server" >/dev/null 2>&1 || true
  # 20 connections, 3 reserved for superusers: 17 available.
  docker run -d --name "$server" --network "$network" -v "$socket:/var/run/postgresql" \
    -e POSTGRES_PASSWORD=admin-secret "$pg_image" -c max_connections=20 >/dev/null
  # The init server only listens on the socket: wait for the final one on TCP.
  for _ in $(seq 60); do docker exec "$server" pg_isready -q -h 127.0.0.1 && break; sleep 1; done
  # sensu has pg_monitor and sees every session; nopriv is a regular role.
  psql "CREATE ROLE sensu LOGIN PASSWORD 'sensu-secret' IN ROLE pg_monitor; CREATE ROLE nopriv LOGIN PASSWORD 'nopriv-secret'; CREATE ROLE app LOGIN"

  tcp=(-H "$server" -P 5432 -d postgres -u sensu -p sensu-secret)
  sock=(-H /var/run/postgresql -P 5432 -d postgres -u sensu)

  expect 3 "alive without host" "--hostname is required" check-postgres-alive -P 5432 -d postgres
  expect 3 "alive without port" "--port is required" check-postgres-alive -H "$server" -d postgres
  expect 3 "alive without database" "--database is required" check-postgres-alive -H "$server" -P 5432
  expect 0 "alive over TCP" "Server version: PostgreSQL" check-postgres-alive "${tcp[@]}"
  expect 0 "alive over Unix socket" "Server version: PostgreSQL" check-postgres-alive "${sock[@]}"
  expect 2 "alive wrong password" "password authentication failed" check-postgres-alive "${tcp[@]}" -p wrong
  expect 2 "alive unknown database" "does not exist" check-postgres-alive "${tcp[@]}" -d nope
  expect 2 "alive unreachable" "Error message:" check-postgres-alive "${tcp[@]}" -P 1
  expect 2 "alive timeout" "Error message:" check-postgres-alive -H 10.255.255.1 -P 5432 -d postgres -T 1
  envs=(-e CHECK_POSTGRES_HOST="$server" -e CHECK_POSTGRES_PORT=5432 -e CHECK_POSTGRES_DATABASE=postgres
        -e CHECK_POSTGRES_USER=sensu -e CHECK_POSTGRES_PASSWORD=sensu-secret)
  expect 0 "alive from environment variables" "Server version: PostgreSQL" check-postgres-alive
  envs=(-e PGUSER=sensu -e PGPASSWORD=sensu-secret)
  expect 0 "alive from libpq variables" "Server version: PostgreSQL" check-postgres-alive -H "$server" -P 5432 -d postgres
  envs=()

  expect 3 "connections warning above critical" "must not exceed" check-postgres-connections "${tcp[@]}" -w 10 -c 5
  expect 3 "connections wrong password" "Unable to query PostgreSQL" check-postgres-connections "${tcp[@]}" -p wrong
  expect 3 "connections unreachable" "Unable to query PostgreSQL" check-postgres-connections "${tcp[@]}" -P 1
  # Only the check itself is connected.
  expect 0 "connections idle" "under threshold: 1 out of 17 connections" check-postgres-connections "${tcp[@]}" -w 5 -c 10
  hold 5
  expect 1 "connections warning" "at 6 out of 17 connections" check-postgres-connections "${tcp[@]}" -w 5 -c 10
  expect 1 "connections warning over socket" "at 6 out of 17 connections" check-postgres-connections "${sock[@]}" -w 5 -c 10
  hold 5
  expect 2 "connections critical" "at 11 out of 17 connections" check-postgres-connections "${tcp[@]}" -w 5 -c 10
  # Without pg_monitor, other sessions are still counted (plus a few internal processes).
  expect 1 "connections without pg_monitor" "PostgreSQL connections at 1" check-postgres-connections -H "$server" -P 5432 -d postgres -u nopriv -p nopriv-secret -w 11 -c 30
  expect 0 "connections default thresholds" "under threshold: 11 out of 17" check-postgres-connections "${tcp[@]}"
  # 11 / 17 = 64%.
  expect 0 "connections percentage ok" "under threshold: 64%, 11 out of 17" check-postgres-connections "${tcp[@]}" -a -w 70 -c 90
  expect 1 "connections percentage warning" "at 64%, 11 out of 17" check-postgres-connections "${tcp[@]}" -a -w 60 -c 90
  expect 2 "connections percentage critical" "at 64%, 11 out of 17" check-postgres-connections "${tcp[@]}" -a -w 50 -c 60
  envs=(-e CHECK_POSTGRES_CONNECTIONS_PERCENTAGE=true -e CHECK_POSTGRES_CONNECTIONS_WARNING=60 -e CHECK_POSTGRES_CONNECTIONS_CRITICAL=90)
  expect 1 "connections percentage from environment variables" "at 64%, 11 out of 17" check-postgres-connections "${tcp[@]}"
  envs=()
done

echo "--- check-postgres-amcheck"
# Without data checksums (on by default since PostgreSQL 18), so that a tuple changed on disk stays a valid page.
docker run -d --name "$amcheck" -e POSTGRES_PASSWORD=admin-secret -e POSTGRES_INITDB_ARGS=--no-data-checksums "$amcheck_image" >/dev/null
for _ in $(seq 60); do docker exec "$amcheck" pg_isready -q -h 127.0.0.1 && break; sleep 1; done
target=$amcheck runas=sensu
apsql() { docker exec "$amcheck" psql -qtAX -U postgres -d "${2:-app}" -c "$1"; }
apsql "CREATE DATABASE app" postgres
apsql "CREATE TABLE t (id int PRIMARY KEY, v text); INSERT INTO t SELECT i, md5(i::text) FROM generate_series(1, 10000) i"
cmd=/usr/lib/postgresql/18/bin/pg_amcheck

expect 3 "amcheck without database" "--database is required" check-postgres-amcheck -C "$cmd"
expect 3 "amcheck extension missing" "amcheck is not installed" check-postgres-amcheck -C "$cmd" -d app
apsql "CREATE EXTENSION amcheck"
expect 0 "amcheck ok" "no corruption found in database app" check-postgres-amcheck -C "$cmd" -d app
expect 0 "amcheck without heapallindexed" "no corruption found" check-postgres-amcheck -C "$cmd" -d app --heapallindexed=false
envs=(-e CHECK_POSTGRES_AMCHECK_DATABASE=app -e CHECK_POSTGRES_AMCHECK_COMMAND="$cmd")
expect 0 "amcheck from environment variables" "no corruption found" check-postgres-amcheck
envs=()
expect 3 "amcheck unknown database" "no connectable databases" check-postgres-amcheck -C "$cmd" -d nope
expect 3 "amcheck sudo refused" "a password is required" check-postgres-amcheck -C "$cmd" -d app -s root
expect 3 "amcheck without sudo (peer authentication)" "pg_amcheck failed" check-postgres-amcheck -C "$cmd" -d app -s ""

# Changes the id of the first heap tuple on disk (to 99999, little-endian int4 after the tuple header), leaving
# the index untouched: only --heapallindexed sees the heap tuple without index entry.
restart() {
  docker restart "$amcheck" >/dev/null
  for _ in $(seq 60); do docker exec "$amcheck" pg_isready -q -h 127.0.0.1 && break; sleep 1; done
}
apsql "CREATE EXTENSION pageinspect"
offset=$(apsql "SELECT lp_off + t_hoff FROM heap_page_items(get_raw_page('t', 0)) WHERE lp = 1")
file=$(apsql "CHECKPOINT; SELECT current_setting('data_directory') || '/' || pg_relation_filepath('t')")
docker exec "$amcheck" sh -c "printf '\\237\\206\\001\\000' | dd of='$file' bs=1 seek=$offset conv=notrunc status=none"
restart
expect 0 "amcheck heap change without heapallindexed" "no corruption found" check-postgres-amcheck -C "$cmd" -d app --heapallindexed=false
expect 2 "amcheck heap change with heapallindexed" "lacks matching index tuple" check-postgres-amcheck -C "$cmd" -d app
apsql "REINDEX TABLE t"
expect 0 "amcheck after reindex" "no corruption found" check-postgres-amcheck -C "$cmd" -d app

# Zeroes the first leaf page of the primary key index, flushed to disk beforehand; the restart drops the cached copy.
file=$(apsql "CHECKPOINT; SELECT current_setting('data_directory') || '/' || pg_relation_filepath('t_pkey')")
docker exec "$amcheck" dd if=/dev/zero of="$file" bs=8192 seek=1 count=1 conv=notrunc status=none
restart
expect 2 "amcheck corruption" "corruption found in database app" check-postgres-amcheck -C "$cmd" -d app

exit $failures
