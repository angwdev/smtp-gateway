package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"
)

const (
	pragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)&_pragma=cache_size(-2000)&_pragma=journal_size_limit(8388608)"

	readPoolSize  = 4
	slowThreshold = 200 * time.Millisecond
)

// DB is one serialised writer plus a small read pool over the same file.
// SQLite allows a single writer; WAL lets readers run beside it, so reads
// (SMTP AUTH, panel lists) never queue behind a write.
type DB struct {
	Write *gorm.DB
	Read  *gorm.DB

	write, read *sql.DB
}

// Open migrates the database at path and returns both pools.
func Open(ctx context.Context, path string, log *slog.Logger) (*DB, error) {
	write, err := sql.Open("sqlite", "file:"+path+"?"+pragmas+"&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	if err := Migrate(ctx, write, log); err != nil {
		return nil, errors.Join(err, write.Close())
	}

	read, err := sql.Open("sqlite", "file:"+path+"?"+pragmas+"&_pragma=query_only(1)")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open readers: %w", err), write.Close())
	}
	read.SetMaxOpenConns(readPoolSize)
	read.SetMaxIdleConns(readPoolSize / 2)
	read.SetConnMaxIdleTime(5 * time.Minute)

	cfg := func() *gorm.Config {
		return &gorm.Config{
			Logger: logger.New(slog.NewLogLogger(log.Handler(), slog.LevelWarn), logger.Config{
				SlowThreshold:             slowThreshold,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
			}),
			PrepareStmt:            true,
			SkipDefaultTransaction: true, // explicit transactions only, see MessageRepository.Record
		}
	}
	gw, err := gorm.Open(gormsqlite.New(gormsqlite.Config{Conn: write}), cfg())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("gorm writer: %w", err), write.Close(), read.Close())
	}
	gr, err := gorm.Open(gormsqlite.New(gormsqlite.Config{Conn: read}), cfg())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("gorm readers: %w", err), write.Close(), read.Close())
	}
	return &DB{Write: gw, Read: gr, write: write, read: read}, nil
}

// Close releases both pools.
func (d *DB) Close() error { return errors.Join(d.read.Close(), d.write.Close()) }

// Ping checks that the database answers.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.read.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}
