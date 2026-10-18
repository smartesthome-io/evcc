package db

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrOutdated is returned when the source database lacks tables or columns of the current version
var ErrOutdated = errors.New("database schema is outdated, start the current evcc version once on it first")

// OpenReadOnly opens an existing SQLite database file without modifying it
func OpenReadOnly(path string) (*gorm.DB, error) {
	u := url.URL{Scheme: "file", Opaque: path, RawQuery: "mode=ro"}

	return gorm.Open(sqlite.Open(u.String()), &gorm.Config{})
}

// HasModelTables returns the tables of the registered models that exist in conn
func HasModelTables(conn *gorm.DB) ([]string, error) {
	var res []string

	for _, m := range Models() {
		table, err := TableName(conn, m)
		if err != nil {
			return nil, err
		}

		if conn.Migrator().HasTable(table) {
			res = append(res, table)
		}
	}

	return res, nil
}

// DropModelTables drops the tables of the registered models, referencing tables first
func DropModelTables(conn *gorm.DB) error {
	all := Models()

	for i := len(all) - 1; i >= 0; i-- {
		if err := conn.Migrator().DropTable(all[i]); err != nil {
			return err
		}
	}

	return nil
}

// CheckSchema verifies that src has all tables and columns of the registered models
func CheckSchema(src *gorm.DB) error {
	for _, m := range Models() {
		stmt := &gorm.Statement{DB: src}
		if err := stmt.Parse(m); err != nil {
			return err
		}

		if !src.Migrator().HasTable(stmt.Schema.Table) {
			return fmt.Errorf("%w: missing table %s", ErrOutdated, stmt.Schema.Table)
		}

		for _, field := range stmt.Schema.Fields {
			if field.DBName == "" {
				continue
			}
			if !src.Migrator().HasColumn(m, field.DBName) {
				return fmt.Errorf("%w: missing column %s.%s", ErrOutdated, stmt.Schema.Table, field.DBName)
			}
		}
	}

	return nil
}

// Copy copies the registered models from src to dst in registration order.
// dst must carry the schema already. Rows with an existing primary key are replaced.
// It returns the number of rows copied per table and fails if dst holds a different count afterwards.
func Copy(src, dst *gorm.DB, batchSize int) (map[string]int64, error) {
	res := make(map[string]int64)

	for _, m := range Models() {
		stmt := &gorm.Statement{DB: dst}
		if err := stmt.Parse(m); err != nil {
			return nil, err
		}
		table := stmt.Schema.Table

		var copied int64
		err := dst.Transaction(func(tx *gorm.DB) error {
			var err error
			copied, err = copyTable(src, tx, m, stmt.Schema.PrioritizedPrimaryField != nil, batchSize)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", table, err)
		}

		res[table] = copied

		var have int64
		if err := dst.Model(m).Count(&have).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		if have != copied {
			return nil, fmt.Errorf("copy %s: %d rows read, %d rows in target", table, copied, have)
		}
	}

	return res, nil
}

func copyTable(src, dst *gorm.DB, model any, hasPK bool, batchSize int) (int64, error) {
	elem := reflect.TypeOf(model).Elem()

	rows, err := src.Model(model).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total int64
	batch := reflect.MakeSlice(reflect.SliceOf(elem), 0, batchSize)

	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}

		tx := dst
		if hasPK {
			tx = tx.Clauses(clause.OnConflict{UpdateAll: true})
		}

		ptr := reflect.New(batch.Type())
		ptr.Elem().Set(batch)

		if err := tx.Create(ptr.Interface()).Error; err != nil {
			return err
		}

		total += int64(batch.Len())
		batch = batch.Slice(0, 0)

		return nil
	}

	for rows.Next() {
		item := reflect.New(elem)
		if err := src.ScanRows(rows, item.Interface()); err != nil {
			return 0, err
		}

		batch = reflect.Append(batch, item.Elem())
		if batch.Len() >= batchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return total, flush()
}

// ResetSequences moves the id sequences of the registered models behind the highest copied id (PostgreSQL)
func ResetSequences(conn *gorm.DB) error {
	if Dialect(conn) != Postgres {
		return nil
	}

	for _, m := range Models() {
		stmt := &gorm.Statement{DB: conn}
		if err := stmt.Parse(m); err != nil {
			return err
		}

		pk := stmt.Schema.PrioritizedPrimaryField
		if pk == nil || !pk.AutoIncrement {
			continue
		}

		table, col := stmt.Quote(stmt.Schema.Table), stmt.Quote(pk.DBName)
		sql := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence(?, ?), COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false)`, col, table)

		var next int64
		if err := conn.Raw(sql, strings.Trim(table, `"`), pk.DBName).Scan(&next).Error; err != nil {
			return fmt.Errorf("reset sequence %s: %w", stmt.Schema.Table, err)
		}
	}

	return nil
}

// TableName returns the table of a registered model
func TableName(conn *gorm.DB, model any) (string, error) {
	stmt := &gorm.Statement{DB: conn}
	if err := stmt.Parse(model); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}
