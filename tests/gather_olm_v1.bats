#!/usr/bin/env bats
# Tests for collection-scripts/gather_olm_v1

load test_helper

# =============================================================================
# getOLMv1CRDs() / CRD discovery tests
# =============================================================================

@test "gather_olm_v1 exits early when no OLM v1 CRDs found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT=''
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
	"

	assert_success
	assert_output --partial "INFO: OLM v1 CRDs not detected"
}

@test "gather_olm_v1 filters CRDs by olm.operatorframework.io suffix" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT='clusterextensions.olm.operatorframework.io
clustercatalogs.olm.operatorframework.io
unrelated.example.com
operators.coreos.com'
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
	"

	assert_success
	# Should collect the two olm.operatorframework.io CRDs
	assert_output --partial "Collecting clusterextensions.olm.operatorframework.io"
	assert_output --partial "Collecting clustercatalogs.olm.operatorframework.io"
	# Should NOT collect unrelated CRDs
	refute_output --partial "Collecting unrelated.example.com"
	refute_output --partial "Collecting operators.coreos.com"
}

@test "gather_olm_v1 inspects each discovered CRD" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT='clusterextensions.olm.operatorframework.io
clustercatalogs.olm.operatorframework.io'
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	assert_output --partial "adm inspect"
	assert_output --partial "clusterextensions.olm.operatorframework.io"
	assert_output --partial "clustercatalogs.olm.operatorframework.io"
	assert_output --partial "--all-namespaces"
}

# =============================================================================
# Error handling tests
# =============================================================================

@test "gather_olm_v1 handles inspect failure per CRD with warning" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT='clusterextensions.olm.operatorframework.io
clustercatalogs.olm.operatorframework.io'
		export MOCK_OC_INSPECT_EXIT_CODE=1
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
	"

	assert_success
	# Script has || { echo "WARNING: Failed..." } for each CRD
	assert_output --partial "WARNING: Failed to collect clusterextensions.olm.operatorframework.io"
	assert_output --partial "WARNING: Failed to collect clustercatalogs.olm.operatorframework.io"
	# Should still complete
	assert_output --partial "OLM v1 resource collection complete"
}

# =============================================================================
# Completion tests
# =============================================================================

@test "gather_olm_v1 prints completion message and calls sync" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT='clusterextensions.olm.operatorframework.io'
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
	"

	assert_success
	assert_output --partial "OLM v1 resource collection complete"
	assert_output --partial "MOCK sync called"
}

@test "gather_olm_v1 does not call sync when no CRDs found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_CRDS_OUTPUT=''
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_olm_v1 2>&1
	"

	assert_success
	assert_output --partial "OLM v1 CRDs not detected"
	refute_output --partial "MOCK sync called"
}
