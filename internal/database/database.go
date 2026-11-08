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

// NewSqlite creates a new sqlite DB connected to dsn (a file path, or ":memory:" for an in-mem DB).
func NewSqlite(dsn string) *sql.DB {
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
	const q = `SELECT key, value, producer, updated_at, version, meta
		FROM status_observations
		WHERE entity_id = ?`
	rows, err := db.QueryContext(ctx, q, entityRef)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	observations := make(map[string]catalog.Observation)
	for rows.Next() {
		var key, value, producer, updatedAtStr string
		var version sql.NullString
		var meta JSON[map[string]string]
		if err := rows.Scan(&key, &value, &producer, &updatedAtStr, &version, &meta); err != nil {
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
			Version:   version.String,
			Meta:      meta.V,
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
			(entity_id, key, value, producer, updated_at, version, meta) VALUES (?, ?, ?, ?, ?, ?, ?)`
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
				JSON[map[string]string]{V: o.Meta},
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

// KeySelector selects observation keys: exactly Key, or all keys starting with
// Prefix. Exactly one of the two must be set.
type KeySelector struct {
	Key    string
	Prefix string
}

// EntityKeys lists observation keys of one entity.
type EntityKeys struct {
	EntityRef string
	Keys      []string
}

// DeleteObservationsByKey deletes the observations whose key sel selects,
// across all entities in db, including entities that no catalog revision
// currently contains. It returns the deleted keys per entity, sorted by entity
// and key. With dryRun, it deletes nothing and only reports.
func DeleteObservationsByKey(ctx context.Context, db *sql.DB, sel KeySelector, dryRun bool) ([]EntityKeys, error) {
	var where string
	var args []any
	switch {
	case sel.Key != "" && sel.Prefix == "":
		where, args = `key = ?`, []any{sel.Key}
	case sel.Prefix != "" && sel.Key == "":
		// Not LIKE, which would treat '%' and '_' in the prefix as wildcards.
		where, args = `substr(key, 1, length(?)) = ?`, []any{sel.Prefix, sel.Prefix}
	default:
		return nil, fmt.Errorf("exactly one of key and prefix must be set")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT entity_id, key FROM status_observations WHERE `+where+` ORDER BY entity_id, key`, args...)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	var result []EntityKeys
	for rows.Next() {
		var entityRef, key string
		if err := rows.Scan(&entityRef, &key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		if n := len(result); n == 0 || result[n-1].EntityRef != entityRef {
			result = append(result, EntityKeys{EntityRef: entityRef})
		}
		result[len(result)-1].Keys = append(result[len(result)-1].Keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate observations: %w", err)
	}
	rows.Close()

	if dryRun || len(result) == 0 {
		return result, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM status_observations WHERE `+where, args...); err != nil {
		return nil, fmt.Errorf("delete observations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func RecreateTables(ctx context.Context, db *sql.DB, dropAll bool) error {

	if dropAll {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS status_observations`)
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
		meta       TEXT,
		PRIMARY KEY (entity_id, key)
	)`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	return nil
}
