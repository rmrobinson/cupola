package store

import "testing"

func TestNewSQLiteStoreFreshDataDirMigrates(t *testing.T) {
	s, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer s.db.Close()

	tables := []string{
		"profiles",
		"notes",
		"transit_agencies",
	}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing after migration: %v", table, err)
		}
	}
}
