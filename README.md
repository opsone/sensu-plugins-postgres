# sensu-plugins-postgres

Sensu Go checks for PostgreSQL:

- `check-postgres-alive`: the server accepts connections and answers a query;
- `check-postgres-connections`: number of client connections against thresholds, in absolute number or in percentage of the available connections.

Static Go binaries (no CGO, no libpq, no Ruby runtime): one asset for every Linux distribution, `amd64` only.

## Behavior

`check-postgres-alive` runs `SELECT version()`:

| Situation | State |
|---|---|
| Query succeeds | OK (`Server version: …`) |
| Connection or query fails (unreachable, authentication, unknown database, timeout…) | CRITICAL |
| Invalid argument | UNKNOWN |

`check-postgres-connections` counts the client backends of `pg_stat_activity`, the only processes taking a `max_connections` slot (internal processes such as autovacuum or walwriter are left out). The connections available to regular roles are `max_connections - superuser_reserved_connections - reserved_connections` (the last one exists since PostgreSQL 16). With `--percentage`, the thresholds are percentages of those available connections: 100% means regular roles are refused.

| Situation | State |
|---|---|
| Connections ≥ `--critical` | CRITICAL |
| Connections ≥ `--warning` | WARNING |
| Below thresholds | OK |
| Connection or query fails, invalid argument (`--warning` above `--critical`…) | UNKNOWN |

The check counts every session only if its role can read all statistics: grant it `pg_monitor` (or `pg_read_all_stats`). A regular role still counts the other sessions, but cannot tell their type: it may overcount by a few internal processes (logical replication launcher, walsenders, parallel workers).

```sql
CREATE ROLE sensu LOGIN PASSWORD '…' IN ROLE pg_monitor;
```

## Usage

```
check-postgres-alive [flags]
  -H, --hostname string   PostgreSQL host, or Unix socket directory, e.g. /var/run/postgresql (required)
  -P, --port int          PostgreSQL port, e.g. 5432 (required)
  -d, --database string   Database to connect to, e.g. postgres (required)
  -u, --user string       PostgreSQL user (fallback: PGUSER, then the system user)
  -p, --password string   PostgreSQL password (fallback: PGPASSWORD, then ~/.pgpass)
  -T, --timeout int       Timeout of the whole check (connection and queries) in seconds (default 5)

check-postgres-connections [flags]
  (same connection flags)
  -w, --warning int       Warning threshold, in connections or in % of available connections with --percentage (default 200)
  -c, --critical int      Critical threshold, in connections or in % of available connections with --percentage (default 250)
  -a, --percentage        Thresholds are percentages of the available connections
```

Every option can also be set with an environment variable, which keeps secrets out of the process list:

| Option | Variable |
|---|---|
| `--hostname` | `CHECK_POSTGRES_HOST` |
| `--port` | `CHECK_POSTGRES_PORT` |
| `--database` | `CHECK_POSTGRES_DATABASE` |
| `--user` | `CHECK_POSTGRES_USER` |
| `--password` | `CHECK_POSTGRES_PASSWORD` |
| `--timeout` | `CHECK_POSTGRES_TIMEOUT` |
| `--warning` | `CHECK_POSTGRES_CONNECTIONS_WARNING` |
| `--critical` | `CHECK_POSTGRES_CONNECTIONS_CRITICAL` |
| `--percentage` | `CHECK_POSTGRES_CONNECTIONS_PERCENTAGE` (`true` / `false`) |

The libpq environment variables still apply to what these options leave unset: `PGUSER`, `PGPASSWORD`, `PGPASSFILE` (default `~/.pgpass`), `PGSSLMODE` (default `prefer`), `PGSSLROOTCERT`, `PGAPPNAME`… `PGHOST`, `PGPORT` and `PGDATABASE` are overridden by the required options.

Examples:

```sh
check-postgres-alive -H /var/run/postgresql -P 5432 -d postgres -u sensu
check-postgres-connections -H db.example.com -P 5432 -d postgres -u sensu -a -w 80 -c 90   # password in CHECK_POSTGRES_PASSWORD
```

## Check definitions

Every option is passed through `env_vars`, filled per host from entity annotations with token substitution (applied by the agent to the whole check definition, `env_vars` included). This keeps credentials out of the process list, and one annotation maps to one option.

Naming rule: an annotation is named after its environment variable, lowercased, without the `CHECK_` prefix (`CHECK_POSTGRES_HOST` → `postgres_host`).

- Optional options use `| default ""`: an empty variable is ignored and the option keeps its default value.
- Host, port and database have their default in the check definition (`/var/run/postgresql`, `5432`, `postgres`): set the annotation only to change it.

Annotations, e.g. in the agent configuration (`/etc/sensu/agent.yml`):

```yaml
annotations:
  postgres_host: /var/run/postgresql            # optional, default /var/run/postgresql (or db.example.com)
  postgres_port: "5432"                         # optional, default 5432
  postgres_database: postgres                   # optional, default postgres
  postgres_user: sensu
  postgres_password: secret
  postgres_connections_warning: "80"            # optional, default 200
  postgres_connections_critical: "90"           # optional, default 250
  postgres_connections_percentage: "true"       # optional, default false
```

### check-postgres-alive

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-postgres-alive
spec:
  command: check-postgres-alive
  env_vars:
    - CHECK_POSTGRES_HOST={{ .annotations.postgres_host | default "/var/run/postgresql" }}
    - CHECK_POSTGRES_PORT={{ .annotations.postgres_port | default "5432" }}
    - CHECK_POSTGRES_DATABASE={{ .annotations.postgres_database | default "postgres" }}
    - CHECK_POSTGRES_USER={{ .annotations.postgres_user | default "" }}
    - CHECK_POSTGRES_PASSWORD={{ .annotations.postgres_password | default "" }}
  runtime_assets:
    - opsone/sensu-plugins-postgres
  interval: 60
  timeout: 30
  publish: true
  subscriptions:
    - postgres
```

### check-postgres-connections

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-postgres-connections
spec:
  command: check-postgres-connections
  env_vars:
    - CHECK_POSTGRES_HOST={{ .annotations.postgres_host | default "/var/run/postgresql" }}
    - CHECK_POSTGRES_PORT={{ .annotations.postgres_port | default "5432" }}
    - CHECK_POSTGRES_DATABASE={{ .annotations.postgres_database | default "postgres" }}
    - CHECK_POSTGRES_USER={{ .annotations.postgres_user | default "" }}
    - CHECK_POSTGRES_PASSWORD={{ .annotations.postgres_password | default "" }}
    - CHECK_POSTGRES_CONNECTIONS_WARNING={{ .annotations.postgres_connections_warning | default "" }}
    - CHECK_POSTGRES_CONNECTIONS_CRITICAL={{ .annotations.postgres_connections_critical | default "" }}
    - CHECK_POSTGRES_CONNECTIONS_PERCENTAGE={{ .annotations.postgres_connections_percentage | default "" }}
  runtime_assets:
    - opsone/sensu-plugins-postgres
  interval: 60
  timeout: 30
  publish: true
  subscriptions:
    - postgres
```

Over the Unix socket, the agent runs the check as the `sensu` system user: with the default `peer` authentication, the PostgreSQL role must match it (`CREATE ROLE sensu LOGIN IN ROLE pg_monitor`), or `pg_hba.conf` must allow another method.

Entity annotations are visible in the Sensu dashboard and API: to keep the password out of them, use Sensu [secrets](https://docs.sensu.io/sensu-go/latest/operations/manage-secrets/secrets/) instead of `env_vars`, or a `~/.pgpass` file for the `sensu` user.

## Migrating from the Ruby version (≤ 0.2.1)

- Commands lose their `.rb` suffix: `check-postgres-alive.rb` → `check-postgres-alive`, `check-postgres-connections.rb` → `check-postgres-connections`.
- The host shorthand is now `-H` (`-h` is `--help`).
- `check-postgres-connections`: `--db` is now `--database`.
- `--hostname`, `--port` and `--database` are required (libpq defaults applied before: local socket, 5432, database named after the user); the defaults now live in the check definition. `PGHOST`, `PGPORT` and `PGDATABASE` are no longer read.
- `--timeout` bounds the whole check (connection and queries), not only the connection.
- `check-postgres-connections`:
  - only client backends are counted, no more internal processes (about 5 to 10 fewer on an idle server);
  - with `--percentage`, the percentage is computed on the available connections instead of `max_connections` (97 out of 97 available now reads 100%, not 97%);
  - `reserved_connections` (PostgreSQL 16+) is subtracted from the available connections;
  - `--warning` above `--critical` is rejected (UNKNOWN);
  - the role should have `pg_monitor` for an exact count.
- Error messages keep the final cause of the connection failure (pgx instead of libpq: wording differs).
- The asset no longer depends on the Ruby runtime asset, nor on the Debian release.

## Build and release

```sh
make test          # go vet + unit tests
make integration   # runs the checks against real PostgreSQL 18 and 15 servers in Docker
```

Release:

```sh
git tag 0.3.0 && git push origin 0.3.0
make release       # version taken from the tag, or forced: make release VERSION=0.3.0
sensuctl create -f dist/opsone-sensu-plugins-postgres_0.3.0.yml
```

`make release` runs `make dist`, which runs the tests then produces in `dist/`:

- one archive per architecture listed in `ARCHS` (`linux/amd64`) in Sensu asset layout (`bin/check-postgres-*`);
- the sha512 checksums file;
- `opsone-sensu-plugins-postgres_<version>.yml`, the asset definition pointing to the GitHub release of the tag.

It then creates the GitHub release of the tag with `gh`, uploads the archives and checksums, and uses the asset definition as release description.

`check-postgres-alive version` prints the version, commit and build date.
