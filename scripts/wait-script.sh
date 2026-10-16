#!/usr/bin/env bash
set -eu

if [[ ! ${WAIT_PORTS:-} =~ ^[0-9]+(,[0-9]+)*$ ]]; then
  echo 'WAIT_PORTS must be a comma-separated list of ports' >&2
  exit 1
fi

IFS=',' read -r -a ports <<< "$WAIT_PORTS"
for port in "${ports[@]}"; do
  if (( ${#port} > 5 )) || (( 10#$port < 1 || 10#$port > 65535 )); then
    echo "Invalid port: $port" >&2
    exit 1
  fi
done

script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
pids=()

cleanup() {
  trap - EXIT INT TERM
  for pid in "${pids[@]}"; do
    # Job control gives each waiter its own group, including wget and sleep.
    kill -TERM -- "-$pid" 2>/dev/null || true
  done
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
set -m

for port in "${ports[@]}"; do
  "$script_dir/wait-for.sh" -t 120 "http://localhost:$port/manage/health" \
    -- echo "Host localhost:$port is active" &
  pids+=("$!")
done

for pid in "${pids[@]}"; do
  wait "$pid"
  pids=("${pids[@]:1}")
done
