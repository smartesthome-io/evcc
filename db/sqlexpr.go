package db

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/evcc-io/evcc/util"
	"gorm.io/gorm"
)

var (
	zoneOnce sync.Once
	zoneName string

	zonePattern = regexp.MustCompile(`^[A-Za-z0-9_+\-/]+$`)

	// strftime directives and their to_char counterparts
	formatDirectives = strings.NewReplacer("%Y", "YYYY", "%m", "MM", "%d", "DD", "%H", "HH24", "%M", "MI")
)

// LocalZone returns the IANA name of the process time zone. SQLite resolves
// 'localtime' itself, PostgreSQL needs the zone named explicitly.
func LocalZone() string {
	zoneOnce.Do(func() {
		zoneName = resolveZone()
	})
	return zoneName
}

func resolveZone() string {
	candidates := []string{time.Local.String(), strings.TrimPrefix(os.Getenv("TZ"), ":")}

	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			candidates = append(candidates, zone)
		}
	}

	for _, name := range candidates {
		if name == "" || name == "Local" || !zonePattern.MatchString(name) {
			continue
		}
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}

	util.NewLogger("db").WARN.Println("cannot determine time zone, using UTC for time based statistics. Set the TZ environment variable")

	return "UTC"
}

// checkZone verifies that the connection knows the process time zone
func checkZone(conn *gorm.DB) error {
	var ts time.Time
	if err := conn.Raw(fmt.Sprintf("SELECT now() AT TIME ZONE '%s'", LocalZone())).Scan(&ts).Error; err != nil {
		return fmt.Errorf("time zone %s: %w", LocalZone(), err)
	}
	return nil
}

// BinaryCollation returns an SQL expression comparing text byte-wise, the SQLite default
func BinaryCollation(conn *gorm.DB, expr string) string {
	if Dialect(conn) == Postgres {
		return "(" + expr + ` COLLATE "C")`
	}
	return expr
}

// LocalTime returns an SQL expression formatting a unix seconds column in the local time zone.
// The format uses the strftime directives %Y %m %d %H %M.
func LocalTime(conn *gorm.DB, col, format string) string {
	if Dialect(conn) == Postgres {
		return fmt.Sprintf("to_char(to_timestamp(%s) AT TIME ZONE '%s', '%s')", col, LocalZone(), formatDirectives.Replace(format))
	}
	return fmt.Sprintf("strftime('%s', %s, 'unixepoch', 'localtime')", format, col)
}

// LocalTimestamp is LocalTime for timestamp columns
func LocalTimestamp(conn *gorm.DB, col, format string) string {
	if Dialect(conn) == Postgres {
		return fmt.Sprintf("to_char(%s AT TIME ZONE '%s', '%s')", col, LocalZone(), formatDirectives.Replace(format))
	}
	return fmt.Sprintf("strftime('%s', %s, 'localtime')", format, col)
}

// LocalWeekday returns an SQL expression for the local weekday of a unix seconds column, 0 is Sunday
func LocalWeekday(conn *gorm.DB, col string) string {
	if Dialect(conn) == Postgres {
		return fmt.Sprintf("CAST(EXTRACT(DOW FROM to_timestamp(%s) AT TIME ZONE '%s') AS INTEGER)", col, LocalZone())
	}
	return fmt.Sprintf("CAST(strftime('%%w', %s, 'unixepoch', 'localtime') AS INTEGER)", col)
}
