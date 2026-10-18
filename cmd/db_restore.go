package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/evcc-io/evcc/db"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

const flagYes = "yes"

var dbRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore the database from a backup file",
	Long: `Restore the configured database from a backup file created by "evcc db backup".

All data in the database is replaced. Stop evcc before restoring. PostgreSQL databases
are restored with pg_restore, which must be installed. The restore is refused while
other evcc instances are connected to the database, unless --force is given.`,
	Args: cobra.NoArgs,
	Run:  runDBRestore,
}

func init() {
	dbCmd.AddCommand(dbRestoreCmd)

	// replaces the interrupt handling that exits at once
	dbRestoreCmd.PersistentPreRun = func(*cobra.Command, []string) {}

	dbRestoreCmd.Flags().String(flagFrom, "", "Backup file")
	dbRestoreCmd.Flags().Bool(flagYes, false, "Do not ask for confirmation")
	dbRestoreCmd.Flags().Bool(flagForce, false, "Restore even if other evcc instances are connected to the database")
	_ = dbRestoreCmd.MarkFlagRequired(flagFrom)
}

type restoreOptions struct {
	yes, force bool
	in         *os.File
}

func runDBRestore(cmd *cobra.Command, args []string) {
	typ, dsn, err := databaseFromConfig(cmd)
	if err != nil {
		log.FATAL.Fatal(err)
	}

	from, _ := cmd.Flags().GetString(flagFrom)
	opts := restoreOptions{in: os.Stdin}
	opts.yes, _ = cmd.Flags().GetBool(flagYes)
	opts.force, _ = cmd.Flags().GetBool(flagForce)

	// interrupting must not kill the process, the restore leaves TimescaleDB in restore mode otherwise
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := restoreDatabaseFile(ctx, typ, dsn, from, opts); err != nil {
		log.FATAL.Fatal(err)
	}

	log.INFO.Println("restore completed, start evcc")
}

// restoreDatabaseFile replaces the database content with a backup
func restoreDatabaseFile(ctx context.Context, typ, dsn, from string, opts restoreOptions) error {
	if _, err := os.Stat(from); err != nil {
		return fmt.Errorf("backup file: %w", err)
	}

	switch typ {
	case db.Postgres:
		return restorePostgres(ctx, dsn, from, opts)
	case db.SQLite:
		return restoreSQLite(ctx, dsn, from, opts)
	default:
		return fmt.Errorf("invalid database type: %s", typ)
	}
}

func restoreSQLite(ctx context.Context, dsn, from string, opts restoreOptions) error {
	if err := confirm("This replaces all data in database "+dsn+" with the backup. evcc must not be running.", opts.yes, opts.in); err != nil {
		return err
	}

	// opening does not migrate the database, evcc does when it starts
	conn, err := db.New(db.SQLite, dsn)
	if err != nil {
		return err
	}
	db.Instance = conn

	restoreErr := db.Restore(ctx, from)

	// closing checkpoints the write ahead log into the database file
	if err := db.Close(); restoreErr == nil {
		restoreErr = err
	}

	return restoreErr
}

func restorePostgres(ctx context.Context, dsn, from string, opts restoreOptions) error {
	env, database, err := pgEnv(dsn)
	if err != nil {
		return err
	}

	timescaleDump, err := archiveNeedsTimescale(ctx, from, env)
	if err != nil {
		return err
	}

	conn, err := db.New(db.Postgres, dsn)
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	// a single connection keeps the guard and the restore mode on the same session
	if sqlDB, err := conn.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	timescale := db.HasTimescale(conn)
	if timescaleDump && !timescale {
		return errors.New("the backup was taken from a TimescaleDB database, enable the timescaledb extension in the target database first")
	}

	if err := checkNoInstances(conn, opts.force); err != nil {
		return err
	}

	question := fmt.Sprintf("This replaces all data in database %s on %s with the backup.", database, envValue(env, "PGHOST"))
	if err := confirm(question, opts.yes, opts.in); err != nil {
		return err
	}

	// an instance may have started while waiting for the answer
	if err := checkNoInstances(conn, opts.force); err != nil {
		return err
	}

	dropped, restoring := false, false

	if timescale {
		// pg_restore cannot drop hypertables it does not know the chunks of, e.g. when restoring a plain backup.
		// Dropping must happen before the restore mode is enabled, it leaves stale hypertable metadata otherwise.
		if err := db.DropModelTables(conn); err != nil {
			return err
		}
		dropped = true

		if err := db.SetTimescaleRestoring(conn, true); err != nil {
			return fmt.Errorf("restore failed after the evcc tables were dropped, restore again from the backup file: %w", err)
		}

		// leave the database usable even if the restore fails or is interrupted
		restoring = true
		defer func() {
			if restoring {
				_ = db.SetTimescaleRestoring(conn, false)
			}
		}()
	}

	f, err := os.Open(from)
	if err != nil {
		return err
	}
	defer f.Close()

	// all or nothing: a failing restore leaves the database as it was
	args := []string{"--clean", "--if-exists", "--no-owner", "--no-acl", "--no-password", "--single-transaction", "--exit-on-error", "--dbname=" + database}
	if err := toolRunner(ctx, "pg_restore", args, env, f, nil); err != nil {
		if dropped {
			return fmt.Errorf("restore failed after the evcc tables were dropped, restore again from the backup file: %w", err)
		}
		return err
	}

	if restoring {
		restoring = false
		return db.SetTimescaleRestoring(conn, false)
	}

	return nil
}

// checkNoInstances refuses to restore while other evcc instances are connected to the database
func checkNoInstances(conn *gorm.DB, force bool) error {
	var others int64
	if err := conn.Raw(`SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND application_name = ? AND pid <> pg_backend_pid()`, db.ApplicationName).Scan(&others).Error; err != nil {
		return err
	}

	if others > 0 && !force {
		return fmt.Errorf("%d evcc connection(s) are active on this database, stop evcc first or use --force", others)
	}

	return nil
}

func envValue(env []string, key string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v
		}
	}
	return ""
}

// archiveNeedsTimescale checks that the file is a valid archive and whether it contains the timescaledb extension
func archiveNeedsTimescale(ctx context.Context, from string, env []string) (bool, error) {
	f, err := os.Open(from)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var list bytes.Buffer
	if err := toolRunner(ctx, "pg_restore", []string{"--list"}, env, f, &list); err != nil {
		return false, fmt.Errorf("invalid backup file: %w", err)
	}

	return strings.Contains(list.String(), " EXTENSION - timescaledb"), nil
}
