#!/usr/bin/env bash
set -euo pipefail

# Compare against the working sources so this also works before the first commit.
snapshot=$(mktemp -d)
trap 'rm -rf "$snapshot"' EXIT
snapshot_generated() {
  local destination=$1
  local paths=(services/reservation/ent services/payment/ent services/loyalty/ent
    services/gateway/internal/rest/bookings/api/generated.go
    services/gateway/internal/clients/contracts/generated.go)
  while IFS= read -r path; do paths+=("$path"); done < <(rg --files services -g '*_mock_test.go')
  for path in "${paths[@]}"; do
    mkdir -p "$destination/$(dirname "$path")"
    cp -a "$path" "$destination/$path"
  done
}
snapshot_generated "$snapshot/before"
for service in reservation payment loyalty gateway; do
  make -C "services/$service" generate
done
snapshot_generated "$snapshot/after"
diff -ru "$snapshot/before" "$snapshot/after"
