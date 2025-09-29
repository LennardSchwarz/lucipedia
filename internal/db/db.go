package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Options controls how the SQLite database connection is initialised.
type Options struct {
	Path         string
	Logger       logger.Interface
	BusyTimeout  time.Duration
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxIdle  time.Duration
	ConnMaxLife  time.Duration
}

// Open establishes a SQLite connection using Gorm.
func Open(opts Options) (*gorm.DB, error) {
	if opts.Path == "" {
		return nil, eris.New("database path is required")
	}

	if opts.BusyTimeout == 0 {
		opts.BusyTimeout = 5 * time.Second
	}

	busyTimeout := opts.BusyTimeout
	busyTimeoutMillis := busyTimeout / time.Millisecond
	dsn := fmt.Sprintf("file:%s?_busy_timeout=%d&_foreign_keys=1&_journal_mode=WAL", opts.Path, busyTimeoutMillis)

	gormLogger := opts.Logger
	if gormLogger == nil {
		gormLogger = logger.Default.LogMode(logger.Warn)
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, eris.Wrap(err, "opening sqlite database")
	}

	if err := applyConnectionSettings(db, opts); err != nil {
		return nil, err
	}

	if err := enforcePragmas(db, busyTimeout); err != nil {
		return nil, err
	}

	return db, nil
}

func applyConnectionSettings(db *gorm.DB, opts Options) error {
	sqlDB, err := db.DB()
	if err != nil {
		return eris.Wrap(err, "retrieving sql.DB from gorm")
	}

	if opts.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(opts.MaxOpenConns)
	}

	if opts.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(opts.MaxIdleConns)
	}

	if opts.ConnMaxIdle > 0 {
		sqlDB.SetConnMaxIdleTime(opts.ConnMaxIdle)
	}

	if opts.ConnMaxLife > 0 {
		sqlDB.SetConnMaxLifetime(opts.ConnMaxLife)
	}

	return nil
}

func enforcePragmas(db *gorm.DB, busyTimeout time.Duration) error {
	timeoutMillis := int(busyTimeout / time.Millisecond)

	if err := db.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		return eris.Wrap(err, "enabling foreign keys pragma")
	}

	if err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", timeoutMillis)).Error; err != nil {
		return eris.Wrap(err, "configuring busy timeout pragma")
	}

	if err := db.Exec("PRAGMA journal_mode = WAL;").Error; err != nil {
		return eris.Wrap(err, "setting journal mode to WAL")
	}

	return nil
}

// Close releases the underlying database resources.
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return eris.Wrap(err, "retrieving sql.DB for close")
	}

	if err := sqlDB.Close(); err != nil {
		return eris.Wrap(err, "closing database connection")
	}

	return nil
}

// SQLDB exposes the underlying *sql.DB for advanced use cases.
func SQLDB(db *gorm.DB) (*sql.DB, error) {
	if db == nil {
		return nil, eris.New("gorm.DB is nil")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, eris.Wrap(err, "retrieving sql.DB")
	}

	return sqlDB, nil
}
