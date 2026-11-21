package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReadOnlySchemaRejectsOldDatabase(t *testing.T) {
	for _, tc := range []struct {
		name         string
		version      int
		parentColumn bool
	}{
		{"old-version", 6, false},
		{"missing-parent-column", 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := OpenReadOnly(writeSchemaDB(t, tc.version, tc.parentColumn))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			err = st.CheckReadOnlySchema(context.Background())
			if !errors.Is(err, ErrSchemaOutdated) || !strings.Contains(err.Error(), "桌面") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func writeSchemaDB(t *testing.T, version int, parentColumn bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	parent := ""
	if parentColumn {
		parent = ", parent_id TEXT"
	}
	for _, statement := range []string{
		`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE accounts (id TEXT PRIMARY KEY, name TEXT NOT NULL` + parent + `)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES (?, '2026-01-01T00:00:00Z')`, version); err != nil {
		t.Fatal(err)
	}
	return path
}
