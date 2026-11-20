package store

import (
	"database/sql"
	"os"
)

// OpenReadOnly opens an existing database without migrations or writes.
// It returns nil, nil when path does not exist.
func OpenReadOnly(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// Without a live WAL, immutable avoids creating -wal/-shm files. A running
	// writer keeps a WAL, so fall back to a normal read-only connection.
	dsn := "file:" + path + "?mode=ro"
	if _, err := os.Stat(path + "-wal"); os.IsNotExist(err) {
		dsn += "&immutable=1"
	} else {
		dsn += "&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", dsn+"&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
