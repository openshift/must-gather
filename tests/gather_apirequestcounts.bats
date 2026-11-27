#!/usr/bin/env bats
# Tests for collection-scripts/gather_apirequestcounts

load test_helper

@test "gather_apirequestcounts writes JSON and top20 files" {
	create_mock_oc "$(cat "$FIXTURES_DIR/oc_outputs/apirequestcounts.json")"
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_apirequestcounts
	"
	assert_success
	[ -s "$TEST_TMPDIR/must-gather/requests/apirequestscounts.json" ]
	[ -s "$TEST_TMPDIR/must-gather/requests/top20-resources-last24h" ]
	[ -s "$TEST_TMPDIR/must-gather/requests/top20-users-last24h" ]
	run cat "$TEST_TMPDIR/must-gather/requests/top20-resources-last24h"
	assert_output --partial "100: pods.core"
	assert_output --partial "50: flowschemas.v1beta3.flowcontrol.apiserver.k8s.io"
}

@test "gather_apirequestcounts JSON keeps removedInRelease for API-removal checks" {
	create_mock_oc "$(cat "$FIXTURES_DIR/oc_outputs/apirequestcounts.json")"
	run bash -c "
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		export BASE_COLLECTION_PATH=\"$TEST_TMPDIR/must-gather\"
		cd \"$SCRIPT_DIR\"
		/bin/bash gather_apirequestcounts
		jq -r '.items[] | select(.status.removedInRelease != \"\") | \"\(.metadata.name) \(.status.removedInRelease)\"' \"\$BASE_COLLECTION_PATH/requests/apirequestscounts.json\"
	"
	assert_success
	assert_output --partial "flowschemas.v1beta3.flowcontrol.apiserver.k8s.io 1.32"
}
