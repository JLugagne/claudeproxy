package usage

import (
	"database/sql"
	"strconv"
)

// addedColumns are appended to a database written by an earlier version of the
// schema. SQLite has no ADD COLUMN IF NOT EXISTS, so column presence is read
// from the table first.
var addedColumns = []struct{ name, definition string }{
	{"cost_usd", "REAL NOT NULL DEFAULT 0"},
	{"has_cost", "INTEGER NOT NULL DEFAULT 0"},
	{"ttft_ms", "INTEGER"},
}

// compactRateLimits migrates a ratelimit table written one row per request: it
// adds the bucket column, then keeps the newest row of each session and
// interval, which the unique index requires.
func compactRateLimits(db *sql.DB) error {
	columns, err := tableColumns(db, "ratelimit")
	if err != nil {
		return err
	}
	if !columns["bucket"] {
		if _, err := db.Exec("ALTER TABLE ratelimit ADD COLUMN bucket INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	_, err = db.Exec(`
UPDATE ratelimit SET bucket = ts / ` + strconv.FormatInt(RateLimitInterval.Milliseconds(), 10) + ` WHERE bucket = 0;
DELETE FROM ratelimit WHERE rowid NOT IN (SELECT MAX(rowid) FROM ratelimit GROUP BY bucket, project, session);
CREATE UNIQUE INDEX IF NOT EXISTS ratelimit_bucket ON ratelimit(bucket, project, session);`)
	return err
}

// ensureColumns brings an existing database up to the current schema.
func ensureColumns(db *sql.DB) error {
	existing, err := tableColumns(db, "usage")
	if err != nil {
		return err
	}
	for _, column := range addedColumns {
		if existing[column.name] {
			continue
		}
		if _, err := db.Exec("ALTER TABLE usage ADD COLUMN " + column.name + " " + column.definition); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			columnTyp string
			notNull   int
			dflt      any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &columnTyp, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
