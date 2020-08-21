PG_HOST=$1
PG_PASS=$2

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

hash1(){
echo "trying first hash option"
PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "EXPLAIN (ANALYZE, TIMING OFF) SELECT * FROM t WHERE a = 1;"
}

hash2() {
  echo "attempting to lower freaable memory"
	PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "EXPLAIN (ANALYZE, TIMING OFF) SELECT b, count(*) from (table t union all table t) u group by 1;"
}

for i in {1..3}
do
  echo $i
  hash2 &
done