#!/usr/bin/env bats
# Test commands expand in child shells; BATS isolates each test's environment.
# shellcheck disable=SC2016,SC2030,SC2031
load test_helper

run_reporting() {
	run bash -euo pipefail -c 'source "$SCRIPT_DIR/status_reporting.sh"; status_init "$BASE_COLLECTION_PATH" test-version; eval "$1"' test "$1"
}

assert_report() {
	run jq -e "$1" "$BASE_COLLECTION_PATH/status.json"
	assert_success
}

prepare_gather() {
	export GATHER_BIN="$TEST_TMPDIR/bin" MOCK_STATE="$TEST_TMPDIR/state" OS_GIT_VERSION=test-image-version
	mkdir -p "$GATHER_BIN" "$MOCK_STATE"
	cp "$SCRIPT_DIR/"{gather,common.sh,version,status_reporting.sh,collectors.conf} "$GATHER_BIN/"
	cat >"$GATHER_BIN/collector" <<'EOF'
#!/bin/bash
unit=${STATUS_TEST_UNIT:-${0##*/}}
printf '%s\t%s\t%s\n' "$unit" "$*" "${MUST_GATHER_NODES_INSPECTED:-}" >>"$MOCK_STATE/calls"
if [[ $unit == gather_aro && ${MOCK_CHECK_ORDER:-0} == 1 ]]; then
	completed=("$MOCK_STATE/"*.done)
	((${#completed[@]} == 27)) || exit 98
fi
if [[ $unit == "${MOCK_WAIT_UNIT:-}" ]]; then
	for ((i=0; i<400; i++)); do [[ ! -f $MOCK_STATE/gather_istio.done ]] || break; sleep 0.02; done
	[[ -f $MOCK_STATE/gather_istio.done ]] || exit 99
fi
if [[ $unit == "${MOCK_SLOW_UNIT:-}" ]]; then
	if [[ ${MOCK_IGNORE_TERM:-false} == true ]]; then trap '' TERM; fi
	sleep 300 &
	printf '%s\n' "$!" >"$MOCK_STATE/descendant"
	: >"$MOCK_STATE/ready"
	wait "$!"
fi
source "$GATHER_BIN/status_reporting.sh"
if [[ $unit == "${MOCK_ERROR_UNIT:-}" ]]; then report_error TEST_PARTIAL "one subcommand failed"; fi
if [[ $unit == "${MOCK_SKIP_UNIT:-}" ]]; then report_skip TEST_ABSENT "optional target absent"; fi
: >"$MOCK_STATE/$unit.done"
case ",${MOCK_FAIL_UNITS:-}," in *",$unit,"*) exit 37 ;; esac
EOF
	local script
	for script in "$SCRIPT_DIR/"gather_*; do cp "$GATHER_BIN/collector" "$GATHER_BIN/${script##*/}"; done
	cat >"$GATHER_BIN/oc" <<'EOF'
#!/bin/bash
[[ ${1:-} != get ]] || exit 0
case " $* " in
*" --all-namespaces "*) export STATUS_TEST_UNIT=inspect_all_namespaces ;;
*" ns/openshift-cluster-version "*) export STATUS_TEST_UNIT=inspect_named_resources ;;
*) export STATUS_TEST_UNIT=inspect_group_resources ;;
esac
exec "$GATHER_BIN/collector" "$@"
EOF
	printf '#!/bin/bash\nexit "${MOCK_SYNC_EXIT:-0}"\n' >"$GATHER_BIN/sync"
	chmod +x "$GATHER_BIN/"*
	export PATH="$GATHER_BIN:$PATH"
}

@test "source-only reporting preserves shell options and has no inactive side effects" {
	run bash -euo pipefail -c '
		before=$(set +o); source "$SCRIPT_DIR/status_reporting.sh"
		[[ $(set +o) == "$before" ]]
		report_skip TEST_SKIP "absent"; report_error TEST_ERROR "failure"
		[[ ! -e "$BASE_COLLECTION_PATH/.status" ]]
	'
	assert_success
}

@test "individual waits preserve exits events and native collector strict mode" {
	run_reporting '
		for name in failed healthy skipped partial strict missing; do status_register "$name"; done
		status_start failed bash -c '\''source "$SCRIPT_DIR/status_reporting.sh"; report_skip TEST_SKIP "absent"; exit 37'\''
		status_start healthy true
		status_start skipped bash -c '\''source "$SCRIPT_DIR/status_reporting.sh"; report_skip TEST_SKIP "absent"'\''
		status_start partial bash -c '\''source "$SCRIPT_DIR/status_reporting.sh"; report_skip TEST_SKIP "absent"; report_error TEST_PARTIAL "failed command" error; echo continued >"$TEST_TMPDIR/continued"'\''
		status_start strict bash -euo pipefail -c '\''false | true; touch "$TEST_TMPDIR/incorrect"'\''
		status_start missing "$TEST_TMPDIR/no-such-command"
		status_wait_all; status_finalize
		[[ -f "$TEST_TMPDIR/continued" && ! -f "$TEST_TMPDIR/incorrect" ]]
	'
	assert_success
	assert_report '.schemaVersion == 1 and .mustGatherVersion == "test-version" and .overallStatus == "degraded" and .framework == {}
		and (.collectors | map(.exitCode)) == [37,0,0,0,1,127]
		and (.collectors | map(.status)) == ["error","success","skipped","error","error","error"]
		and .collectors[3].messages[1].severity == "error"
		and all(.collectors[]; .startedAt != null and .completedAt != null and .durationSeconds >= 0)'
}

@test "concurrent bounded messages redact secrets and cannot hide a late error" {
	export MESSAGE=$'A "quoted" resource\nBearer canary-token password="canary password" https://user:canary@example.test'
	run_reporting '
		status_register unit
		status_start unit bash -euo pipefail -c '\''
			source "$SCRIPT_DIR/status_reporting.sh"; printf -v padding "%2000s" ""; pids=()
			for i in {1..24}; do (report_skip TEST_SKIP "$MESSAGE$padding") & pids+=("$!"); done
			for pid in "${pids[@]}"; do wait "$pid"; done
			report_error TEST_LATE_ERROR "late error"
		'\''
		status_wait_all; status_finalize
	'
	assert_success
	assert_report '.overallStatus == "degraded" and (.collectors[0] | .status == "error" and .exitCode == 0 and (.messages | length) == 10
		and all(.messages[]; (.message | length) == 500 and (.message | contains("canary") | not)
			and (.message | contains("A \"quoted\" resource\n")) and (.message | contains("[REDACTED]"))))'
}

@test "bounded stderr drains excess preserves the log and keeps warnings successful" {
	run_reporting '
		printf "prefix\n" >"$TEST_TMPDIR/console"
		status_register first; status_register noisy
		status_start first bash -c "printf '\''warning\\n'\'' >&2" 2>>"$TEST_TMPDIR/console"
		status_wait_all
		status_start noisy bash -c "head -c 262144 /dev/zero | tr '\''\\0'\'' x >&2" 2>>"$TEST_TMPDIR/console"
		status_wait_all; status_finalize
		artifact=$(jq -r ".collectors[1].artifacts[0]" "$BASE_COLLECTION_PATH/status.json")
		[[ $(wc -c <"$BASE_COLLECTION_PATH/$artifact") == 65536 && $(wc -c <"$TEST_TMPDIR/console") == 262159 ]]
		[[ $(head -n2 "$TEST_TMPDIR/console") == $'\''prefix\nwarning'\'' ]]
	'
	assert_success
	assert_report '.overallStatus == "success" and all(.collectors[]; .exitCode == 0 and .messages == [])'
}

@test "parent reports all 28 units preserves arguments concurrency and ARO ordering" {
	prepare_gather
	export MOCK_CHECK_ORDER=1 MOCK_WAIT_UNIT=gather_insights MUST_GATHER_SINCE=2h REDUCE_LOGS=skip_rotated_logs
	run bash "$GATHER_BIN/gather"
	assert_success
	assert_report '.overallStatus == "success" and .mustGatherVersion == "test-image-version" and (.collectors | length) == 28
		and ([.collectors[].name] | unique | length) == 28 and all(.collectors[]; .status == "success" and .exitCode == 0)'
	run bash -c '
		jq -r ".collectors[].name" "$BASE_COLLECTION_PATH/status.json" | sort >"$TEST_TMPDIR/reported"
		cut -f1 "$MOCK_STATE/calls" | sort >"$TEST_TMPDIR/executed"
		cmp "$TEST_TMPDIR/reported" "$TEST_TMPDIR/executed"
	'
	assert_success
	run awk -F '\t' '
		$1 == "gather_ppc" && $3 == "true" {ppc=1}
		$1 == "gather_service_logs" && $2 == "master" {service=1}
		$1 ~ /^inspect_/ && ($2 !~ /--since=2h/ || $2 ~ /--rotated-pod-logs/) {bad=1}
		END {exit !(ppc && service && !bad && $1 == "gather_aro")}
	' "$MOCK_STATE/calls"
	assert_success
	run cat "$BASE_COLLECTION_PATH/version"
	assert_output $'openshift/must-gather\ntest-image-version'
}

@test "background and ARO errors degrade the archive while gather exits zero" {
	prepare_gather
	export MOCK_FAIL_UNITS=gather_insights,gather_aro MOCK_ERROR_UNIT=gather_monitoring MOCK_SKIP_UNIT=gather_vsphere
	run bash "$GATHER_BIN/gather"
	assert_success
	assert_report '.overallStatus == "degraded"
		and (.collectors[] | select(.name == "gather_insights") | .status == "error" and .exitCode == 37)
		and (.collectors[] | select(.name == "gather_aro") | .status == "error" and .exitCode == 37)
		and (.collectors[] | select(.name == "gather_monitoring") | .status == "error" and .exitCode == 0)
		and (.collectors[] | select(.name == "gather_vsphere") | .status == "skipped" and .exitCode == 0)'
}

@test "cancellation preserves completed work and handles ARO and stubborn descendants" {
	prepare_gather
	local mode expected
	for mode in TERM INT stubborn; do
		export MOCK_STATE="$TEST_TMPDIR/$mode" BASE_COLLECTION_PATH="$TEST_TMPDIR/output-$mode" MOCK_IGNORE_TERM=false
		mkdir -p "$MOCK_STATE"
		if [[ $mode == INT ]]; then
			export MOCK_SLOW_UNIT=gather_aro
			expected=130
		else
			export MOCK_SLOW_UNIT=gather_istio
			expected=143
		fi
		if [[ $mode == stubborn ]]; then export MOCK_IGNORE_TERM=true; fi
		run bash -c '
			env --default-signal=INT bash "$GATHER_BIN/gather" >"$MOCK_STATE/log" 2>&1 & parent=$!
			for ((i=0; i<600; i++)); do [[ ! -f "$MOCK_STATE/ready" ]] || break; sleep 0.02; done
			if [[ ! -f "$MOCK_STATE/ready" ]]; then kill -TERM "$parent"; wait "$parent"; cat "$MOCK_STATE/log"; exit 99; fi
			if [[ $1 == INT ]]; then kill -INT "$parent"; else kill -TERM "$parent"; fi
			wait "$parent"; rc=$?; cat "$MOCK_STATE/log"; exit "$rc"
		' test "$mode"
		assert_failure "$expected"
		assert_report '.overallStatus == "error" and .interrupted == true and (.collectors | length) == 28
			and (.collectors[] | select(.name == "inspect_named_resources") | .status == "success" and .exitCode == 0)
			and all(.collectors[] | select(.status == "error"); .exitCode == null and .messages[0].code == "INTERRUPTED")'
		if [[ $mode != INT ]]; then assert_report '.collectors[] | select(.name == "gather_aro") | .startedAt == null and .durationSeconds == null'; fi
		run bash -c 'state=$(ps -o stat= -p "$(cat "$MOCK_STATE/descendant")"); [[ -z $state || $state == Z* ]]'
		assert_success
		run find "$BASE_COLLECTION_PATH/.status" -type p -print
		assert_output ""
	done
}

@test "framework failures do not claim finalized success or stop unrelated collectors" {
	prepare_gather
	local mode
	for mode in flush init config publish; do
		export MOCK_STATE="$TEST_TMPDIR/$mode" BASE_COLLECTION_PATH="$TEST_TMPDIR/output-$mode" MOCK_SYNC_EXIT=0
		mkdir -p "$MOCK_STATE"
		unset REDUCE_LOGS
		case "$mode" in
		flush) export MOCK_SYNC_EXIT=1 ;;
		init) : >"$BASE_COLLECTION_PATH" ;;
		config) export REDUCE_LOGS=invalid ;;
		publish)
			cat >"$GATHER_BIN/mv" <<'EOF'
#!/bin/bash
if [[ $2 == "$BASE_COLLECTION_PATH/status.json" ]]; then exit 1; fi
exec /usr/bin/mv "$@"
EOF
			chmod +x "$GATHER_BIN/mv"
			;;
		esac
		run bash "$GATHER_BIN/gather"
		assert_failure 1
		case "$mode" in
		flush) assert_report '.overallStatus == "error" and .interrupted == false and all(.collectors[]; .status == "success")' ;;
		config) assert_report '.overallStatus == "error" and (.collectors | length) == 28 and all(.collectors[]; .exitCode == null and .startedAt == null)' ;;
		*) [[ ! -f $BASE_COLLECTION_PATH/status.json ]] ;;
		esac
		if [[ $mode != config ]]; then [[ -f $MOCK_STATE/gather_aro.done ]]; fi
	done
}

@test "reporting IO failure returns control to collectors and records framework error" {
	run_reporting '
		status_register unit
		status_run unit bash -euo pipefail -c '\''
			source "$SCRIPT_DIR/status_reporting.sh"; rm -r "$MUST_GATHER_STATUS_UNIT_DIR/events"
			report_error TEST_ERROR "failed command"; touch "$TEST_TMPDIR/continued"
		'\''
		[[ -f "$TEST_TMPDIR/continued" ]]; status_finalize
	'
	assert_failure
	assert_report '.overallStatus == "error" and .collectors[0].exitCode == 0'
}

@test "malformed fragments cannot replace valid JSON and new runs ignore stale fragments" {
	run_reporting '
		status_register unit; status_run unit true; status_finalize
		cp "$BASE_COLLECTION_PATH/status.json" "$TEST_TMPDIR/published"
		mkdir "$_STATUS_DIR/unit/events/0"; printf invalid >"$_STATUS_DIR/unit/events/0/message.json"
		if status_finalize; then exit 99; fi
		cmp "$BASE_COLLECTION_PATH/status.json" "$TEST_TMPDIR/published"
		status_init "$BASE_COLLECTION_PATH" next-version
		status_register unit; status_run unit true; status_finalize
	'
	assert_success
	assert_report '.mustGatherVersion == "next-version" and .overallStatus == "success" and .collectors[0].messages == []'
}
