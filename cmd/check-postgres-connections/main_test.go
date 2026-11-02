package main

import (
	"testing"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		stats      stats
		warning    int
		critical   int
		percentage bool
		state      int
		message    string
	}{
		{
			name: "absolute ok", stats: stats{Max: 100, Reserved: 3, Current: 10}, warning: 50, critical: 80,
			state: sensu.CheckStateOK, message: "PostgreSQL connections under threshold: 10 out of 97 connections",
		},
		{
			name: "absolute warning at threshold", stats: stats{Max: 100, Reserved: 3, Current: 50}, warning: 50, critical: 80,
			state: sensu.CheckStateWarning, message: "PostgreSQL connections at 50 out of 97 connections",
		},
		{
			name: "absolute critical", stats: stats{Max: 100, Reserved: 3, Current: 90}, warning: 50, critical: 80,
			state: sensu.CheckStateCritical, message: "PostgreSQL connections at 90 out of 97 connections",
		},
		{
			name: "percentage ok", stats: stats{Max: 100, Reserved: 3, Current: 10}, warning: 80, critical: 90, percentage: true,
			state: sensu.CheckStateOK, message: "PostgreSQL connections under threshold: 10%, 10 out of 97 connections",
		},
		{
			name: "percentage of available, not of max", stats: stats{Max: 100, Reserved: 3, Current: 97}, warning: 80, critical: 100, percentage: true,
			state: sensu.CheckStateCritical, message: "PostgreSQL connections at 100%, 97 out of 97 connections",
		},
		{
			name: "percentage truncated", stats: stats{Max: 100, Reserved: 3, Current: 78}, warning: 81, critical: 90, percentage: true,
			state: sensu.CheckStateOK, message: "PostgreSQL connections under threshold: 80%, 78 out of 97 connections",
		},
		{
			name: "percentage warning", stats: stats{Max: 20, Reserved: 0, Current: 17}, warning: 80, critical: 90, percentage: true,
			state: sensu.CheckStateWarning, message: "PostgreSQL connections at 85%, 17 out of 20 connections",
		},
		{
			name: "no connection available", stats: stats{Max: 3, Reserved: 3, Current: 1}, warning: 80, critical: 90,
			state: sensu.CheckStateUnknown, message: "no connection available: max_connections 3, reserved 3",
		},
	}

	for _, tt := range tests {
		state, message := evaluate(tt.stats, tt.warning, tt.critical, tt.percentage)
		if state != tt.state || message != tt.message {
			t.Errorf("%s: got %d %q, want %d %q", tt.name, state, message, tt.state, tt.message)
		}
	}
}
