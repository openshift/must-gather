#!/bin/bash
# Common test helper for BATS tests

# Project root directory (consistent with hack/tools.sh)
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export PROJECT_ROOT

# Validate bats libraries exist before loading
for lib in bats-support bats-assert; do
	if [[ ! -d "${PROJECT_ROOT}/tmp/bin/${lib}" ]]; then
		echo "ERROR: ${lib} not found. Run: ./hack/tools.sh bats" >&2
		exit 1
	fi
done

# Directory containing the scripts under test
SCRIPT_DIR="${PROJECT_ROOT}/collection-scripts"
export SCRIPT_DIR

# Directory containing test fixtures
FIXTURES_DIR="${PROJECT_ROOT}/tests/fixtures"
export FIXTURES_DIR

# Load bats helper libraries
load "${PROJECT_ROOT}/tmp/bin/bats-support/load"
load "${PROJECT_ROOT}/tmp/bin/bats-assert/load"

# Common setup for all tests
setup() {
	TEST_TMPDIR="$(mktemp -d)"
	export TEST_TMPDIR

	# Inline mocks take precedence over shared mocks
	export PATH="$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:$PATH"

	export BASE_COLLECTION_PATH="$TEST_TMPDIR/must-gather"
	mkdir -p "$BASE_COLLECTION_PATH"

	unset MOCK_OC_OUTPUT
	unset MOCK_OC_EXIT_CODE
	unset MOCK_OC_PODS_OUTPUT
}

# Common teardown for all tests
teardown() {
	# Clean up temp directory
	if [[ -n "${TEST_TMPDIR:-}" && -d "$TEST_TMPDIR" ]]; then
		rm -rf "$TEST_TMPDIR"
	fi
}

# Helper function to create a fixture file dynamically
create_fixture() {
	local name="$1"
	local content="$2"
	local fixture_file="$TEST_TMPDIR/$name"
	mkdir -p "$(dirname "$fixture_file")"
	echo "$content" >"$fixture_file"
	echo "$fixture_file"
}

# Helper function to get fixture path
# Usage: export MOCK_OC_OUTPUT="$(fixture subs_single.txt)"
fixture() {
	local name="$1"
	echo "${FIXTURES_DIR}/oc_outputs/${name}"
}

# Create an inline mock oc that returns the given output.
# Placed at $TEST_TMPDIR/mocks/oc which takes precedence over
# the shared mock via PATH ordering set up in setup().
#
# Usage:
#   create_mock_oc "openshift-metallb"       # exits 0
#   create_mock_oc "" 1                      # empty output, exits 1
create_mock_oc() {
	local output="${1:-}"
	local exit_code="${2:-0}"
	mkdir -p "$TEST_TMPDIR/mocks"
	printf '%s' "$output" >"$TEST_TMPDIR/mocks/oc_output"
	cat >"$TEST_TMPDIR/mocks/oc" <<EOF
#!/bin/bash
cat "$TEST_TMPDIR/mocks/oc_output"
exit $exit_code
EOF
	chmod +x "$TEST_TMPDIR/mocks/oc"
}

# Helper function to run commands with mocks and strict mode disabled.
# This reduces boilerplate in tests that need to source scripts with strict mode.
#
# Usage:
#   run_with_mocks "source \"$SCRIPT_DIR/monitoring_common.sh\" && get_first_ready_prom_pod"
#
run_with_mocks() {
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
		set +o nounset
		set +o errexit
		set +o pipefail
		$1
	"
}

# Helper function to source scripts that use strict mode (set -o nounset, errexit, pipefail)
# This disables strict mode before sourcing to prevent test failures from unset variables.
#
# NOTE: This function is for use in setup() or direct test code, NOT inside 'run bash -c'.
# For tests using 'run bash -c "..."', use run_with_mocks() instead.
#
# Usage: source_script_safely "$SCRIPT_DIR/monitoring_common.sh"
source_script_safely() {
	local script_path="$1"
	set +o nounset
	set +o errexit
	set +o pipefail
	# shellcheck disable=SC1090
	source "$script_path"
}

# Helper function to check if a specific oc command was called with expected arguments
# The mock oc logs calls to MOCK_OC_LOG_FILE when that variable is set.
#
# Usage in test (inside 'run bash -c'):
#   export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
#   ... run script ...
#   grep 'adm inspect.*bgppeers' \"$TEST_TMPDIR/oc_calls.log\"
#
# Or use this helper after 'run':
#   assert_oc_called "adm inspect" "bgppeers"
#
assert_oc_called() {
	local log_file="${MOCK_OC_LOG_FILE:-$TEST_TMPDIR/oc_calls.log}"
	local pattern="$*"

	if [[ ! -f "$log_file" ]]; then
		echo "Mock oc log file not found: $log_file" >&2
		return 1
	fi

	if grep -q "$pattern" "$log_file"; then
		return 0
	else
		echo "Expected oc call matching '$pattern' not found in log:" >&2
		cat "$log_file" >&2
		return 1
	fi
}
