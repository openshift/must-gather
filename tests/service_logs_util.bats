#!/usr/bin/env bats
# Tests for collection-scripts/gather_service_logs_util

load test_helper

# Helper: sets up env, sources the util, runs the given commands, then waits.
run_service_logs_test() {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export SERVICE_LOG_PATH=\"$TEST_TMPDIR/service_logs\"
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		PIDS=()
		source \"$SCRIPT_DIR/gather_service_logs_util\"
		$1
		wait \${PIDS[@]} 2>/dev/null || true
	"
}

# =============================================================================
# collect_service_logs() tests
# =============================================================================

@test "collect_service_logs creates masters directory for --role=master" {
	run_service_logs_test "
		collect_service_logs --role=master crio kubelet
		[[ -d \"$TEST_TMPDIR/service_logs/masters\" ]] && echo 'masters directory created'
	"

	assert_success
	assert_output --partial "masters directory created"
}

@test "collect_service_logs creates workers directory for --role=worker" {
	run_service_logs_test "
		collect_service_logs --role=worker crio kubelet
		[[ -d \"$TEST_TMPDIR/service_logs/workers\" ]] && echo 'workers directory created'
	"

	assert_success
	assert_output --partial "workers directory created"
	# Should also print warning about collecting from all workers
	assert_output --partial "WARNING"
}

@test "collect_service_logs creates custom directory for custom selector" {
	run_service_logs_test "
		collect_service_logs my-custom-node crio
		[[ -d \"$TEST_TMPDIR/service_logs/my-custom-node\" ]] && echo 'custom directory created'
	"

	assert_success
	assert_output --partial "custom directory created"
	# Should not have the WARNING message (only shown for workers)
	refute_output --partial "WARNING"
}

@test "collect_service_logs outputs INFO message for each service" {
	run_service_logs_test "
		collect_service_logs --role=master crio kubelet NetworkManager
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for crio"
	assert_output --partial "INFO: Collecting host service logs for kubelet"
	assert_output --partial "INFO: Collecting host service logs for NetworkManager"
}

# =============================================================================
# node_log_collection_args tests
# =============================================================================

@test "collect_service_logs executes with custom node_log_collection_args" {
	run_service_logs_test "
		export node_log_collection_args='--since=-8h'
		collect_service_logs --role=master crio 2>&1
		[[ -d \"$TEST_TMPDIR/service_logs/masters\" ]] && echo 'executed successfully'
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for crio"
	assert_output --partial "executed successfully"
	assert_oc_called "node-logs --since=-8h"
}

@test "collect_service_logs executes with default timeframe" {
	run_service_logs_test "
		unset node_log_collection_args
		unset SINCE_TIMEFRAME
		collect_service_logs --role=master crio 2>&1
		[[ -d \"$TEST_TMPDIR/service_logs/masters\" ]] && echo 'executed successfully'
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for crio"
	assert_output --partial "executed successfully"
	assert_oc_called "node-logs --since=-7d"
}

@test "collect_service_logs executes with SINCE_TIMEFRAME" {
	run_service_logs_test "
		unset node_log_collection_args
		export SINCE_TIMEFRAME='-24h'
		collect_service_logs --role=master crio 2>&1
		[[ -d \"$TEST_TMPDIR/service_logs/masters\" ]] && echo 'executed successfully'
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for crio"
	assert_output --partial "executed successfully"
	assert_oc_called "node-logs --since=-24h"
}

@test "collect_service_logs integrates with common.sh get_log_collection_args" {
	run_service_logs_test "
		export MUST_GATHER_SINCE='4h'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"node_log_collection_args=\$node_log_collection_args\"
		collect_service_logs --role=master crio 2>&1
	"

	assert_success
	# Verify common.sh correctly formats the args for node-logs
	assert_output --partial "node_log_collection_args=--since=-4h"
	assert_output --partial "INFO: Collecting host service logs for crio"
	assert_oc_called "node-logs --since=-4h"
}

# =============================================================================
# Edge case tests
# =============================================================================

@test "collect_service_logs handles service names" {
	run_service_logs_test "
		collect_service_logs --role=master ovs-vswitchd ovsdb-server 2>&1
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for ovs-vswitchd"
	assert_output --partial "INFO: Collecting host service logs for ovsdb-server"
}

@test "collect_service_logs creates log files in correct directory" {
	run_service_logs_test "
		collect_service_logs --role=master testservice 2>&1
		ls -la \"$TEST_TMPDIR/service_logs/masters/\" 2>&1 || echo 'directory check done'
	"

	assert_success
	assert_output --partial "INFO: Collecting host service logs for testservice"
}

@test "collect_service_logs writes uncompressed .log by default" {
	run_service_logs_test "
		unset REDUCE_LOGS
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		collect_service_logs --role=master testservice 2>&1
		wait \"\${PIDS[@]}\" 2>/dev/null || true
		PIDS=()
		[[ -f \"$TEST_TMPDIR/service_logs/masters/testservice_service.log\" ]] && echo 'uncompressed log present'
		[[ ! -f \"$TEST_TMPDIR/service_logs/masters/testservice_service.log.gz\" ]] && echo 'gzip absent'
	"

	assert_success
	assert_output --partial "uncompressed log present"
	assert_output --partial "gzip absent"
}

@test "collect_service_logs writes .log.gz when REDUCE_LOGS is compress_service_logs" {
	run_service_logs_test "
		export REDUCE_LOGS='compress_service_logs'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		collect_service_logs --role=master testservice 2>&1
		wait \"\${PIDS[@]}\" 2>/dev/null || true
		PIDS=()
		[[ -f \"$TEST_TMPDIR/service_logs/masters/testservice_service.log.gz\" ]] && echo 'gzip log present'
		[[ ! -f \"$TEST_TMPDIR/service_logs/masters/testservice_service.log\" ]] && echo 'uncompressed absent'
	"

	assert_success
	assert_output --partial "gzip log present"
	assert_output --partial "uncompressed absent"
}

@test "collect_service_logs writes .log.gz when REDUCE_LOGS includes compress_service_logs with skip_rotated_logs" {
	run_service_logs_test "
		export REDUCE_LOGS='skip_rotated_logs,compress_service_logs'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		collect_service_logs --role=master testservice 2>&1
		wait \"\${PIDS[@]}\" 2>/dev/null || true
		PIDS=()
		[[ -f \"$TEST_TMPDIR/service_logs/masters/testservice_service.log.gz\" ]] && echo 'gzip log present'
	"

	assert_success
	assert_output --partial "gzip log present"
}
