package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/evcc-io/evcc/db"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

const (
	flagFrom = "from"
	flagTo   = "to"

	migrateBatchSize = 1000
)

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage the database",
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Copy a SQLite database to PostgreSQL",
	Long: `Copy an existing SQLite database into an empty PostgreSQL database.

evcc must not be running. The source file is opened read-only and never modified.
Afterwards configure the target with database type "postgres" and its dsn and start evcc.`,
	Args: cobra.NoArgs,
	Run:  runDBMigrate,
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbMigrateCmd)

	dbMigrateCmd.Flags().String(flagFrom, userDB, "SQLite database file")
	dbMigrateCmd.Flags().String(flagTo, "", "PostgreSQL dsn, e.g. postgres://user:password@host:5432/evcc")
	dbMigrateCmd.Flags().Bool(flagForce, false, "Drop existing evcc tables in the target first")
	_ = dbMigrateCmd.MarkFlagRequired(flagTo)
}

func runDBMigrate(cmd *cobra.Command, args []string) {
	from, _ := cmd.Flags().GetString(flagFrom)
	to, _ := cmd.Flags().GetString(flagTo)
	force, _ := cmd.Flags().GetBool(flagForce)

	if err := migrateDatabase(from, to, force); err != nil {
		log.FATAL.Fatal(err)
	}
}

// migrateDatabase copies the SQLite database file at from into the empty PostgreSQL database at to
func migrateDatabase(from, to string, force bool) error {
	if db.DriverFromDSN("", to) != db.Postgres {
		return errors.New("target must be a postgres dsn (postgres://...)")
	}

	path, err := homedir.Expand(from)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("source database: %w", err)
	}

	src, err := db.OpenReadOnly(path)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer func() {
		if sqlDB, err := src.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := db.CheckSchema(src); err != nil {
		return err
	}

	// meters.ts was once stored as text, copying it would silently produce wrong timestamps
	var typ string
	if err := src.Raw("SELECT type FROM pragma_table_info('meters') WHERE name = 'ts'").Scan(&typ).Error; err != nil {
		return err
	}
	if !strings.EqualFold(typ, "INTEGER") {
		return fmt.Errorf("%w: meters.ts has type %q", db.ErrOutdated, typ)
	}

	if err := prepareTarget(to, force); err != nil {
		return err
	}

	// creates the schema through the regular migrations
	if err := db.NewInstance(db.Postgres, to); err != nil {
		return err
	}
	defer db.Close()

	counts, err := db.Copy(src, db.Instance, migrateBatchSize)
	if err != nil {
		return err
	}

	if err := db.ResetSequences(db.Instance); err != nil {
		return err
	}

	for _, m := range db.Models() {
		table, err := db.TableName(db.Instance, m)
		if err != nil {
			return err
		}
		log.INFO.Printf("copied %6d rows: %s", counts[table], table)
	}

	return nil
}

// prepareTarget makes sure the target holds no evcc tables, dropping them with force
func prepareTarget(dsn string, force bool) error {
	probe, err := db.New(db.Postgres, dsn)
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, err := probe.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	existing, err := db.HasModelTables(probe)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	if !force {
		return fmt.Errorf("target database is not empty, it contains %s (use --force to drop them)", strings.Join(existing, ", "))
	}

	log.WARN.Println("dropping existing tables in target:", strings.Join(existing, ", "))

	return db.DropModelTables(probe)
}
