#!/usr/bin/env bats
# Tests for collection-scripts/gather_vsphere
#
# NOTE: gather_vsphere uses /usr/bin/oc (absolute path) for the CSI driver
# detection call on line 8. This means PATH-based mocking cannot intercept the
# detection. On CI/test machines where /usr/bin/oc does not exist, the detection
# naturally fails and the script exits 0 (early-exit path).
#
# The happy path (vSphere detected -> inspect CRDs) cannot be tested via UT
# without modifying the script to use plain `oc` instead of `/usr/bin/oc`.
# That path requires e2e testing on a real vSphere cluster.

load test_helper

# =============================================================================
# Early-exit path (not vSphere)
# =============================================================================

@test "gather_vsphere exits early when CSI driver is not found" {
	# When /usr/bin/oc doesn't exist or fails, CSIDRIVER is empty and script exits 0
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_vsphere 2>&1
	"

	assert_success
	# Should NOT reach the CRD collection phase or sync
	refute_output --partial "Collecting vSphere CSI driver CRDs"
	refute_output --partial "MOCK sync called"
}
