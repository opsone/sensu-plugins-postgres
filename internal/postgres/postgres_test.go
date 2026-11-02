package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestValidate(t *testing.T) {
	valid := Conn{Host: "/var/run/postgresql", Port: 5432, Database: "postgres", Timeout: 5}
	tests := []struct {
		name    string
		modify  func(*Conn)
		wantErr bool
	}{
		{name: "valid", modify: func(*Conn) {}},
		{name: "no host", modify: func(c *Conn) { c.Host = "" }, wantErr: true},
		{name: "no port", modify: func(c *Conn) { c.Port = 0 }, wantErr: true},
		{name: "port too high", modify: func(c *Conn) { c.Port = 70000 }, wantErr: true},
		{name: "no database", modify: func(c *Conn) { c.Database = "" }, wantErr: true},
		{name: "zero timeout", modify: func(c *Conn) { c.Timeout = 0 }, wantErr: true},
	}
	for _, tt := range tests {
		c := valid
		tt.modify(&c)
		if err := c.Validate(); (err != nil) != tt.wantErr {
			t.Errorf("%s: Validate() error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestDSN(t *testing.T) {
	t.Setenv("PGUSER", "envuser")
	t.Setenv("PGPASSWORD", "envpass")
	t.Setenv("PGHOST", "envhost")

	tests := []struct {
		name     string
		conn     Conn
		user     string
		password string
	}{
		{
			name:     "explicit credentials with quotes",
			conn:     Conn{Host: "db.example.com", Port: 5433, Database: "app", User: "sensu", Password: `it's a \secret`, Timeout: 5},
			user:     "sensu",
			password: `it's a \secret`,
		},
		{
			name:     "credentials from environment",
			conn:     Conn{Host: "/var/run/postgresql", Port: 5432, Database: "postgres", Timeout: 5},
			user:     "envuser",
			password: "envpass",
		},
	}
	for _, tt := range tests {
		cfg, err := pgx.ParseConfig(tt.conn.DSN())
		if err != nil {
			t.Fatalf("%s: ParseConfig(%q): %v", tt.name, tt.conn.DSN(), err)
		}
		if cfg.Host != tt.conn.Host || int(cfg.Port) != tt.conn.Port || cfg.Database != tt.conn.Database {
			t.Errorf("%s: got %s:%d/%s, want %s:%d/%s", tt.name, cfg.Host, cfg.Port, cfg.Database, tt.conn.Host, tt.conn.Port, tt.conn.Database)
		}
		if cfg.User != tt.user || cfg.Password != tt.password {
			t.Errorf("%s: got user %q password %q, want %q %q", tt.name, cfg.User, cfg.Password, tt.user, tt.password)
		}
		// sslmode=prefer adds a plain-text fallback; PGHOST must not add other hosts.
		for _, fb := range cfg.Fallbacks {
			if fb.Host != tt.conn.Host {
				t.Errorf("%s: unexpected fallback host %q", tt.name, fb.Host)
			}
		}
	}
}

func TestSummary(t *testing.T) {
	tests := []struct{ err, want string }{
		{
			err:  "failed to connect to `user=sensu database=postgres`:\n\t10.0.0.1:5432 (db): tls error: server refused TLS connection\n\t10.0.0.1:5432 (db): failed SASL auth: FATAL: password authentication failed for user \"sensu\" (SQLSTATE 28P01)\n",
			want: "10.0.0.1:5432 (db): failed SASL auth: FATAL: password authentication failed for user \"sensu\" (SQLSTATE 28P01)",
		},
		{err: "ERROR: permission denied (SQLSTATE 42501)", want: "ERROR: permission denied (SQLSTATE 42501)"},
	}
	for _, tt := range tests {
		if got := Summary(errors.New(tt.err)); got != tt.want {
			t.Errorf("Summary(%q) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
