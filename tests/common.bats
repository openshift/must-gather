#!/usr/bin/env bats
# Tests for collection-scripts/common.sh

load test_helper

# =============================================================================
# Sanity checks
# =============================================================================

@test "mock oc binary is resolved from PATH" {
	run command -v oc
	assert_success
	assert_output --partial "mocks/oc"
}

# =============================================================================
# get_operator_ns() tests
# =============================================================================

@test "get_operator_ns sets operator_ns when operator exists with single subscription" {
	create_mock_oc "openshift-metallb"
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_operator_ns 'metallb-operator'
		echo \"\$operator_ns\"
	"

	assert_success
	assert_output --partial "openshift-metallb"
}

@test "get_operator_ns exits 0 with INFO message when operator not found" {
	create_mock_oc ""
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_operator_ns 'nonexistent-operator'
	"

	assert_success
	assert_output --partial "INFO"
	assert_output --partial "not detected"
}

@test "get_operator_ns exits 1 with ERROR message when multiple subscriptions found" {
	create_mock_oc "$(printf 'openshift-metallb\nopenshift-metallb-2')"
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_operator_ns 'duplicated-operator'
	"

	assert_failure
	assert_output --partial "ERROR"
	assert_output --partial "more than one"
}

# =============================================================================
# get_log_collection_args() tests
# =============================================================================

@test "get_log_collection_args sets log_collection_args from MUST_GATHER_SINCE" {
	run bash -c "
		export MUST_GATHER_SINCE='8h'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"log_collection_args=\$log_collection_args\"
	"

	assert_success
	assert_output --partial "log_collection_args=--since=8h"
}

@test "get_log_collection_args sets log_collection_args from MUST_GATHER_SINCE_TIME" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2024-01-15T10:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"log_collection_args=\$log_collection_args\"
	"

	assert_success
	assert_output --partial "log_collection_args=--since-time=2024-01-15T10:00:00Z"
}

@test "get_log_collection_args leaves log_collection_args empty when neither env var set" {
	run bash -c "
		unset MUST_GATHER_SINCE
		unset MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"log_collection_args=[\$log_collection_args]\"
	"

	assert_success
	assert_output --partial "log_collection_args=[]"
}

@test "get_log_collection_args formats node_log_collection_args from MUST_GATHER_SINCE" {
	run bash -c "
		export MUST_GATHER_SINCE='8h'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"node_log_collection_args=\$node_log_collection_args\"
	"

	assert_success
	assert_output --partial "node_log_collection_args=--since=-8h"
}

@test "get_log_collection_args formats node_log_collection_args from MUST_GATHER_SINCE_TIME" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2024-01-15T10:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"node_log_collection_args=\$node_log_collection_args\"
	"

	assert_success
	# The function converts 'T' to space and removes 'Z'
	assert_output --partial "node_log_collection_args=--since=2024-01-15 10:00:00"
}

@test "get_log_collection_args MUST_GATHER_SINCE_TIME overrides MUST_GATHER_SINCE for log_collection_args" {
	run bash -c "
		export MUST_GATHER_SINCE='8h'
		export MUST_GATHER_SINCE_TIME='2024-01-15T10:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"log_collection_args=\$log_collection_args\"
	"

	assert_success
	# MUST_GATHER_SINCE_TIME should override MUST_GATHER_SINCE
	assert_output --partial "log_collection_args=--since-time=2024-01-15T10:00:00Z"
}

@test "get_log_collection_args MUST_GATHER_SINCE_TIME overrides MUST_GATHER_SINCE for node_log_collection_args" {
	run bash -c "
		export MUST_GATHER_SINCE='8h'
		export MUST_GATHER_SINCE_TIME='2024-01-15T10:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"node_log_collection_args=\$node_log_collection_args\"
	"

	assert_success
	# MUST_GATHER_SINCE_TIME should override MUST_GATHER_SINCE for node logs too
	assert_output --partial "node_log_collection_args=--since=2024-01-15 10:00:00"
}

# =============================================================================
# Error case tests
# =============================================================================

@test "get_operator_ns handles oc command failure gracefully" {
	create_mock_oc "" 1
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_operator_ns 'test-operator'
	"

	# When oc fails and returns empty output, it should exit 0 with INFO message
	assert_success
	assert_output --partial "INFO"
	assert_output --partial "not detected"
}

@test "get_log_collection_args handles complex MUST_GATHER_SINCE formats" {
	run bash -c "
		export MUST_GATHER_SINCE='2h30m'
		source \"$SCRIPT_DIR/common.sh\"
		get_log_collection_args
		echo \"log_collection_args=\$log_collection_args\"
		echo \"node_log_collection_args=\$node_log_collection_args\"
	"

	assert_success
	assert_output --partial "log_collection_args=--since=2h30m"
	# node_log_collection_args extracts only the first time unit
	assert_output --partial "node_log_collection_args=--since=-2h"
}
