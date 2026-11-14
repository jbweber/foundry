#!/usr/bin/env bash
# Runs the integration tests (build tag "integration") against the libvirt
# whose socket FOUNDRY_INTEGRATION_SOCKET names, and fails unless at least
# one of them passed. A lane whose tests were all skipped (the variable not
# set) or that found no test proved nothing, so it must not report success.
# Extra arguments go to go test.
set -euo pipefail

log=$(mktemp)
trap 'rm -f "$log"' EXIT

go test -tags integration -race -count=1 -v -run '^TestIntegration' "$@" ./... 2>&1 | tee "$log"

passed=$(grep -cE '^--- PASS: TestIntegration' "$log" || true)
skipped=$(grep -cE '^--- SKIP: TestIntegration' "$log" || true)
echo "integration tests: ${passed} passed, ${skipped} skipped"
if [ "$passed" -eq 0 ]; then
	echo "no integration test ran: the lane needs a libvirt (set FOUNDRY_INTEGRATION_SOCKET; skip reasons above)" >&2
	exit 1
fi
