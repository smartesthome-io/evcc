#!/bin/sh
# Back up the evcc database and keep the newest backups.
# The database is taken from the evcc configuration, see docs/postgres.md.
set -eu

BACKUP_DIR="${BACKUP_DIR:-/var/backups/evcc}"
KEEP="${KEEP:-14}"
EVCC="${EVCC:-evcc}"

# backups contain credentials
umask 077
mkdir -p "$BACKUP_DIR"

"$EVCC" db backup --to "$BACKUP_DIR/evcc-$(date +%Y%m%d-%H%M%S).dump"

# delete all but the newest backups
ls -1t "$BACKUP_DIR"/evcc-*.dump | tail -n "+$((KEEP + 1))" | while read -r old; do
	rm -- "$old"
done
