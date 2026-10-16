#!/bin/sh
# Check configuration selection without PostgreSQL or migration tools.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
mkdir "$workdir/bin"
cat > "$workdir/bin/yq" <<'MOCK'
#!/bin/sh
case "$2" in
    .database.url) printf '%s\n' "$3" >> "$SELECTION_LOG"; printf '%s\n' "$3" ;;
    .directory) echo migrations ;;
    .lock_timeout) echo 1s ;;
    .timeout) echo 5s ;;
esac
MOCK
cat > "$workdir/bin/atlas" <<'MOCK'
#!/bin/sh
printf 'atlas\n' >> "$EXECUTION_LOG"
MOCK
cat > "$workdir/bin/service" <<'MOCK'
#!/bin/sh
printf 'service\n' >> "$EXECUTION_LOG"
printf '%s\n' "$@" > "$ARGUMENT_LOG"
MOCK
chmod +x "$workdir/bin/"*
PATH="$workdir/bin:$PATH"
SELECTION_LOG="$workdir/selection"
EXECUTION_LOG="$workdir/execution"
ARGUMENT_LOG="$workdir/arguments"
export PATH SELECTION_LOG EXECUTION_LOG ARGUMENT_LOG
for service in reservation payment loyalty; do
    ln -s service "$workdir/bin/$service"
    entrypoint="$root/services/$service/docker-entrypoint.sh"
    for scenario in default separate equals later repeated terminator; do
        expected=/app/config.yaml
        case "$scenario" in
            default) set -- ;;
            separate) set -- --config '/custom config.yaml'; expected='/custom config.yaml' ;;
            equals) set -- --config=/custom.yaml; expected=/custom.yaml ;;
            later) set -- --help --config /later.yaml; expected=/later.yaml ;;
            repeated) set -- --config /first.yaml --config=/last.yaml; expected=/last.yaml ;;
            terminator) set -- -- --config=/ignored.yaml ;;
        esac
        : > "$SELECTION_LOG"
        : > "$EXECUTION_LOG"
        sh "$entrypoint" "$@"
        test "$(cat "$SELECTION_LOG")" = "$expected"
        test "$(cat "$EXECUTION_LOG")" = "$(printf 'atlas\nservice')"
        printf '%s\n' "$@" > "$workdir/expected-arguments"
        cmp "$workdir/expected-arguments" "$ARGUMENT_LOG"
    done
    for scenario in missing empty empty_equals; do
        case "$scenario" in
            missing) set -- --config ;;
            empty) set -- --config '' ;;
            empty_equals) set -- --config= ;;
        esac
        : > "$SELECTION_LOG"
        : > "$EXECUTION_LOG"
        if sh "$entrypoint" "$@" 2> "$workdir/error"; then
            echo "$service accepted $scenario configuration" >&2
            exit 1
        fi
        test ! -s "$SELECTION_LOG"
        test ! -s "$EXECUTION_LOG"
    done
done
echo 'Entrypoint configuration checks passed'
