package main

import (
	"fmt"

	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-postgres/internal/postgres"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	postgres.Conn
	Warning    int
	Critical   int
	Percentage bool
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-postgres-connections",
			Short:    "Check the number of PostgreSQL client connections against thresholds",
			Keyspace: "sensu.io/plugins/check-postgres-connections/config",
		},
	}

	options = append(plugin.Conn.Options(),
		&sensu.PluginConfigOption[int]{
			Path:      "warning",
			Env:       "CHECK_POSTGRES_CONNECTIONS_WARNING",
			Argument:  "warning",
			Shorthand: "w",
			Default:   200,
			Usage:     "Warning threshold, in connections or in % of available connections with --percentage",
			Value:     &plugin.Warning,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "critical",
			Env:       "CHECK_POSTGRES_CONNECTIONS_CRITICAL",
			Argument:  "critical",
			Shorthand: "c",
			Default:   250,
			Usage:     "Critical threshold, in connections or in % of available connections with --percentage",
			Value:     &plugin.Critical,
		},
		&sensu.PluginConfigOption[bool]{
			Path:      "percentage",
			Env:       "CHECK_POSTGRES_CONNECTIONS_PERCENTAGE",
			Argument:  "percentage",
			Shorthand: "a",
			Default:   false,
			Usage:     "Thresholds are percentages of the available connections",
			Value:     &plugin.Percentage,
		},
	)
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if err := plugin.Conn.Validate(); err != nil {
		return sensu.CheckStateUnknown, err
	}
	if plugin.Warning < 0 || plugin.Critical < 0 {
		return sensu.CheckStateUnknown, fmt.Errorf("--warning and --critical must not be negative")
	}
	if plugin.Warning > plugin.Critical {
		return sensu.CheckStateUnknown, fmt.Errorf("--warning (%d) must not exceed --critical (%d)", plugin.Warning, plugin.Critical)
	}
	return sensu.CheckStateOK, nil
}

// statsQuery reads the connection limits and counts the client backends, the
// only processes taking a max_connections slot. reserved_connections exists
// since PostgreSQL 16.
//
// Without pg_read_all_stats (pg_monitor), backend_type is NULL for the sessions
// of other roles and for internal processes. Those with a role (usesysid) are
// counted as clients: a regular role may overcount by a few internal processes
// (logical replication launcher, walsenders, parallel workers).
const statsQuery = `SELECT
  current_setting('max_connections')::int,
  current_setting('superuser_reserved_connections')::int
    + coalesce(current_setting('reserved_connections', true), '0')::int,
  (SELECT count(*) FROM pg_stat_activity
    WHERE backend_type = 'client backend'
       OR (backend_type IS NULL AND usesysid IS NOT NULL))::int`

func executeCheck(event *corev2.Event) (int, error) {
	state, message := run()
	postgres.Print(plugin.Name, state, message)
	return state, nil
}

func run() (int, string) {
	ctx, cancel, conn, err := plugin.Conn.Connect()
	if err != nil {
		return sensu.CheckStateUnknown, "Unable to query PostgreSQL: " + postgres.Summary(err)
	}
	defer cancel()
	defer conn.Close(ctx)

	var s stats
	if err := conn.QueryRow(ctx, statsQuery).Scan(&s.Max, &s.Reserved, &s.Current); err != nil {
		return sensu.CheckStateUnknown, "Unable to query PostgreSQL: " + postgres.Summary(err)
	}
	return evaluate(s, plugin.Warning, plugin.Critical, plugin.Percentage)
}

type stats struct {
	Max      int // max_connections
	Reserved int // superuser_reserved_connections + reserved_connections
	Current  int // client backends
}

// evaluate compares the client connections with the thresholds, in absolute
// number or in percentage of the connections available to regular users.
func evaluate(s stats, warning, critical int, percentage bool) (int, string) {
	available := s.Max - s.Reserved
	if available <= 0 {
		return sensu.CheckStateUnknown, fmt.Sprintf("no connection available: max_connections %d, reserved %d", s.Max, s.Reserved)
	}

	value, detail := s.Current, fmt.Sprintf("%d out of %d connections", s.Current, available)
	if percentage {
		value = s.Current * 100 / available
		detail = fmt.Sprintf("%d%%, %s", value, detail)
	}

	switch {
	case value >= critical:
		return sensu.CheckStateCritical, "PostgreSQL connections at " + detail
	case value >= warning:
		return sensu.CheckStateWarning, "PostgreSQL connections at " + detail
	default:
		return sensu.CheckStateOK, "PostgreSQL connections under threshold: " + detail
	}
}
