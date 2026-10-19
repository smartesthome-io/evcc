# PostgreSQL and TimescaleDB

evcc stores sessions, settings and energy history in SQLite by default. It can use PostgreSQL instead,
and uses TimescaleDB for its time series tables when the extension is available. Tested with
PostgreSQL 18 and TimescaleDB 2.30.

Use a database of its own for evcc. Backup and restore operate on the whole database.

## Configuration

```yaml
database:
  type: postgres
  dsn: postgres://user:password@host:5432/evcc
```

The dsn can also be given with `--database postgres://...` or the `EVCC_DATABASE_DSN` environment variable.
A `postgres://` dsn selects PostgreSQL on its own. evcc creates the tables when it starts.

Connections are named `evcc` (`application_name`), unless the dsn sets another name.

## Moving from SQLite

Stop evcc, then copy the database into an empty PostgreSQL database:

```sh
evcc db migrate --from ~/.evcc/evcc.db --to postgres://user:password@host:5432/evcc
```

The SQLite file is only read. The command refuses a target that already contains evcc tables,
`--force` drops them first. Start the SQLite file once with the current evcc version before migrating,
older files are rejected. Then configure the PostgreSQL dsn and start evcc.

## TimescaleDB

If the `timescaledb` extension is enabled in the database, evcc turns the tables `meters` and `tariffs`
into hypertables with 30 day chunks when it starts. Existing data is kept. Without the extension evcc
keeps plain tables and logs a warning, which goes away once the extension is enabled:

```sql
CREATE EXTENSION timescaledb;
```

evcc never creates the extension itself, this needs superuser rights.

## Backup

```sh
evcc db backup --to /var/backups/evcc/evcc.dump
```

`evcc db backup` dumps PostgreSQL with `pg_dump`, which has to be installed and must not be older than the
server. The backup can be taken while evcc is running. Without `--to` the file is named
`evcc-backup-<timestamp>.dump` in the current directory. An existing file is never overwritten.

The file contains passwords and tokens and is only readable for its owner. Keep it that way.

On SQLite the same command writes a copy of the database file.

## Restore

Stop evcc first, then:

```sh
evcc db restore --from /var/backups/evcc/evcc.dump
```

The tables of the backup replace the ones in the database. The command asks for confirmation unless `--yes` is
given, and runs `pg_restore` in a single transaction: a failing restore leaves the database as it was.
Afterwards start evcc.

The restore is refused while other evcc instances are connected to the database (`--force` overrides). Such an
instance is recognised by its connection name `evcc`. Instances started by an older evcc version or with their own
`application_name` in the dsn are not detected, so always stop evcc first.

On a database with the `timescaledb` extension, evcc drops its tables, prepares TimescaleDB with
`timescaledb_pre_restore()` and finishes with `timescaledb_post_restore()`. Should the restore fail after the
tables were dropped, run it again with the same file. If the command was killed in between, the database stays
in restore mode, run `SELECT timescaledb_post_restore();` in it and restore again.

Restoring a backup between the variants:

| Backup from | Restore into | Result |
|---|---|---|
| PostgreSQL | PostgreSQL | works |
| TimescaleDB | TimescaleDB | works, the extension version must be the same or newer |
| PostgreSQL | TimescaleDB | works, evcc turns the tables into hypertables when it starts |
| TimescaleDB | PostgreSQL | refused, enable the extension in the target first |

## Scheduled backups

`packaging/postgres` contains a script that keeps the newest 14 backups and a systemd service with a timer:

```sh
install -m 755 packaging/postgres/evcc-backup.sh /usr/local/bin/
install -m 644 packaging/postgres/evcc-backup.service packaging/postgres/evcc-backup.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now evcc-backup.timer
```

Put the database settings into `/etc/evcc-backup.env`, readable for root only, as the service reads them from there:

```sh
install -m 600 /dev/null /etc/evcc-backup.env
echo 'EVCC_DATABASE_DSN=postgres://user:password@host:5432/evcc' > /etc/evcc-backup.env
```

Without that file the service uses the database settings of `/etc/evcc.yaml`.

## Docker

The evcc image does not contain the PostgreSQL client tools. Run them in a container of the PostgreSQL image
that can reach the database. The examples assume the Docker network `evcc`, the database host `db`, the database
`evcc` and the user `evcc`.

Backup, works for PostgreSQL and TimescaleDB:

```sh
docker run --rm --network evcc -e PGPASSWORD=password -v "$PWD":/backup postgres:18.6 \
  pg_dump --host db --username evcc --format custom --file /backup/evcc.dump evcc
```

Restore into a new empty database, stop the evcc container first. For TimescaleDB use the matching image,
for example `timescale/timescaledb:2.30.2-pg18`:

```sh
PSQL="docker run --rm --network evcc -e PGPASSWORD=password postgres:18.6 psql --host db --username evcc"

$PSQL --dbname postgres --command "CREATE DATABASE evcc_restored"

# TimescaleDB only
$PSQL --dbname evcc_restored --command "CREATE EXTENSION IF NOT EXISTS timescaledb" --command "SELECT timescaledb_pre_restore()"

docker run --rm --network evcc -e PGPASSWORD=password -v "$PWD":/backup postgres:18.6 \
  pg_restore --host db --username evcc --dbname evcc_restored --no-owner --no-acl \
  --single-transaction --exit-on-error /backup/evcc.dump

# TimescaleDB only
$PSQL --dbname evcc_restored --command "SELECT timescaledb_post_restore()"
```

Then point the evcc dsn to `evcc_restored` and start evcc. The pre and post restore calls need a user with
superuser rights.
