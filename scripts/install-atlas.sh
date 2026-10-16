#!/usr/bin/env bash
set -euo pipefail

# The same immutable Atlas build is used by CI and service entrypoints.
image='arigaio/atlas@sha256:1cba0a901ca5db8bd402c68402607ed47f8eda894d6f865579f3ebaf3b015696'
container=$(docker create "$image")
workdir=$(mktemp -d)
trap 'docker rm "$container" >/dev/null; rm -rf "$workdir"' EXIT
docker cp "$container:/atlas" "$workdir/atlas"
sudo install -m 755 "$workdir/atlas" /usr/local/bin/atlas
atlas version
