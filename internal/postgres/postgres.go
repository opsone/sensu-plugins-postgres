// Package postgres holds the connection options shared by the PostgreSQL checks.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Conn holds the connection settings of a check.
type Conn struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Timeout  int
}

// Options returns the connection options, bound to c.
func (c *Conn) Options() []sensu.ConfigOption {
	return []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "hostname",
			Env:       "CHECK_POSTGRES_HOST",
			Argument:  "hostname",
			Shorthand: "H",
			Usage:     "PostgreSQL host, or Unix socket directory, e.g. /var/run/postgresql (required)",
			Value:     &c.Host,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "port",
			Env:       "CHECK_POSTGRES_PORT",
			Argument:  "port",
			Shorthand: "P",
			Usage:     "PostgreSQL port, e.g. 5432 (required)",
			Value:     &c.Port,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "database",
			Env:       "CHECK_POSTGRES_DATABASE",
			Argument:  "database",
			Shorthand: "d",
			Usage:     "Database to connect to, e.g. postgres (required)",
			Value:     &c.Database,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "user",
			Env:       "CHECK_POSTGRES_USER",
			Argument:  "user",
			Shorthand: "u",
			Usage:     "PostgreSQL user (fallback: PGUSER, then the system user)",
			Value:     &c.User,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "password",
			Env:       "CHECK_POSTGRES_PASSWORD",
			Argument:  "password",
			Shorthand: "p",
			Secret:    true,
			Usage:     "PostgreSQL password (fallback: PGPASSWORD, then ~/.pgpass)",
			Value:     &c.Password,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "timeout",
			Env:       "CHECK_POSTGRES_TIMEOUT",
			Argument:  "timeout",
			Shorthand: "T",
			Default:   5,
			Usage:     "Timeout of the whole check (connection and queries) in seconds",
			Value:     &c.Timeout,
		},
	}
}

// Validate checks the required options.
func (c *Conn) Validate() error {
	if c.Host == "" {
		return errors.New("--hostname is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("--port is required and must be between 1 and 65535")
	}
	if c.Database == "" {
		return errors.New("--database is required")
	}
	if c.Timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	return nil
}

// DSN builds the keyword/value connection string. Unset user and password are
// left out, so that libpq environment variables (PGUSER, PGPASSWORD, PGSSLMODE…)
// and ~/.pgpass still apply.
func (c *Conn) DSN() string {
	params := []string{
		"host=" + quote(c.Host),
		"port=" + strconv.Itoa(c.Port),
		"dbname=" + quote(c.Database),
		"connect_timeout=" + strconv.Itoa(c.Timeout),
	}
	if c.User != "" {
		params = append(params, "user="+quote(c.User))
	}
	if c.Password != "" {
		params = append(params, "password="+quote(c.Password))
	}
	return strings.Join(params, " ")
}

func quote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}

// Connect opens a connection; the returned context bounds the whole check.
func (c *Conn) Connect() (context.Context, context.CancelFunc, *pgx.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Timeout)*time.Second)
	cfg, err := pgx.ParseConfig(c.DSN())
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("invalid connection settings: %w", err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return ctx, cancel, conn, nil
}

// Summary returns the last non-empty line of an error message. pgx reports one
// line per connection attempt (TLS then plain text with sslmode=prefer) under a
// "failed to connect to" header: the last one holds the final cause.
func Summary(err error) string {
	lines := strings.Split(err.Error(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// Print writes the check output line.
func Print(name string, state int, message string) {
	fmt.Printf("%s %s: %s\n", name, StateLabel(state), message)
}

// StateLabel returns the human-readable label of a check state.
func StateLabel(state int) string {
	switch state {
	case sensu.CheckStateOK:
		return "OK"
	case sensu.CheckStateWarning:
		return "WARNING"
	case sensu.CheckStateCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}
