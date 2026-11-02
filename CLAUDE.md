# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Sensu Go checks for PostgreSQL, written in Go with `github.com/sensu/sensu-plugin-sdk` and `github.com/jackc/pgx/v5` (no CGO, no libpq). Published as a Sensu asset (static binaries, `linux/amd64` only, see `ARCHS` in the Makefile). Same layout and tooling as `opsone/sensu-plugins-backup`.

## Common Commands

```bash
make test          # go vet + unit tests
make build         # local binaries at the root
make integration   # Docker: real PostgreSQL 18 and 15 servers, checks run as nobody
make dist          # tests, then tar.gz per arch + sha512 + asset YAML in dist/
make release       # dist + GitHub release of the current tag (gh)
```

Version comes from `git describe --tags` (override with `VERSION=`), injected via ldflags into the SDK `version` package.

## Architecture

```
cmd/check-postgres-alive/        # SELECT version(); connection/query failure is CRITICAL
cmd/check-postgres-connections/  # client backends vs thresholds (pure evaluate()); failure is UNKNOWN
internal/postgres/               # shared connection options (Conn), keyword/value DSN, error Summary, output
integration/                     # Dockerfile + run.sh
scripts/asset-yml.sh             # builds the Sensu asset definition from the checksums file
```

The DSN only sets user/password when given, so libpq environment variables (`PGUSER`, `PGPASSWORD`, `PGSSLMODE`…) and `~/.pgpass` still apply. pgx returns one error line per connection attempt (TLS then plain text with `sslmode=prefer`); `Summary` keeps the last one.

Connection counting: `backend_type = 'client backend'`, plus rows with a NULL `backend_type` and a role, as seen by roles without `pg_read_all_stats`. Available = `max_connections - superuser_reserved_connections - reserved_connections` (PG 16+).

Options are declared with `sensu.PluginConfigOption`, each with a `CHECK_POSTGRES_*` env var. `-h` is reserved by cobra for `--help` (redefining it panics). Host, port and database are required in the binary; their defaults live in the check definitions (README).
