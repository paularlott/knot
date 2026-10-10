package harness

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var nonIdentChars = regexp.MustCompile(`[^a-z0-9_]`)

// testDatabase is a server's own database on an external database server.
type testDatabase struct {
	driver string // "pgx" or "mysql"
	dsn    string // admin connection, for creating and dropping it
	name   string
}

// newDatabaseName returns a unique database name for a server.
func newDatabaseName(serverName string) (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return "knot_it_" + nonIdentChars.ReplaceAllString(strings.ToLower(serverName), "_") + "_" + hex.EncodeToString(suffix), nil
}

// createPostgresDatabase makes a fresh, empty database for one server on the
// PostgreSQL server at dsn, a postgres:// URL, and returns it with the server
// flags that point knot at it.
func createPostgresDatabase(dsn, serverName string) (*testDatabase, []string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse KNOT_TEST_POSTGRES: %w", err)
	}
	name, err := newDatabaseName(serverName)
	if err != nil {
		return nil, nil, err
	}

	tdb := &testDatabase{driver: "pgx", dsn: dsn, name: name}
	if err := tdb.exec(`CREATE DATABASE "` + name + `"`); err != nil {
		return nil, nil, fmt.Errorf("create database %s: %w", name, err)
	}

	password, _ := u.User.Password()
	args := []string{
		"--postgres-enabled",
		"--postgres-host", u.Hostname(),
		"--postgres-port", u.Port(),
		"--postgres-user", u.User.Username(),
		"--postgres-password", password,
		"--postgres-database", name,
	}
	if mode := u.Query().Get("sslmode"); mode != "" {
		args = append(args, "--postgres-sslmode", mode)
	}
	return tdb, args, nil
}

// createMySQLDatabase is createPostgresDatabase for a MySQL or MariaDB server
// at dsn, a mysql://user:password@host:port URL.
func createMySQLDatabase(dsn, serverName string) (*testDatabase, []string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse KNOT_TEST_MYSQL: %w", err)
	}
	name, err := newDatabaseName(serverName)
	if err != nil {
		return nil, nil, err
	}

	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	cfg := gomysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = u.Hostname() + ":" + port

	tdb := &testDatabase{driver: "mysql", dsn: cfg.FormatDSN(), name: name}
	if err := tdb.exec("CREATE DATABASE `" + name + "`"); err != nil {
		return nil, nil, fmt.Errorf("create database %s: %w", name, err)
	}

	args := []string{
		"--mysql-enabled",
		"--mysql-host", u.Hostname(),
		"--mysql-port", port,
		"--mysql-user", u.User.Username(),
		"--mysql-password", password,
		"--mysql-database", name,
	}
	return tdb, args, nil
}

func (d *testDatabase) exec(stmt string) error {
	db, err := sql.Open(d.driver, d.dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(stmt)
	return err
}

// drop removes the database.
func (d *testDatabase) drop() {
	if d.driver == "mysql" {
		d.exec("DROP DATABASE IF EXISTS `" + d.name + "`")
	} else {
		d.exec(`DROP DATABASE IF EXISTS "` + d.name + `" WITH (FORCE)`)
	}
}
