PG_HOST=$1
PG_PASS=$2

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

drop_table(){
	echo "deleting tables if they exist"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "DROP TABLE t"
}

create_table_t(){
	echo "Creating table 't'"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "CREATE TABLE IF NOT EXISTS t (a int, b int);"
}

insert_into_t() {
	echo "inserting data into table 't' "
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "INSERT INTO t SELECT 0, b from generate_series(1, (10^8) ::int) b;"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "ANALYZE t;"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "INSERT INTO t SELECT 1, b from generate_series(1, (5*10^6) ::int) b;"
}

# ensure all params are passed before executing psql commands.
if [ -z "${PG_HOST}" ] | [ -z "${PG_PASS}" ]; then
  echo "host and password required and not set - exiting"
  exit 1
fi

drop_table
create_table_t
insert_into_t