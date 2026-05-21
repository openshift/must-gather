#!/usr/bin/env bats
# Tests for collection-scripts/gather_aro
#
# NOTE: gather_aro does NOT source common.sh -- it is fully standalone.
# Detection uses: oc get clusters.aro.openshift.io --ignore-not-found=true

load test_helper

# =============================================================================
# Early-exit path (not ARO)
# =============================================================================

@test "gather_aro exits early when ARO CR is not found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_ARO_OUTPUT=''
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_aro 2>&1
	"

	assert_success
	refute_output --partial "Collecting ARO Cluster Data"
	refute_output --partial "MOCK sync called"
}

# =============================================================================
# Happy path (ARO detected)
# =============================================================================

@test "gather_aro collects data when ARO CR is found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_ARO_OUTPUT='cluster'
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_aro 2>&1
	"

	assert_success
	assert_output --partial "INFO: Collecting ARO Cluster Data"
}

@test "gather_aro inspects all expected targets" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_ARO_OUTPUT='cluster'
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_aro 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	# Verify all three inspect targets from the script
	assert_output --partial "clusters.aro.openshift.io"
	assert_output --partial "ns/openshift-azure-operator"
	assert_output --partial "ns/openshift-azure-logging"
}

@test "gather_aro calls sync at end" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_ARO_OUTPUT='cluster'
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_aro 2>&1
	"

	assert_success
	assert_output --partial "MOCK sync called"
}

@test "gather_aro handles oc adm inspect failure gracefully" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_ARO_OUTPUT='cluster'
		export MOCK_OC_INSPECT_EXIT_CODE=1
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_aro 2>&1
	"

	assert_output --partial "Collecting ARO Cluster Data"
	assert_output --partial "clusters.aro.openshift.io"
	assert_output --partial "ns/openshift-azure-operator"
}
