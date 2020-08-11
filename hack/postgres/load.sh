PG_HOST=$1
PG_PASS=$2
PG_SIZE=$3

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

calc_fill() {
  # 225000 entries required to fill 1GiB of storage
  ONE_GIB=300000
  PG_FILL=$(($PG_SIZE * $ONE_GIB))
}

# create `stuff` table if one does not exist
create_table() {
  echo "Creating Table in $PG_HOST"
  PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "CREATE TABLE IF NOT EXISTS stuff (thing varchar, otherthing varchar, anotherthing varchar, thirdthing varchar);"
}

# insert `calc_fill` entries into the table over a 2 hour period
insert_data() {
  calc_fill
  INSERT="insert into stuff (thing, otherthing, anotherthing, thirdthing) select md5(random()::text), md5(random()::text), md5(random()::text), md5(random()::text) from generate_series(1, $PG_FILL) s(i);"

  echo "Starting Load"
  i="0"
  while [ $i -lt 24 ]
  do
    echo "loading data iteration $i"
    PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "$INSERT"
    sleep 5m
    i=$[$i+1]
  done
}

# ensure all params are passed before executing psql commands.
if [ -z "${PG_HOST}" ] | [ -z "${PG_PASS}" ] | [ -z "${PG_SIZE}" ]; then
  echo "host, password, and entry count required and not set - exiting"
  exit 1
fi

create_table
insert_data
