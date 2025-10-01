package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenRequiresPath(t *testing.T) {
	t.Parallel()

	_, err := Open(Options{})
	if err == nil {
		t.Fatalf("expected error when no path supplied")
	}
}

func TestOpenAppliesPragmasWithDefaultTimeout(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "lucipedia.db")

	database, err := Open(Options{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := Close(database); closeErr != nil {
			t.Errorf("closing database failed: %v", closeErr)
		}
	})

	var foreignKeys int
	if queryErr := database.Raw("PRAGMA foreign_keys;").Scan(&foreignKeys).Error; queryErr != nil {
		t.Fatalf("querying foreign_keys pragma failed: %v", queryErr)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign keys pragma to be enabled, got %d", foreignKeys)
	}

	var journalMode string
	if queryErr := database.Raw("PRAGMA journal_mode;").Scan(&journalMode).Error; queryErr != nil {
		t.Fatalf("querying journal_mode pragma failed: %v", queryErr)
	}
	if !strings.EqualFold(strings.TrimSpace(journalMode), "wal") {
		t.Fatalf("expected journal mode WAL, got %q", journalMode)
	}

	var busyTimeout int
	if queryErr := database.Raw("PRAGMA busy_timeout;").Scan(&busyTimeout).Error; queryErr != nil {
		t.Fatalf("querying busy_timeout pragma failed: %v", queryErr)
	}

	expectedTimeout := int((5 * time.Second) / time.Millisecond)
	if busyTimeout != expectedTimeout {
		t.Fatalf("expected busy timeout %d, got %d", expectedTimeout, busyTimeout)
	}
}

func TestOpenHonoursBusyTimeoutAndConnectionLimits(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "lucipedia_custom_test.db")
	opts := Options{
		Path:         path,
		BusyTimeout:  1500 * time.Millisecond,
		MaxOpenConns: 7,
		MaxIdleConns: 3,
		ConnMaxIdle:  2 * time.Second,
		ConnMaxLife:  5 * time.Second,
	}

	database, err := Open(opts)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := Close(database); closeErr != nil {
			t.Errorf("closing database failed: %v", closeErr)
		}
	})

	var busyTimeout int
	if queryErr := database.Raw("PRAGMA busy_timeout;").Scan(&busyTimeout).Error; queryErr != nil {
		t.Fatalf("querying busy_timeout pragma failed: %v", queryErr)
	}

	expectedTimeout := int(opts.BusyTimeout / time.Millisecond)
	if busyTimeout != expectedTimeout {
		t.Fatalf("expected busy timeout %d, got %d", expectedTimeout, busyTimeout)
	}

	sqlDB, err := SQLDB(database)
	if err != nil {
		t.Fatalf("SQLDB returned error: %v", err)
	}

	if stats := sqlDB.Stats(); stats.MaxOpenConnections != opts.MaxOpenConns {
		t.Fatalf("expected MaxOpenConns %d, got %d", opts.MaxOpenConns, stats.MaxOpenConnections)
	}
}

func TestSQLDBWithNilDatabase(t *testing.T) {
	t.Parallel()

	_, err := SQLDB(nil)
	if err == nil {
		t.Fatalf("expected error when database is nil")
	}
}
