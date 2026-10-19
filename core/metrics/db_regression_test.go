package metrics

import (
	"os"
	"testing"
	"time"

	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/db/dbtest"
	"github.com/evcc-io/evcc/tariff"
	"github.com/jinzhu/now"
	"github.com/stretchr/testify/require"
)

// persistDay stores n consecutive 15min slots starting at start
func persistDay(t *testing.T, e entity, start time.Time, n int, energy float64) {
	t.Helper()
	for i := range n {
		require.NoError(t, persist(e, start.Add(time.Duration(i)*tariff.SlotDuration), energy, 0, nil, false))
	}
}

func TestSetupSchemaLegacyMeters(t *testing.T) {
	dbtest.SkipPostgres(t)
	dbtest.NewInstance(t)
	require.NoError(t, db.Instance.Create(&entity{Id: 2, Name: "pv1", Group: PV}).Error)

	// replace the current table with the oldest layout: datetime ts, val/neg columns, obsolete index
	require.NoError(t, db.Instance.Migrator().DropTable("meters"))
	for _, stmt := range []string{
		`CREATE TABLE meters (meter integer, ts datetime, val real, neg real)`,
		`CREATE UNIQUE INDEX meter_ts ON meters(meter, ts)`,
		`INSERT INTO meters (meter, ts, val, neg) VALUES (2, '2024-03-01 10:15:00+00:00', 1.5, 0.5)`,
	} {
		require.NoError(t, db.Instance.Exec(stmt).Error, stmt)
	}

	require.NoError(t, SetupSchema())

	m := db.Instance.Migrator()
	require.True(t, m.HasColumn(new(meter), "energy"))
	require.True(t, m.HasColumn(new(meter), "return_energy"))
	require.False(t, m.HasColumn(new(meter), "val"))
	require.False(t, m.HasColumn(new(meter), "neg"))
	require.False(t, m.HasIndex(new(meter), "meter_ts"))
	require.True(t, m.HasIndex(new(meter), "meters_meter_ts"))

	var typ string
	require.NoError(t, db.Instance.Raw(`SELECT typeof(ts) FROM meters`).Scan(&typ).Error)
	require.Equal(t, "integer", typ)

	var res meter
	require.NoError(t, db.Instance.First(&res).Error)
	require.Equal(t, time.Date(2024, 3, 1, 10, 15, 0, 0, time.UTC).Unix(), res.Timestamp)
	require.Equal(t, 1.5, res.Energy)
	require.Equal(t, 0.5, res.ReturnEnergy)

	// idempotent on an already migrated table
	require.NoError(t, SetupSchema())
}

func TestEnergyProfileRecoveredFilter(t *testing.T) {
	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	e := entity{Id: 2, Name: "pv1", Group: PV}
	require.NoError(t, db.Instance.Create(&e).Error)

	for day := -3; day < 0; day++ {
		persistDay(t, e, now.BeginningOfDay().AddDate(0, 0, day), 96, 1.0)
	}

	// slot 10 of the last day is recovered downtime energy and must not count
	last := now.BeginningOfDay().AddDate(0, 0, -1).Add(10 * tariff.SlotDuration)
	require.NoError(t, db.Instance.Model(new(meter)).Where("meter = ? AND ts = ?", e.Id, last.Unix()).
		Updates(map[string]any{"energy": 100.0, "recovered": true}).Error)

	// slot 20 of the middle day has a legacy NULL recovered flag and still counts
	legacy := now.BeginningOfDay().AddDate(0, 0, -2).Add(20 * tariff.SlotDuration)
	require.NoError(t, db.Instance.Exec(`UPDATE meters SET energy = 4, recovered = NULL WHERE meter = ? AND ts = ?`, e.Id, legacy.Unix()).Error)

	res, err := energyProfileFiltered(e, now.BeginningOfDay().AddDate(0, 0, -3), nil, 0)
	require.NoError(t, err)
	require.Equal(t, 1.0, res[10], "recovered slot is excluded")
	require.Equal(t, 2.0, res[20], "NULL recovered slot is included")
	require.Equal(t, 1.0, res[0])

	// same with the percentile path
	res, err = energyProfileFiltered(e, now.BeginningOfDay().AddDate(0, 0, -3), nil, 0.5)
	require.NoError(t, err)
	require.Equal(t, 1.0, res[10])
	require.Equal(t, 1.0, res[20], "median of 1, 1, 4")
}

func TestEnergyProfileTemperatureBinnedRecoveredFilter(t *testing.T) {
	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	e := entity{Id: 2, Name: "heater1", Group: Loadpoint}
	require.NoError(t, db.Instance.Create(&e).Error)

	temp := 10.0
	for day := -3; day < 0; day++ {
		base := now.BeginningOfDay().AddDate(0, 0, day)
		persistDay(t, e, base, 96, 1.0)
		for i := range 96 {
			require.NoError(t, PersistTariffs(base.Add(time.Duration(i)*tariff.SlotDuration), nil, nil, nil, &temp))
		}
	}

	last := now.BeginningOfDay().AddDate(0, 0, -1).Add(5 * tariff.SlotDuration)
	require.NoError(t, db.Instance.Model(new(meter)).Where("meter = ? AND ts = ?", e.Id, last.Unix()).
		Updates(map[string]any{"energy": 100.0, "recovered": true}).Error)

	res, err := energyProfileTemperatureBinned(e)
	require.NoError(t, err)
	require.Len(t, res, 96)
	require.InDelta(t, 1.0, res[5][10], 1e-9, "recovered slot is excluded")
	require.InDelta(t, 1.0, res[6][10], 1e-9)
}

func TestQueryEnergyDayMonthBuckets(t *testing.T) {
	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	e := entity{Id: 2, Name: "pv1", Group: PV}
	require.NoError(t, db.Instance.Create(&e).Error)

	// local 23:45 on the last day of March, then the first two slots of April
	for i, energy := range []float64{1, 2, 4} {
		ts := time.Date(2026, 3, 31, 23, 45, 0, 0, time.Local).Add(time.Duration(i) * tariff.SlotDuration)
		require.NoError(t, persist(e, ts, energy, 0, nil, false))
	}

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)

	for _, tc := range []struct {
		aggregate string
		starts    []time.Time
		energy    []float64
	}{
		{"day", []time.Time{
			time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local),
		}, []float64{1, 6}},
		{"month", []time.Time{
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local),
			time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local),
		}, []float64{1, 6}},
	} {
		t.Run(tc.aggregate, func(t *testing.T) {
			res, err := QueryEnergy(from, to, tc.aggregate, false)
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Len(t, res[0].Data, len(tc.starts))
			for i, slot := range res[0].Data {
				require.True(t, tc.starts[i].Equal(slot.Start), "start %d: want %v, got %v", i, tc.starts[i], slot.Start)
				require.InDelta(t, tc.energy[i], slot.Energy, 1e-9, "energy %d", i)
			}
		})
	}
}

func TestQueryTariffsDayMonthBuckets(t *testing.T) {
	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	for i, grid := range []float64{0.2, 0.4, 0.9} {
		ts := time.Date(2026, 3, 31, 23, 45, 0, 0, time.Local).Add(time.Duration(i) * tariff.SlotDuration)
		require.NoError(t, PersistTariffs(ts, &grid, nil, nil, nil))
	}

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)

	for _, aggregate := range []string{"day", "month"} {
		t.Run(aggregate, func(t *testing.T) {
			res, err := QueryTariffs(from, to, aggregate)
			require.NoError(t, err)
			require.Len(t, res, 2)
			require.InDelta(t, 0.2, *res[0].Grid, 1e-9)
			require.InDelta(t, 0.65, *res[1].Grid, 1e-9)
			require.InDelta(t, 0.4, *res[1].GridMin, 1e-9)
			require.InDelta(t, 0.9, *res[1].GridMax, 1e-9)
		})
	}
}

// SQLite resolves 'localtime' from the TZ at process start, so this runs only with TZ=Europe/Berlin
func TestQueryEnergyDST(t *testing.T) {
	if os.Getenv("TZ") != "Europe/Berlin" {
		t.Skip("requires TZ=Europe/Berlin")
	}

	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	e := entity{Id: 2, Name: "pv1", Group: PV}
	require.NoError(t, db.Instance.Create(&e).Error)

	for _, tc := range []struct {
		name  string
		day   time.Time
		slots int
		hours int
	}{
		{"spring forward", time.Date(2026, 3, 29, 0, 0, 0, 0, time.Local), 92, 23},
		{"fall back", time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local), 100, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			persistDay(t, e, tc.day, tc.slots, 1.0)
			// first slot of the following day lands in its own bucket
			next := tc.day.AddDate(0, 0, 1)
			require.NoError(t, persist(e, next, 10, 0, nil, false))

			res, err := QueryEnergy(tc.day, next.AddDate(0, 0, 1), "day", false)
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Len(t, res[0].Data, 2)
			require.True(t, tc.day.Equal(res[0].Data[0].Start))
			require.InDelta(t, float64(tc.slots), res[0].Data[0].Energy, 1e-9)
			require.InDelta(t, 10.0, res[0].Data[1].Energy, 1e-9)

			res, err = QueryEnergy(tc.day, next, "hour", false)
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Len(t, res[0].Data, tc.hours)
		})
	}
}

func TestQueryEnergySeriesOrderIsBytewise(t *testing.T) {
	dbtest.NewInstance(t)
	require.NoError(t, SetupSchema())

	ts := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	for i, title := range []string{"solar roof", "Zeta", "alpha"} {
		e := entity{Id: 2 + i, Name: "e" + title, Group: Meter, Title: title}
		require.NoError(t, db.Instance.Create(&e).Error)
		require.NoError(t, persist(e, ts, 1, 0, nil, false))
	}

	res, err := QueryEnergy(ts.Add(-time.Hour), ts.Add(time.Hour), "hour", false)
	require.NoError(t, err)

	var titles []string
	for _, s := range res {
		titles = append(titles, s.Title)
	}
	require.Equal(t, []string{"Zeta", "alpha", "solar roof"}, titles)
}

func TestTimeSeriesTablesAreHypertables(t *testing.T) {
	dbtest.RequireTimescale(t)
	dbtest.NewInstance(t)

	hypertables := func() map[string]int64 {
		var rows []struct {
			Name     string
			Interval int64
		}
		require.NoError(t, db.Instance.Raw(`SELECT hypertable_name AS name, integer_interval AS interval
			FROM timescaledb_information.dimensions WHERE hypertable_schema = current_schema()`).Scan(&rows).Error)

		res := make(map[string]int64)
		for _, r := range rows {
			res[r.Name] = r.Interval
		}
		return res
	}

	chunk := int64(hypertableChunk.Seconds())
	require.Equal(t, map[string]int64{"meters": chunk, "tariffs": chunk}, hypertables())

	e := entity{Id: 2, Name: "pv1", Group: PV}
	require.NoError(t, db.Instance.Create(&e).Error)
	persistDay(t, e, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), 96, 1.0)

	// restart on converted tables
	require.NoError(t, SetupSchema())
	require.NoError(t, db.Instance.AutoMigrate(new(tariffValue)))
	require.Equal(t, map[string]int64{"meters": chunk, "tariffs": chunk}, hypertables())

	var count int64
	require.NoError(t, db.Instance.Model(new(meter)).Count(&count).Error)
	require.EqualValues(t, 96, count)
}

func TestTimestampColumnsAreNotNullOnFreshSQLite(t *testing.T) {
	dbtest.SkipPostgres(t)
	dbtest.NewInstance(t)

	for _, table := range []string{"meters", "tariffs"} {
		var notNull int
		require.NoError(t, db.Instance.Raw(`SELECT "notnull" FROM pragma_table_info(?) WHERE name = 'ts'`, table).Scan(&notNull).Error)
		require.Equal(t, 1, notNull, table)
	}
}

func TestSetupSchemaKeepsNullableTimestamp(t *testing.T) {
	dbtest.SkipPostgres(t)
	dbtest.NewInstance(t)

	// a database created before ts was declared not null
	require.NoError(t, db.Instance.Migrator().DropTable("meters", "tariffs"))
	for _, stmt := range []string{
		`CREATE TABLE meters (meter integer, ts integer, energy real, return_energy real, soc_temp real, recovered numeric)`,
		`CREATE UNIQUE INDEX meters_meter_ts ON meters(meter, ts)`,
		`INSERT INTO meters (meter, ts, energy) VALUES (1, 900, 1.5)`,
		`CREATE TABLE tariffs (ts integer, grid real, feedin real, co2 real, temperature real)`,
		`CREATE UNIQUE INDEX idx_tariffs_timestamp ON tariffs(ts)`,
	} {
		require.NoError(t, db.Instance.Exec(stmt).Error, stmt)
	}

	require.NoError(t, SetupSchema())
	require.NoError(t, db.Instance.AutoMigrate(new(tariffValue)))

	for _, table := range []string{"meters", "tariffs"} {
		var notNull int
		require.NoError(t, db.Instance.Raw(`SELECT "notnull" FROM pragma_table_info(?) WHERE name = 'ts'`, table).Scan(&notNull).Error)
		require.Zero(t, notNull, "%s is not rebuilt", table)
	}

	var count int64
	require.NoError(t, db.Instance.Table("meters").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
