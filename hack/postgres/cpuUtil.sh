PG_HOST=$1
PG_PASS=$2

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

drop_tables(){
	echo "deleting tables if they exist"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "DROP TABLE stuff, stuff2;"
}

create_tables(){
	echo "Creating table 'stuff'"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "CREATE TABLE IF NOT EXISTS stuff (thing varchar, otherthing varchar,thirdthing varchar, number int);"
	echo "Creating table 'stuff2'"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "CREATE TABLE IF NOT EXISTS stuff2 (thing varchar, otherthing varchar,thirdthing varchar, number int);"
}

insert_data() {
	INSERT="INSERT into stuff (thing, otherthing, number, thirdthing) select md5(random()::text), md5(random()::text), random()*4000, md5(random()::text)from generate_series(1, 1000) s(i);"
	INSERT2="INSERT into stuff2 (thing, otherthing, number, thirdthing) select md5(random()::text), md5(random()::text),random()*5000, md5(random()::text)from generate_series(1, 1000) s(i);"

	echo "Starting to fill tables"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "$INSERT"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "$INSERT2"
}

# busywork to raise cpuUtil to >90%
select_data() {
	echo "starting busywork"
	echo "SELECT concat(stuff.thing, stuff2.otherthing) AS combinedThing, concat(stuff.number^3, stuff2.number / stuff.number) AS combinedNum FROM stuff, stuff2 WHERE (stuff.thing <> stuff2.otherthing) AND stuff.number <> stuff2.number;" > /var/lib/pgsql/select.sh
	PGPASSWORD=$PG_PASS pgbench -c 15 -T 900 -n -f /var/lib/pgsql/select.sh --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER"
	echo "busywork complete"
}



# ensure all params are passed before executing psql commands.
if [ -z "${PG_HOST}" ] | [ -z "${PG_PASS}" ]; then
  echo "host and password required and not set - exiting"
  exit 1
fi

drop_tables
create_tables
insert_data
select_data
