package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"

	"github.com/opsone/sensu-plugins-postgres/internal/postgres"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Database       string
	Command        string
	SudoUser       string
	HeapAllIndexed bool
	Timeout        int
}

// maxLines bounds the pg_amcheck output copied into the check output.
const maxLines = 20

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-postgres-amcheck",
			Short:    "Check a PostgreSQL database for corruption with pg_amcheck",
			Keyspace: "sensu.io/plugins/check-postgres-amcheck/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "database",
			Env:       "CHECK_POSTGRES_AMCHECK_DATABASE",
			Argument:  "database",
			Shorthand: "d",
			Usage:     "Database to check (required)",
			Value:     &plugin.Database,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "command",
			Env:       "CHECK_POSTGRES_AMCHECK_COMMAND",
			Argument:  "command",
			Shorthand: "C",
			Default:   "pg_amcheck",
			Usage:     "pg_amcheck executable, e.g. /usr/lib/postgresql/18/bin/pg_amcheck",
			Value:     &plugin.Command,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "sudo-user",
			Env:       "CHECK_POSTGRES_AMCHECK_SUDO_USER",
			Argument:  "sudo-user",
			Shorthand: "s",
			Default:   "postgres",
			Usage:     "System user pg_amcheck runs as through sudo -n; empty to run it directly",
			Value:     &plugin.SudoUser,
		},
		&sensu.PluginConfigOption[bool]{
			Path:     "heapallindexed",
			Env:      "CHECK_POSTGRES_AMCHECK_HEAPALLINDEXED",
			Argument: "heapallindexed",
			Default:  true,
			Usage:    "Check that every heap tuple has a matching index entry (--heapallindexed=false to skip)",
			Value:    &plugin.HeapAllIndexed,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "timeout",
			Env:       "CHECK_POSTGRES_AMCHECK_TIMEOUT",
			Argument:  "timeout",
			Shorthand: "T",
			Default:   3600,
			Usage:     "Timeout of pg_amcheck in seconds",
			Value:     &plugin.Timeout,
		},
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if plugin.Database == "" {
		return sensu.CheckStateUnknown, errors.New("--database is required")
	}
	if plugin.Command == "" {
		return sensu.CheckStateUnknown, errors.New("--command must not be empty")
	}
	if plugin.Timeout <= 0 {
		return sensu.CheckStateUnknown, errors.New("--timeout must be greater than 0")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	state, message := run(plugin)
	postgres.Print(plugin.Name, state, message)
	return state, nil
}

// commandLine returns the executable and arguments running pg_amcheck. sudo -n
// fails instead of prompting for a password; pg_amcheck --no-password likewise.
func commandLine(c Config) (string, []string) {
	args := []string{"--database", c.Database, "--no-password"}
	if c.HeapAllIndexed {
		args = append(args, "--heapallindexed")
	}
	if c.SudoUser == "" {
		return c.Command, args
	}
	return "sudo", append([]string{"-n", "-u", c.SudoUser, "--", c.Command}, args...)
}

func run(c Config) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Timeout)*time.Second)
	defer cancel()

	name, args := commandLine(c)
	cmd := exec.CommandContext(ctx, name, args...)
	// SIGTERM rather than SIGKILL: sudo relays it to pg_amcheck, which would
	// otherwise keep running. WaitDelay bounds the wait if it ignores it.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return sensu.CheckStateUnknown, fmt.Sprintf("pg_amcheck on database %s timed out after %ds", c.Database, c.Timeout)
	}
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return sensu.CheckStateUnknown, fmt.Sprintf("unable to run %s: %v", name, err)
		}
		code = exitErr.ExitCode()
	}
	return evaluate(c.Database, code, stdout.String(), stderr.String())
}

// evaluate maps the pg_amcheck exit code: 0 no corruption, 2 corruption found
// (reported on stdout), anything else a failure to check (connection, missing
// amcheck extension, sudo refused…, reported on stderr).
func evaluate(database string, code int, stdout, stderr string) (int, string) {
	switch code {
	case 0:
		return sensu.CheckStateOK, "no corruption found in database " + database
	case 2:
		return sensu.CheckStateCritical, "corruption found in database " + database + "\n" + truncate(stdout)
	default:
		detail := truncate(stderr)
		if detail == "" {
			detail = truncate(stdout)
		}
		return sensu.CheckStateUnknown, fmt.Sprintf("pg_amcheck failed on database %s (exit code %d)\n%s", database, code, detail)
	}
}

// truncate keeps the first maxLines non-empty lines.
func truncate(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("… %d more lines", len(lines)-maxLines))
	}
	return strings.Join(lines, "\n")
}
