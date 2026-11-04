#!/usr/bin/env bash
# Export the reviewed tree; mutate only a disposable copy.
set -euo pipefail
source_repo=${1:?usage: reproduce.sh REPOSITORY chunked|setup-lifetime|mandatory-disabled|loopback-assertion|inspect-bindings}
mode=${2:?missing mode}
evidence_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
review_copy=$(mktemp -d "${TMPDIR:-/tmp}/deps-review-repro.XXXXXX")
trap 'rm -rf -- "$review_copy"' EXIT
git -C "$source_repo" archive ac8ab8e93c0eeef029da27d533593699db09c1bc | tar -x -C "$review_copy"
cd "$review_copy"
base64 --decode "$evidence_dir/$mode.patch.b64" > "$review_copy/review.patch"
git apply "$review_copy/review.patch"
case "$mode" in
  chunked) go test -race -short -count=1 -run TestChunkedJSONTrailingLimit ./httprequest ;;
  setup-lifetime) go test -race -short -count=1 -run TestSubscribeOutlivesConstructionContext ./messaging ;;
  mandatory-disabled) go test -v -race -tags integration -run 'TestRabbitMQIntegration/mandatory' -count=1 -timeout=3m ./messaging ;;
  loopback-assertion) go test -v -tags integration -run TestPostgresIntegration -count=1 -timeout=3m ./postgres ;;
  inspect-bindings) go test -v -tags integration -run Integration -count=1 -timeout=3m ./postgres ./mongo ./messaging ;;
  *) printf 'unknown mode: %s\n' "$mode" >&2; exit 2 ;;
esac
