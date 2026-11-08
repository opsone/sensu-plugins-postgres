package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

func TestCommandLine(t *testing.T) {
	c := Config{Database: "app", Command: "/usr/lib/postgresql/18/bin/pg_amcheck", SudoUser: "postgres", HeapAllIndexed: true}
	name, args := commandLine(c)
	want := []string{"-n", "-u", "postgres", "--", "/usr/lib/postgresql/18/bin/pg_amcheck", "--database", "app", "--no-password", "--heapallindexed"}
	if name != "sudo" || !reflect.DeepEqual(args, want) {
		t.Errorf("with sudo: got %s %v, want sudo %v", name, args, want)
	}

	c.SudoUser, c.HeapAllIndexed = "", false
	name, args = commandLine(c)
	want = []string{"--database", "app", "--no-password"}
	if name != c.Command || !reflect.DeepEqual(args, want) {
		t.Errorf("without sudo: got %s %v, want %s %v", name, args, c.Command, want)
	}
}

func TestEvaluate(t *testing.T) {
	long := strings.Repeat("line\n", maxLines+5)
	tests := []struct {
		name           string
		code           int
		stdout, stderr string
		state          int
		message        string
	}{
		{name: "ok", state: sensu.CheckStateOK, message: "no corruption found in database app"},
		{
			name: "corruption", code: 2, stdout: "heap table \"app.public.t\", block 0, offset 1:\n    invalid line pointer\n",
			state:   sensu.CheckStateCritical,
			message: "corruption found in database app\nheap table \"app.public.t\", block 0, offset 1:\n    invalid line pointer",
		},
		{
			name: "corruption truncated", code: 2, stdout: long, state: sensu.CheckStateCritical,
			message: "corruption found in database app\n" + strings.Repeat("line\n", maxLines) + "… 5 more lines",
		},
		{
			name: "failure", code: 1, stderr: "pg_amcheck: warning: skipping database \"app\": amcheck is not installed\npg_amcheck: error: no relations to check\n",
			state:   sensu.CheckStateUnknown,
			message: "pg_amcheck failed on database app (exit code 1)\npg_amcheck: warning: skipping database \"app\": amcheck is not installed\npg_amcheck: error: no relations to check",
		},
		{
			name: "failure without stderr", code: 1, stdout: "something\n", state: sensu.CheckStateUnknown,
			message: "pg_amcheck failed on database app (exit code 1)\nsomething",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, message := evaluate("app", tt.code, tt.stdout, tt.stderr)
			if state != tt.state || message != tt.message {
				t.Errorf("got (%d, %q), want (%d, %q)", state, message, tt.state, tt.message)
			}
		})
	}
}

// fake writes an executable script standing for pg_amcheck.
func fake(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pg_amcheck")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		timeout int
		state   int
		prefix  string
	}{
		{name: "arguments passed", body: `[ "$*" = "--database app --no-password --heapallindexed" ] || exit 1`, timeout: 5,
			state: sensu.CheckStateOK, prefix: "no corruption found in database app"},
		{name: "corruption", body: `echo 'btree index "app.public.t_pkey":'; exit 2`, timeout: 5,
			state: sensu.CheckStateCritical, prefix: "corruption found in database app\nbtree index"},
		{name: "failure", body: `echo 'connection failed' >&2; exit 1`, timeout: 5,
			state: sensu.CheckStateUnknown, prefix: "pg_amcheck failed on database app (exit code 1)\nconnection failed"},
		{name: "timeout", body: `exec sleep 30`, timeout: 1,
			state: sensu.CheckStateUnknown, prefix: "pg_amcheck on database app timed out after 1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Database: "app", Command: fake(t, tt.body), HeapAllIndexed: true, Timeout: tt.timeout}
			state, message := run(c)
			if state != tt.state || !strings.HasPrefix(message, tt.prefix) {
				t.Errorf("got (%d, %q), want (%d, %q…)", state, message, tt.state, tt.prefix)
			}
		})
	}

	c := Config{Database: "app", Command: filepath.Join(t.TempDir(), "missing"), Timeout: 5}
	if state, message := run(c); state != sensu.CheckStateUnknown || !strings.HasPrefix(message, "unable to run") {
		t.Errorf("missing command: got (%d, %q)", state, message)
	}
}
