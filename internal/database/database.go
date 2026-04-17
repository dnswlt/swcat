package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"modernc.org/sqlite"
)

// JSON is a generic column type that transparently marshals/unmarshals any
// value to/from a SQLite TEXT column. Implement driver.Valuer for writes and
// sql.Scanner for reads so callers never touch encoding/json at the call site.
type JSON[T any] struct{ V T }

func (j JSON[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	return string(b), err
}

func (j *JSON[T]) Scan(src any) error {
	var b []byte
	switch s := src.(type) {
	case string:
		b = []byte(s)
	case []byte:
		b = s
	default:
		return fmt.Errorf("JSON.Scan: unsupported source type %T", src)
	}
	return json.Unmarshal(b, &j.V)
}

// sqliteConnector opens a new SQLite connection and applies pragmas before
// returning it to database/sql's pool. This ensures every connection —
// not just the first one — gets the desired settings.
var sqliteDrv = &sqlite.Driver{}

type sqliteConnector struct {
	dsn     string
	pragmas []string
}

func (c *sqliteConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := sqliteDrv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	ec, ok := conn.(driver.ExecerContext)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("sqlite driver connection does not implement ExecerContext")
	}
	for _, p := range c.pragmas {
		if _, err := ec.ExecContext(ctx, p, nil); err != nil {
			conn.Close()
			return nil, fmt.Errorf("exec %q: %w", p, err)
		}
	}
	return conn, nil
}

func (c *sqliteConnector) Driver() driver.Driver { return sqliteDrv }

// New creates a new sqlite DB connected to dsn (a file path, or ":memory:" for an in-mem DB).
func New(dsn string) *sql.DB {
	// Use a custom connector so every pooled connection gets the pragmas,
	// not just whichever one happens to run the first Exec.
	return sql.OpenDB(&sqliteConnector{
		dsn: dsn,
		pragmas: []string{
			`PRAGMA journal_mode=WAL`,
			`PRAGMA synchronous=NORMAL`,
			`PRAGMA busy_timeout=5000`,
		},
	})
}

// LoadObservations reads all status observations for entityRef from db.
func LoadObservations(ctx context.Context, db *sql.DB, entityRef string) (map[string]catalog.Observation, error) {
	const q = `SELECT key, value, producer, updated_at, version
		FROM status_observations
		WHERE entity_id = ?`
	rows, err := db.QueryContext(ctx, q, entityRef)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	observations := make(map[string]catalog.Observation)
	for rows.Next() {
		var key, value, producer, updatedAtStr, version string
		if err := rows.Scan(&key, &value, &producer, &updatedAtStr, &version); err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at %q: %w", updatedAtStr, err)
		}
		observations[key] = catalog.Observation{
			Value:     json.RawMessage(value),
			Producer:  producer,
			UpdatedAt: updatedAt,
			Version:   version,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observations: %w", err)
	}

	return observations, nil
}

// StoreObservations replaces all status observations for e in db with the
// observations currently in e's Status. Existing rows for e are removed,
// even when e has no observations. The whole operation runs in a single
// transaction.
func StoreObservations(ctx context.Context, db *sql.DB, e catalog.Entity) error {
	entityRef := e.GetRef().String()
	var obs map[string]catalog.Observation
	if status := e.GetStatus(); status != nil {
		obs = status.Observations
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM status_observations WHERE entity_id = ?`, entityRef); err != nil {
		return fmt.Errorf("delete observations: %w", err)
	}

	if len(obs) > 0 {
		const insert = `INSERT INTO status_observations
			(entity_id, key, value, producer, updated_at, version) VALUES (?, ?, ?, ?, ?, ?)`
		stmt, err := tx.PrepareContext(ctx, insert)
		if err != nil {
			return fmt.Errorf("prepare insert: %w", err)
		}
		defer stmt.Close()

		for key, o := range obs {
			if _, err := stmt.ExecContext(ctx,
				entityRef, key, string(o.Value), o.Producer,
				o.UpdatedAt.UTC().Format(time.RFC3339Nano),
				o.Version,
			); err != nil {
				return fmt.Errorf("insert observation %q: %w", key, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func RecreateTables(ctx context.Context, db *sql.DB, dropAll bool) error {

	if dropAll {
		_, err := db.ExecContext(ctx, `DROP TABLE status_observations`)
		if err != nil {
			return fmt.Errorf("drop table: %w", err)
		}
	}

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS status_observations (
		entity_id  TEXT NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT,
		producer   TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		version    TEXT,
		PRIMARY KEY (entity_id, key)
	)`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	return nil
}
