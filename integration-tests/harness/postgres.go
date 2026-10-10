package harness

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var nonIdentChars = regexp.MustCompile(`[^a-z0-9_]`)

// createPostgresDatabase makes a fresh, empty database for one server on the
// PostgreSQL server at dsn and returns its name and the server flags that
// point knot at it.
func createPostgresDatabase(dsn, serverName string) (string, []string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", nil, fmt.Errorf("parse KNOT_TEST_POSTGRES: %w", err)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", nil, err
	}
	name := "knot_it_" + nonIdentChars.ReplaceAllString(strings.ToLower(serverName), "_") + "_" + hex.EncodeToString(suffix)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		return "", nil, fmt.Errorf("create database %s: %w", name, err)
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
	return name, args, nil
}

// dropPostgresDatabase removes a database made by createPostgresDatabase.
func dropPostgresDatabase(dsn, name string) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	db.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`)
}
