#!/usr/bin/env bats
# Tests for collection-scripts/gather_ingress_node_firewall

load test_helper

# =============================================================================
# CRD inspection tests
# =============================================================================

@test "gather_ingress_node_firewall inspects all CRDs" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
	"

	assert_success
	# Namespaced CRDs
	assert_output --partial "ingressnodefirewallconfigs"
	assert_output --partial "ingressnodefirewallnodestates"
	# Cluster-scoped CRDs
	assert_output --partial "ingressnodefirewalls"
}

@test "namespaced CRDs are inspected with namespace flag" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	# Namespaced CRDs should be inspected with -n <namespace>
	assert_output --partial "-n openshift-ingress-node-firewall"
	assert_output --partial "ingressnodefirewallconfigs"
}

# =============================================================================
# Main script behavior tests
# =============================================================================

@test "gather_ingress_node_firewall exits early when operator not found" {
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$FIXTURES_DIR/oc_outputs/subs_empty.txt\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall
	"

	assert_success
	assert_output --partial "INFO"
	assert_output --partial "not detected"
}

@test "gather_ingress_node_firewall completes successfully when operator is found" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
	"

	assert_success
	refute_output --partial "not detected"
}

@test "gather_ingress_node_firewall inspects operator namespace" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
		echo '---OC CALLS LOG---'
		cat \"$TEST_TMPDIR/oc_calls.log\"
	"

	assert_success
	assert_output --partial "ns/openshift-ingress-node-firewall"
}

@test "gather_ingress_node_firewall calls sync at end" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
	"

	assert_success
	assert_output --partial "MOCK sync called"
}

@test "gather_ingress_node_firewall handles oc adm inspect failure gracefully" {
	local ns_fixture
	ns_fixture=$(create_fixture "subs.txt" "openshift-ingress-node-firewall")
	run bash -c "
		export PATH=\"$BATS_TEST_DIRNAME/mocks:\$PATH\"
		export MOCK_OC_OUTPUT=\"$ns_fixture\"
		export MOCK_OC_INSPECT_EXIT_CODE=1
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_ingress_node_firewall 2>&1
	"

	assert_output --partial "ingressnodefirewallconfigs"
	assert_output --partial "ingressnodefirewalls"
}
