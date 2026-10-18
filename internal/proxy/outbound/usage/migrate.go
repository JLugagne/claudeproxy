package usage

import "database/sql"

// addedColumns are appended to a database written by an earlier version of the
// schema. SQLite has no ADD COLUMN IF NOT EXISTS, so column presence is read
// from the table first.
var addedColumns = []struct{ name, definition string }{
	{"cost_usd", "REAL NOT NULL DEFAULT 0"},
	{"has_cost", "INTEGER NOT NULL DEFAULT 0"},
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
