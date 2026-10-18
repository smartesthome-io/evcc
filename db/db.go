package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evcc-io/evcc/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/libtnb/sqlite"
	"github.com/mitchellh/go-homedir"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"
)

const (
	SQLite   = "sqlite"
	Postgres = "postgres"

	// ApplicationName identifies evcc connections on the PostgreSQL server
	ApplicationName = "evcc"
)

var (
	Instance *gorm.DB
	filePath string // Store the actual SQLite file path
	location string // Human readable server location of a network database
)

func FilePath() string {
	return filePath
}

// Location describes where the database lives, without credentials
func Location() string {
	if filePath != "" {
		return filePath
	}
	return location
}

// Dialect returns the database type of the connection
func Dialect(conn *gorm.DB) string {
	if conn == nil {
		return ""
	}
	return conn.Dialector.Name()
}

// DriverFromDSN returns the driver for a dsn that unambiguously names one, otherwise the given driver
func DriverFromDSN(driver, dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return Postgres
	}
	return driver
}

func New(driver, dsn string) (*gorm.DB, error) {
	var dialect gorm.Dialector
	var pgDB *sql.DB
	var memory bool

	switch driver {
	case SQLite:
		// Split database path and connection parameters
		dbPath, params, _ := strings.Cut(dsn, "?")

		file, err := homedir.Expand(dbPath)
		if err != nil {
			return nil, err
		}

		memory = file == ":memory:" || strings.Contains(params, "mode=memory")

		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return nil, err
		}

		// Store the expanded file path for later use
		filePath, location = file, ""

		// WAL with synchronous NORMAL syncs to disk on checkpoint instead of every commit,
		// a power loss may drop the latest commits but cannot corrupt the database.
		// It creates evcc.db-wal and evcc.db-shm next to the database.
		for _, pragma := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)", "foreign_keys(1)", "auto_vacuum(INCREMENTAL)"} {
			// add pragma if not already present
			if short, _, _ := strings.Cut(pragma, "("); strings.Contains(params, "_pragma="+short) {
				continue
			}

			// append '&' if there are existing connection parameters
			if len(params) > 0 {
				params += "&"
			}

			params += "_pragma=" + pragma
		}

		// take the write lock on BEGIN, so concurrent writers wait on busy_timeout
		// instead of failing with SQLITE_BUSY when upgrading from a read
		if !strings.Contains(params, "_txlock=") {
			params += "&_txlock=immediate"
		}

		connectionStr := file + "?" + params

		util.NewLogger("main").INFO.Println("using sqlite database:", connectionStr)

		dialect = sqlite.Open(connectionStr)

	case Postgres:
		// parsing redacts the password in its errors
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, fmt.Errorf("invalid postgres dsn: %w", err)
		}

		if _, ok := cfg.RuntimeParams["application_name"]; !ok {
			cfg.RuntimeParams["application_name"] = ApplicationName
		}

		filePath = ""
		location = fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, cfg.Database)

		util.NewLogger("main").INFO.Println("using postgres database:", location)

		pgDB = stdlib.OpenDB(*cfg)
		dialect = postgres.New(postgres.Config{Conn: pgDB})
	default:
		return nil, fmt.Errorf("invalid database type: %s not in [%s, %s]", driver, SQLite, Postgres)
	}

	db, err := gorm.Open(dialect, &gorm.Config{
		Logger: &Logger{util.NewLogger("db")},
	})
	if err != nil {
		if pgDB != nil {
			_ = pgDB.Close()
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}
		return nil, err
	}

	if driver == Postgres {
		if err := checkZone(db); err != nil {
			_ = pgDB.Close()
			return nil, err
		}
	}

	// WAL lets readers run alongside the single writer. In-memory databases
	// exist per connection and need exactly one.
	if sqlDB, err := db.DB(); err == nil && memory {
		sqlDB.SetMaxOpenConns(1)
	}

	return db, nil
}

func NewInstance(driver, dsn string) error {
	db, err := New(strings.ToLower(driver), dsn)
	if err != nil {
		return err
	}

	Instance = db

	mu.Lock()
	defer mu.Unlock()

	for _, f := range registry {
		if err := f(db); err != nil {
			return err
		}
	}

	return nil
}

func Close() error {
	db, err := Instance.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// IsReadonly reports whether err indicates a database that is not
// writable by the current user. The code mask covers extended result codes.
func IsReadonly(err error) bool {
	if serr, ok := errors.AsType[*sqlite3.Error](err); ok {
		return serr.Code()&0xff == sqlite3lib.SQLITE_READONLY
	}

	// read_only_sql_transaction
	perr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && perr.Code == "25006"
}

type backuper interface {
	NewBackup(string) (*sqlite3.Backup, error)
	NewRestore(string) (*sqlite3.Backup, error)
}

func runWithBackuper(ctx context.Context, fun func(backuper) (*sqlite3.Backup, error)) error {
	live, err := Instance.DB()
	if err != nil {
		return err
	}

	conn, err := live.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		conn, ok := driverConn.(backuper)
		if !ok {
			return errors.New("invalid db type")
		}

		bck, err := fun(conn)
		if err != nil {
			return err
		}

		if _, err := bck.Step(-1); err != nil {
			return err
		}

		return bck.Finish()
	})
}

func Backup(ctx context.Context, target string) error {
	return runWithBackuper(ctx, func(conn backuper) (*sqlite3.Backup, error) {
		return conn.NewBackup(target)
	})
}

func Restore(ctx context.Context, target string) error {
	return runWithBackuper(ctx, func(conn backuper) (*sqlite3.Backup, error) {
		return conn.NewRestore(target)
	})
}
