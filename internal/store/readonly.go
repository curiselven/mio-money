package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var ErrSchemaOutdated = errors.New("数据库版本较旧")

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

// CheckReadOnlySchema rejects old databases that CLI queries cannot read safely.
func (s *Store) CheckReadOnlySchema(ctx context.Context) error {
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("%w：无法读取 schema_version，请先启动桌面应用完成迁移", ErrSchemaOutdated)
	}
	current, err := latestSchemaVersion()
	if err != nil {
		return err
	}
	if version < current {
		return fmt.Errorf("%w：当前 %d，需要 %d，请先启动桌面应用完成迁移",
			ErrSchemaOutdated, version, current)
	}
	var parentColumns int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('accounts') WHERE name = 'parent_id'`).Scan(&parentColumns); err != nil {
		return fmt.Errorf("%w：无法读取 accounts 表结构，请先启动桌面应用完成迁移", ErrSchemaOutdated)
	}
	if parentColumns == 0 {
		return fmt.Errorf("%w：缺少 accounts.parent_id，请先启动桌面应用完成迁移", ErrSchemaOutdated)
	}
	return nil
}

func latestSchemaVersion() (int, error) {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, entry := range entries {
		version, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err == nil && version > latest {
			latest = version
		}
	}
	return latest, nil
}
