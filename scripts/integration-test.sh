#!/bin/sh
# Run from a service directory; migration settings come from its YAML files.
set -eu

config=$1
shift
workdir=$(mktemp -d)
container=
cleanup() {
    status=$?
    trap - EXIT
    if [ -n "$container" ]; then
        if [ "$status" -ne 0 ]; then docker logs "$container" >&2 || true; fi
        docker rm -f -v "$container" >/dev/null || true
    fi
    rm -rf "$workdir"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

container=$(docker run -d --rm \
    -e POSTGRES_DB=test -e POSTGRES_USER=program -e POSTGRES_PASSWORD=test \
    -p 127.0.0.1::5432 postgres:13-alpine)
attempt=0
until docker exec "$container" pg_isready -h 127.0.0.1 -U program -d test >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 120 ]; then
        echo "PostgreSQL did not become ready within 120 seconds" >&2
        exit 1
    fi
    sleep 1
done
port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$container")
TEST_DATABASE_URL="postgres://program:test@127.0.0.1:$port/test?sslmode=disable"
export TEST_DATABASE_URL
"$YQ" '.database | .url = strenv(TEST_DATABASE_URL)' "$config" > "$workdir/database.yaml"
lock=$("$YQ" -er '.lock_timeout' "$MIGRATION_CONFIG")
duration=$("$YQ" -er '.timeout' "$MIGRATION_CONFIG")
directory=$("$YQ" -er '.directory' "$MIGRATION_CONFIG")
timeout "$duration" "$ATLAS" migrate apply \
    --dir "file://$directory" --url "$TEST_DATABASE_URL" --lock-timeout "$lock"
TEST_DATABASE_CONFIG="$workdir/database.yaml" "$@"
