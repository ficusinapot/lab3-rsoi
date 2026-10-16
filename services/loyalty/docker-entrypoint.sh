#!/bin/sh
set -eu

config=$(
    selected=/app/config.yaml
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --) break ;;
            --config)
                if [ "$#" -lt 2 ] || [ -z "$2" ]; then
                    echo '--config requires a YAML path' >&2
                    exit 1
                fi
                selected=$2
                shift
                ;;
            --config=*)
                selected=${1#--config=}
                if [ -z "$selected" ]; then
                    echo '--config requires a YAML path' >&2
                    exit 1
                fi
                ;;
        esac
        shift
    done
    printf '%s\n' "$selected"
)
dsn=$(yq -er '.database.url' "$config")
directory=$(yq -er '.directory' /app/migrations.yaml)
lock=$(yq -er '.lock_timeout' /app/migrations.yaml)
duration=$(yq -er '.timeout' /app/migrations.yaml)
timeout "$duration" atlas migrate apply --dir "file://$directory" --url "$dsn" --lock-timeout "$lock"
exec loyalty "$@"
