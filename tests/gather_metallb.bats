#!/usr/bin/env bats
# Tests for collection-scripts/gather_metallb

load test_helper

# =============================================================================
# get_metallb_crs() tests
# =============================================================================

@test "get_metallb_crs inspects all MetalLB CRDs" {
	# Run the actual script and verify all CRDs are inspected
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=0
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb 2>&1
	"

	assert_success
	# Check that oc adm inspect was called for each CRD defined in the actual script
	assert_output --partial "bgppeers"
	assert_output --partial "bfdprofiles"
	assert_output --partial "bgpAdvertisements"
	assert_output --partial "ipaddresspools"
	assert_output --partial "l2advertisements"
	assert_output --partial "communities"
}

# =============================================================================
# Main script behavior tests
# =============================================================================

@test "gather_metallb exits early when operator not found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_empty.txt\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb
	"

	assert_success
	assert_output --partial "INFO"
	assert_output --partial "not detected"
}

@test "gather_metallb exits with message when metallb not started" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=1
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb
	"

	assert_success
	assert_output --partial "metallb not started"
}

@test "gather_metallb completes successfully when metallb is running" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=0
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb
	"

	assert_success
	# Should not have the "not started" message
	refute_output --partial "metallb not started"
}

@test "gather_metallb uses correct namespace from subscription" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=0
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	# Verify that oc adm inspect was called with the correct namespace (openshift-metallb)
	assert_output --partial "adm inspect"
	assert_output --partial "-n openshift-metallb"
}

@test "gather_metallb calls sync at end when metallb is running" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=0
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb 2>&1
	"

	assert_success
	assert_output --partial "MOCK sync called"
}

@test "gather_metallb handles oc adm inspect failure gracefully" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_single.txt\"
		export MOCK_METALLB_EXISTS=0
		export MOCK_OC_INSPECT_EXIT_CODE=1
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_metallb 2>&1
	"

	assert_output --partial "bgppeers"
	assert_output --partial "ipaddresspools"
}
