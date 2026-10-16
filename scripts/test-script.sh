#!/usr/bin/env bash
set -euo pipefail

variant=${1:-v2}
service=${2:-loyalty}
port=${3:-8050}
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir/.."
environment=$(mktemp)
cp "${4:-$variant/postman/environment.json}" "$environment"
cleanup() {
  result=$?
  if (( result != 0 )); then docker compose logs --no-color; fi
  docker compose start "$service" >/dev/null || true
  rm -f "$environment"
  exit "$result"
}
trap cleanup EXIT

run_folder() {
  newman run --delay-request=100 --folder="$1" \
    --export-environment "$environment" --environment "$environment" \
    "$variant/postman/collection.json"
}

wait_counter() {
  # The collection records the count before the reservation in step2.
  # step3 cancels it, so step4 must eventually observe the same value.
  python3 - "$environment" <<'PY'
import json, sys, time, urllib.request, urllib.error
with open(sys.argv[1]) as source:
    values = {item['key']: item['value'] for item in json.load(source)['values']}
expected = int(values['reservationCount'])
request = urllib.request.Request(values['baseUrl'] + '/api/v1/loyalty',
                                 headers={'X-User-Name': values['username']})
deadline = time.monotonic() + 30
while time.monotonic() < deadline:
    try:
        with urllib.request.urlopen(request, timeout=min(2, deadline - time.monotonic())) as response:
            if json.load(response)['reservationCount'] == expected:
                break
    except (urllib.error.URLError, KeyError, ValueError, TimeoutError):
        pass
    time.sleep(min(0.25, max(0, deadline - time.monotonic())))
else:
    raise SystemExit('Loyalty counter did not converge within 30s')
PY
}

run_folder success
for step in 1 2 3 4; do
  if (( step % 2 == 1 )); then
    docker compose stop "$service"
  else
    docker compose start "$service"
    "$script_dir/wait-for.sh" -t 120 "http://localhost:$port/manage/health"
  fi
  if (( step == 4 )); then wait_counter; fi
  run_folder "step$step"
done
