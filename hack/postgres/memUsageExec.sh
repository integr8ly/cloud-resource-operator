PG_HOST=$1
PG_PASS=$2

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

max_connections=25

do_intensive_query() {
  echo "running query"
  PGPASSWORD=$PG_PASS psql --host="$PG_HOST" --port="$PG_PORT" --username="$PG_USER" --dbname="$PG_DB" -a -c "EXPLAIN ANALYZE WITH cte_1g as (select repeat('a', 1024*1024*100 - 100) as a1g) SELECT * FROM cte_1g a"
  echo "query done"
}

while :
do

  job_list=($(jobs -p))
  job_count=${#job_list[@]}

  if [ $job_count -lt $max_connections ]
  then
    do_intensive_query &
    echo "Job Count: $job_count"
  fi
  
  sleep .1
done