# Changelog

## [Unreleased]

### Added
- `check-postgres-amcheck`: checks a database for corruption with `sudo -n -u postgres pg_amcheck --heapallindexed`; CRITICAL on corruption, UNKNOWN when the check cannot run.

## [0.3.0] - 2026-11-02

### Changed
- Rewritten in Go with the Sensu plugin SDK and pgx: static binaries for `linux/amd64`, no Ruby runtime asset, no libpq, one asset for every Debian release.
- Commands renamed without the `.rb` suffix (`check-postgres-alive`, `check-postgres-connections`).
- The `--hostname` shorthand is `-H` (`-h` is `--help`).
- `--hostname`, `--port` and `--database` are required; set their defaults in the check definition (`/var/run/postgresql`, `5432`, `postgres`).
- `--timeout` bounds the whole check, connection and queries.
- `check-postgres-connections`: `--db` renamed `--database`.
- `check-postgres-connections`: only client backends are counted, internal processes (autovacuum, walwriter…) are left out.
- `check-postgres-connections`: with `--percentage`, the percentage is computed on the available connections (`max_connections - superuser_reserved_connections - reserved_connections`) instead of `max_connections`.
- `check-postgres-connections`: `--warning` above `--critical` is rejected (UNKNOWN).

### Added
- Every option can be set through an environment variable (`CHECK_POSTGRES_*`).
- `check-postgres-connections`: `reserved_connections` (PostgreSQL 16+) is subtracted from the available connections.

## [0.2.1]

### Changed
- Ruby version, assets per Debian release (10 to 13).
