package driver_postgres

import (
	"database/sql"
	"strconv"
	"strings"
)

// pgConn wraps the connection pool so the queries can be written with ?
// placeholders, as the MySQL driver's are, and still run on PostgreSQL,
// which numbers its placeholders ($1, $2, ...).
type pgConn struct {
	*sql.DB
}

func (c *pgConn) Exec(query string, args ...interface{}) (sql.Result, error) {
	return c.DB.Exec(rebind(query), args...)
}

func (c *pgConn) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return c.DB.Query(rebind(query), args...)
}

func (c *pgConn) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.DB.QueryRow(rebind(query), args...)
}

func (c *pgConn) Begin() (*pgTx, error) {
	tx, err := c.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &pgTx{Tx: tx}, nil
}

// pgTx is the transaction counterpart of pgConn.
type pgTx struct {
	*sql.Tx
}

func (t *pgTx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return t.Tx.Exec(rebind(query), args...)
}

func (t *pgTx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return t.Tx.Query(rebind(query), args...)
}

func (t *pgTx) QueryRow(query string, args ...interface{}) *sql.Row {
	return t.Tx.QueryRow(rebind(query), args...)
}

// rebind rewrites each ? placeholder as $n. A ? inside a quoted string or
// identifier is left alone.
func rebind(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 16)

	n := 0
	var quote byte
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(ch)
	}

	return b.String()
}
