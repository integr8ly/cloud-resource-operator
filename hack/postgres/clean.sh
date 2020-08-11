PG_HOST=$1
PG_PASS=$2

if [ -z "${PG_HOST}" ]; then
  echo "Host not set - exiting"
  exit 1
fi

if [ -z "${PG_PASS}" ]; then
  echo "Password not set - exiting"
  exit 1
fi

PG_USER=postgres
PG_PORT=5432
PG_DB=postgres

PGPASSWORD=$PG_PASS psql --host=$PG_HOST --port=$PG_PORT --username=$PG_USER --dbname=$PG_DB -c 'DROP TABLE stuff'


