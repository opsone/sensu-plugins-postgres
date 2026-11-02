package main

import (
	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-postgres/internal/postgres"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	postgres.Conn
}

var plugin = Config{
	PluginConfig: sensu.PluginConfig{
		Name:     "check-postgres-alive",
		Short:    "Check that PostgreSQL accepts connections and answers queries",
		Keyspace: "sensu.io/plugins/check-postgres-alive/config",
	},
}

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, plugin.Conn.Options(), checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if err := plugin.Conn.Validate(); err != nil {
		return sensu.CheckStateUnknown, err
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	state, message := run()
	postgres.Print(plugin.Name, state, message)
	return state, nil
}

func run() (int, string) {
	ctx, cancel, conn, err := plugin.Conn.Connect()
	if err != nil {
		return sensu.CheckStateCritical, "Error message: " + postgres.Summary(err)
	}
	defer cancel()
	defer conn.Close(ctx)

	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return sensu.CheckStateCritical, "Error message: " + postgres.Summary(err)
	}
	return sensu.CheckStateOK, "Server version: " + version
}
