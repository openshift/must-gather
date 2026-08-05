#!/usr/bin/env bats
# Tests for collection-scripts/gather_nmstate

load test_helper

# =============================================================================
# get_nmstate_crs() tests
# =============================================================================

@test "get_nmstate_crs inspects all NMState CRDs" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-nmstate")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate 2>&1
	"

	assert_success
	assert_output --partial "nmstates"
	assert_output --partial "nodenetworkconfigurationenactments"
	assert_output --partial "nodenetworkconfigurationpolicies"
	assert_output --partial "nodenetworkstates"
}

# =============================================================================
# Main script behavior tests
# =============================================================================

@test "gather_nmstate exits early when operator not found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_empty.txt\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate
	"

	assert_success
	assert_output --partial "INFO"
	assert_output --partial "not detected"
}

@test "gather_nmstate completes successfully when operator is found" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-nmstate")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate 2>&1
	"

	assert_success
	refute_output --partial "not detected"
}

@test "gather_nmstate uses correct namespace from subscription" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-nmstate")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	assert_output --partial "adm inspect"
	assert_output --partial "ns/openshift-nmstate"
}

@test "gather_nmstate calls sync at end" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-nmstate")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate 2>&1
	"

	assert_success
	assert_output --partial "MOCK sync called"
}

@test "gather_nmstate handles oc adm inspect failure gracefully" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-nmstate")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export MOCK_OC_INSPECT_EXIT_CODE=1
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_nmstate 2>&1
	"

	assert_output --partial "nmstates"
	assert_output --partial "nodenetworkstates"
}
