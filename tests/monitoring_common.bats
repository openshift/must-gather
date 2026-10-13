#!/usr/bin/env bats
# Tests for collection-scripts/monitoring_common.sh

load test_helper

# =============================================================================
# get_first_ready_prom_pod() tests
# =============================================================================

@test "get_first_ready_prom_pod returns first pod name when pods exist" {
	create_mock_oc "$(printf 'prometheus-k8s-0\nprometheus-k8s-1')"
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		get_first_ready_prom_pod
	"

	assert_success
	assert_output --partial "prometheus-k8s-0"
}

@test "get_first_ready_prom_pod returns empty when no pods exist" {
	create_mock_oc ""
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		set +o nounset
		result=\$(get_first_ready_prom_pod)
		echo \"result=[\$result]\"
	"

	assert_success
	assert_output --partial "result=[]"
}

# =============================================================================
# get_first_ready_alertmanager_pod() tests
# =============================================================================

@test "get_first_ready_alertmanager_pod returns first pod name when pods exist" {
	create_mock_oc "$(printf 'alertmanager-main-0\nalertmanager-main-1')"
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		get_first_ready_alertmanager_pod
	"

	assert_success
	assert_output --partial "alertmanager-main-0"
}

@test "get_first_ready_alertmanager_pod returns empty when no pods exist" {
	create_mock_oc ""
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		set +o nounset
		result=\$(get_first_ready_alertmanager_pod)
		echo \"result=[\$result]\"
	"

	assert_success
	assert_output --partial "result=[]"
}

# =============================================================================
# metrics_gather() tests
# =============================================================================

@test "metrics_gather exits with error when no arguments provided" {
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather
	"

	assert_failure
	assert_output --partial "ERROR"
	assert_output --partial "Not setting any arguments"
}

@test "metrics_gather succeeds when arguments are provided" {
	mkdir -p "$TEST_TMPDIR/metrics"
	run_with_mocks "
		export METRICS_PATH=\"$TEST_TMPDIR/metrics\"
		export MOCK_OC_PODS_OUTPUT='prometheus-k8s-0'
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather --min-time=123456789 2>&1
		# The oc exec output goes to files, so check the stderr log file for the mock call
		echo '---MOCK STDERR LOG---'
		cat \"$TEST_TMPDIR/metrics/metrics.stderr\" 2>/dev/null || echo 'no stderr file'
	"

	assert_success
	# Verify that metrics_get was called and printed INFO message
	assert_output --partial "INFO: Getting metrics from prometheus-k8s-0"
	# The mock oc logs to stderr, which gets redirected to metrics.stderr file
	assert_output --partial "MOCK oc called with: exec prometheus-k8s-0"
}

@test "metrics_gather passes arguments to metrics_get" {
	mkdir -p "$TEST_TMPDIR/metrics"
	run_with_mocks "
		export METRICS_PATH=\"$TEST_TMPDIR/metrics\"
		export MOCK_OC_PODS_OUTPUT='prometheus-k8s-0'
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather --min-time=123456789 --max-time=999999999 2>&1
		# Check the stderr log file for mock call details
		echo '---MOCK STDERR LOG---'
		cat \"$TEST_TMPDIR/metrics/metrics.stderr\" 2>/dev/null || echo 'no stderr file'
	"

	assert_success
	# The mock logs to stderr which goes to metrics.stderr file
	assert_output --partial "MOCK oc called with: exec prometheus-k8s-0"
	assert_output --partial "--min-time=123456789"
	assert_output --partial "--max-time=999999999"
}

# =============================================================================
# Error case tests
# =============================================================================

@test "metrics_gather handles missing prometheus pod gracefully" {
	mkdir -p "$TEST_TMPDIR/metrics"
	run_with_mocks "
		export METRICS_PATH=\"$TEST_TMPDIR/metrics\"
		export MOCK_OC_PODS_OUTPUT=''
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather --min-time=123456789 2>&1
	"

	# metrics_gather has '|| true' after metrics_get, so it should still succeed
	assert_success
	# Verify it attempted to get metrics (even with empty pod name)
	assert_output --partial "INFO: Getting metrics from"
}

@test "metrics_gather creates METRICS_PATH directory if it does not exist" {
	run_with_mocks "
		export METRICS_PATH=\"$TEST_TMPDIR/new_metrics_dir\"
		export MOCK_OC_PODS_OUTPUT='prometheus-k8s-0'
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather --min-time=123456789 2>&1
		# Verify directory was created
		if [[ -d \"$TEST_TMPDIR/new_metrics_dir\" ]]; then
			echo 'METRICS_PATH directory created'
		fi
	"

	assert_success
	assert_output --partial "METRICS_PATH directory created"
}

@test "metrics_gather error message includes required argument examples" {
	run_with_mocks "
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather 2>&1
	"

	assert_failure
	# Verify the error message includes helpful information about required arguments
	assert_output --partial "--min-time"
	assert_output --partial "--max-time"
	assert_output --partial "--match"
}

@test "metrics_gather calls sync after collecting metrics" {
	run_with_mocks "
		export METRICS_PATH=\"$TEST_TMPDIR/metrics\"
		export MOCK_OC_PODS_OUTPUT='prometheus-k8s-0'
		source \"$SCRIPT_DIR/monitoring_common.sh\"
		metrics_gather --min-time=123456789 2>&1
	"

	assert_success
	assert_output --partial "MOCK sync called"
}
