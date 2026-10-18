package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evcc-io/evcc/db"
	"github.com/spf13/cobra"
)

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Write a backup of the database to a file",
	Long: `Write a backup of the configured database to a file.

PostgreSQL databases are dumped with pg_dump, which must be installed. The backup
can be taken while evcc is running. The file contains credentials and is created
readable for the current user only. An existing file is never overwritten.`,
	Args: cobra.NoArgs,
	Run:  runDBBackup,
}

func init() {
	dbCmd.AddCommand(dbBackupCmd)

	dbBackupCmd.Flags().String(flagTo, "", "Backup file (default evcc-backup-<timestamp>.dump or .db)")
}

func runDBBackup(cmd *cobra.Command, args []string) {
	typ, dsn, err := databaseFromConfig(cmd)
	if err != nil {
		log.FATAL.Fatal(err)
	}

	to, _ := cmd.Flags().GetString(flagTo)
	if to == "" {
		to = defaultBackupName(typ, time.Now())
	}

	if err := backupDatabase(context.Background(), typ, dsn, to); err != nil {
		log.FATAL.Fatal(err)
	}

	log.INFO.Println("backup written to", to)
}

func defaultBackupName(typ string, now time.Time) string {
	ext := "db"
	if typ == db.Postgres {
		ext = "dump"
	}
	return fmt.Sprintf("evcc-backup-%s.%s", now.Format("20060102-150405"), ext)
}

// backupDatabase writes a backup of the database to a new file
func backupDatabase(ctx context.Context, typ, dsn, to string) error {
	switch typ {
	case db.Postgres:
		return backupPostgres(ctx, dsn, to)
	case db.SQLite:
		return backupSQLite(ctx, dsn, to)
	default:
		return fmt.Errorf("invalid database type: %s", typ)
	}
}

func backupPostgres(ctx context.Context, dsn, to string) error {
	env, _, err := pgEnv(dsn)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	err = toolRunner(ctx, "pg_dump", []string{"--format=custom", "--no-password"}, env, nil, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(to)
		return err
	}

	return nil
}

func backupSQLite(ctx context.Context, dsn, to string) error {
	path, err := db.SQLitePath(dsn)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("database: %w", err)
	}

	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s already exists", to)
	}

	// opening does not migrate the database
	conn, err := db.New(db.SQLite, dsn)
	if err != nil {
		return err
	}
	db.Instance = conn
	defer func() { _ = db.Close() }()

	if err := db.Backup(ctx, to); err != nil {
		_ = os.Remove(to)
		return err
	}

	return os.Chmod(to, 0o600)
}
