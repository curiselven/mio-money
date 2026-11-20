package store

import (
	"database/sql"
	"fmt"
	"os"
)

// OpenReadOnly opens an existing database without migrations or writes.
// It returns nil, nil when path does not exist.
func OpenReadOnly(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("只读打开数据库失败：%w", err)
	}
	// Without a live WAL, immutable avoids creating -wal/-shm files. A running
	// writer keeps a WAL, so fall back to a normal read-only connection.
	dsn := "file:" + path + "?mode=ro"
	_, walErr := os.Stat(path + "-wal")
	_, journalErr := os.Stat(path + "-journal")
	if os.IsNotExist(walErr) && os.IsNotExist(journalErr) {
		dsn += "&immutable=1"
	} else {
		dsn += "&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", dsn+"&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("只读打开数据库失败（请确认文件完整且未被其他进程锁定）：%w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("只读读取数据库失败（请确认数据库未被占用）：%w", err)
	}
	return &Store{db: db}, nil
}
