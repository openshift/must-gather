#!/usr/bin/env bats
# Tests for the --since helpers in collection-scripts/common.sh.
#
# These cover the behaviour that makes --since mean something for file-based
# collection: oc drops the since parameter for any --path request, so the
# window has to be applied by the collection scripts themselves.

load test_helper

# =============================================================================
# duration_to_seconds()
# =============================================================================

@test "duration_to_seconds sums every component of a Go duration" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		duration_to_seconds '2h30m0s'
	"
	assert_success
	assert_output "9000"
}

@test "duration_to_seconds handles single components" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		echo \"\$(duration_to_seconds '30m0s') \$(duration_to_seconds '45s') \$(duration_to_seconds '1h0m0s') \$(duration_to_seconds '2d0h0m0s')\"
	"
	assert_success
	assert_output "1800 45 3600 172800"
}

@test "duration_to_seconds does not mistake the m of ms for minutes" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		duration_to_seconds '500ms'
	"
	assert_success
	assert_output "0"
}

@test "duration_to_seconds truncates fractional units" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		duration_to_seconds '1.5s'
	"
	assert_success
	assert_output "1"
}

@test "duration_to_seconds returns 0 for junk" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		echo \"[\$(duration_to_seconds 'soon')][\$(duration_to_seconds '')]\"
	"
	assert_success
	assert_output "[0][0]"
}


# =============================================================================
# get_since_cutoff()
# =============================================================================

@test "get_since_cutoff resolves MUST_GATHER_SINCE to an absolute instant" {
	run bash -c "
		export MUST_GATHER_SINCE='1h0m0s'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		now=\$(date -u +%s)
		delta=\$((now - since_cutoff_epoch))
		# allow a couple of seconds of slack for slow machines
		[ \"\$delta\" -ge 3598 ] && [ \"\$delta\" -le 3602 ] && echo in-range || echo \"out-of-range: \$delta\"
	"
	assert_success
	assert_output "in-range"
}

@test "get_since_cutoff resolves MUST_GATHER_SINCE_TIME exactly" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T09:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		echo \"\$since_cutoff_rfc3339\"
	"
	assert_success
	assert_output "2026-10-16T09:00:00Z"
}

@test "get_since_cutoff prefers MUST_GATHER_SINCE_TIME over MUST_GATHER_SINCE" {
	run bash -c "
		export MUST_GATHER_SINCE='8h'
		export MUST_GATHER_SINCE_TIME='2026-10-16T09:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		echo \"\$since_cutoff_rfc3339\"
	"
	assert_success
	assert_output "2026-10-16T09:00:00Z"
}

@test "get_since_cutoff leaves the cutoff empty when no window was requested" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		echo \"[\$since_cutoff_epoch][\$since_cutoff_rfc3339]\"
	"
	assert_success
	assert_output "[][]"
}

# =============================================================================
# since_find_predicate()
# =============================================================================

@test "since_find_predicate is empty without a window" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		echo \"[\$(since_find_predicate)]\"
	"
	assert_success
	assert_output "[]"
}

@test "since_find_predicate emits a newermt predicate with a window" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T09:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		since_find_predicate
	"
	assert_success
	assert_output --partial "-newermt @1792141200"
}

@test "the window keeps a live log even when nothing was written during it" {
	# mtime says when a file was last written, not what period it covers.
	# A VM whose QEMU log was last written half an hour ago is still running,
	# and dropping its only log leaves nothing at all for that VM. The same
	# would empty /var/log/ceph on an idle cluster.
	mkdir -p "$TEST_TMPDIR/logs"
	touch -d '30 minutes ago' "$TEST_TMPDIR/logs/cnv-test_bg-testvm.log"
	touch -d '30 minutes ago' "$TEST_TMPDIR/logs/ceph-osd.1.log"
	touch -d '2 minutes ago' "$TEST_TMPDIR/logs/ceph-mon.a.log"

	run bash -c "
		export MUST_GATHER_SINCE=5m0s
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		eval \"find '$TEST_TMPDIR/logs' -type f \$(since_find_predicate)\" | sed 's|.*/||' | sort
	"
	assert_success
	assert_output "ceph-mon.a.log
ceph-osd.1.log
cnv-test_bg-testvm.log"
}

@test "the window still drops rotated and compressed copies" {
	mkdir -p "$TEST_TMPDIR/rot"
	touch -d '3 days ago' "$TEST_TMPDIR/rot/audit-2026-10-15T19-30-44.933.log"
	touch -d '3 days ago' "$TEST_TMPDIR/rot/ceph-osd.1.log.gz"
	touch -d '3 days ago' "$TEST_TMPDIR/rot/messages.1"
	touch -d '3 days ago' "$TEST_TMPDIR/rot/live.log"

	run bash -c "
		export MUST_GATHER_SINCE=5m0s
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		eval \"find '$TEST_TMPDIR/rot' -type f \$(since_find_predicate)\" | sed 's|.*/||' | sort
	"
	assert_success
	# only the live one survives; the three archived copies do not
	assert_output "live.log"
}

@test "the predicate is still empty without a window" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		echo \"[\$(since_find_predicate)]\"
	"
	assert_success
	assert_output "[]"
}

# =============================================================================
# filename_timestamp_epoch()
# =============================================================================

@test "filename_timestamp_epoch reads the audit log naming scheme" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		filename_timestamp_epoch 'audit-2026-10-16T00-15-40.612.log'
	"
	assert_success
	assert_output "1792109740"
}

@test "filename_timestamp_epoch reads the insights archive naming scheme" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		filename_timestamp_epoch 'insights-2026-10-16-181710.tar.gz'
	"
	assert_success
	assert_output "1792174630"
}

@test "filename_timestamp_epoch is silent for names without a timestamp" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		for n in audit.log termination.log .lock .terminating startup.log; do
			printf '[%s]' \"\$(filename_timestamp_epoch \"\$n\")\"
		done
	"
	assert_success
	assert_output "[][][][][]"
}

# =============================================================================
# filter_names_since()
# =============================================================================

@test "filter_names_since passes everything through without a window" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' 'audit-2020-01-01T00-00-00.000.log' 'audit.log' | filter_names_since test
	"
	assert_success
	assert_output "audit-2020-01-01T00-00-00.000.log
audit.log"
}

@test "filter_names_since drops rotated files that end before the cutoff" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' \
			'audit-2026-10-16T18-00-00.000.log' \
			'audit-2026-10-16T06-00-00.000.log' | filter_names_since test
	"
	assert_success
	assert_output "audit-2026-10-16T18-00-00.000.log"
}

@test "filter_names_since always keeps entries with no timestamp in the name" {
	# audit.log is the live file and termination.log has no rotation stamp;
	# these are the most likely to hold recent events, so they must survive.
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' 'audit.log' 'termination.log' '.lock' '.terminating' \
			'audit-2026-10-16T06-00-00.000.log' | filter_names_since test
	"
	assert_success
	assert_output "audit.log
termination.log
.lock
.terminating"
}

@test "filter_names_since handles listings prefixed with a node name" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' \
			'master-0 audit-2026-10-16T18-00-00.000.log' \
			'master-1 audit-2026-10-16T06-00-00.000.log' | filter_names_since test
	"
	assert_success
	assert_output "master-0 audit-2026-10-16T18-00-00.000.log"
}

@test "filter_names_since records what it kept and skipped" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' 'audit.log' \
			'audit-2026-10-16T18-00-00.000.log' \
			'audit-2026-10-16T06-00-00.000.log' \
			'audit-2026-10-15T06-00-00.000.log' | filter_names_since audit_logs/kube-apiserver >/dev/null
		cat \"$TEST_TMPDIR/report.txt\"
	"
	assert_success
	assert_output --partial "audit_logs/kube-apiserver"
	assert_output --partial "kept=2"
	assert_output --partial "skipped=2"
}

# =============================================================================
# trim_jsonl_since()
# =============================================================================

@test "trim_jsonl_since passes everything through without a window" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' '{\"requestReceivedTimestamp\":\"2020-01-01T00:00:00Z\"}' | trim_jsonl_since
	"
	assert_success
	assert_output '{"requestReceivedTimestamp":"2020-01-01T00:00:00Z"}'
}

@test "trim_jsonl_since drops audit events older than the cutoff" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' \
			'{\"auditID\":\"new\",\"requestReceivedTimestamp\":\"2026-10-16T18:00:00.1Z\"}' \
			'{\"auditID\":\"old\",\"requestReceivedTimestamp\":\"2026-10-16T06:00:00.1Z\"}' | trim_jsonl_since
	"
	assert_success
	assert_output --partial '"auditID":"new"'
	refute_output --partial '"auditID":"old"'
}

@test "trim_jsonl_since passes non-JSON lines through verbatim" {
	# /var/log/etcd holds plain text, not audit events. Silently emptying it
	# would be far worse than collecting too much.
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' \
			'2026-10-16 06:00:00 I | etcdserver: plain text line' \
			'another plain line' | trim_jsonl_since
	"
	assert_success
	assert_output "2026-10-16 06:00:00 I | etcdserver: plain text line
another plain line"
}

@test "trim_jsonl_since keeps JSON objects that lack the timestamp field" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		printf '%s\n' '{\"auditID\":\"nofield\"}' | trim_jsonl_since
	"
	assert_success
	assert_output '{"auditID":"nofield"}'
}

# =============================================================================
# since_report()
# =============================================================================

@test "since_report_header names the window in force" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T09:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		since_report_header
		cat \"$TEST_TMPDIR/report.txt\"
	"
	assert_success
	assert_output --partial "must-gather --since report"
	assert_output --partial "2026-10-16T09:00:00Z"
}

@test "since_report_header says so when no window was requested" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		since_report_header
		cat \"$TEST_TMPDIR/report.txt\"
	"
	assert_success
	assert_output --partial "none requested"
}

@test "since_report_header is written only once" {
	run bash -c "
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		since_report_header
		since_report_header
		grep -c 'must-gather --since report' \"$TEST_TMPDIR/report.txt\"
	"
	assert_success
	assert_output "1"
}

# =============================================================================
# Sourcing is side-effect free
# =============================================================================

@test "sourcing common.sh since helpers change nothing when no window is requested" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		since_window_requested && echo 'window' || echo 'no-window'
		echo \"predicate=[\$(since_find_predicate)]\"
	"
	assert_success
	assert_output --partial "no-window"
	assert_output --partial "predicate=[]"
}

# =============================================================================
# copy_pod_dir_since() / pod_tar_dirs_since() layout
# =============================================================================
#
# These exercise the path arithmetic only, with a stub oc. Getting it wrong
# silently reshapes the archive: tarring an absolute path lands everything
# under <dest>/a/b/... instead of where oc cp or rsync would have put it.

@test "copy_pod_dir_since tars the directory itself without a trailing slash" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		mkdir -p \"$TEST_TMPDIR/mocks\"
		printf '#!/bin/bash\necho \"\$*\" >>\"$TEST_TMPDIR/cmds\"\nexit 0\n' > \"$TEST_TMPDIR/mocks/oc\"
		chmod +x \"$TEST_TMPDIR/mocks/oc\"
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		copy_pod_dir_since ns pod '' /a/b \"$TEST_TMPDIR/dest\" lbl >/dev/null 2>&1
		grep -o \"cd '[^']*' .*find '[^']*'\" \"$TEST_TMPDIR/cmds\" | tail -1
	"
	assert_success
	assert_output --partial "cd '/a'"
	assert_output --partial "find 'b'"
}

@test "copy_pod_dir_since tars the contents with a trailing slash" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		mkdir -p \"$TEST_TMPDIR/mocks\"
		printf '#!/bin/bash\necho \"\$*\" >>\"$TEST_TMPDIR/cmds\"\nexit 0\n' > \"$TEST_TMPDIR/mocks/oc\"
		chmod +x \"$TEST_TMPDIR/mocks/oc\"
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		copy_pod_dir_since ns pod '' /a/b/ \"$TEST_TMPDIR/dest\" lbl >/dev/null 2>&1
		grep -o \"cd '[^']*' .*find '[^']*'\" \"$TEST_TMPDIR/cmds\" | tail -1
	"
	assert_success
	assert_output --partial "cd '/a/b'"
	assert_output --partial "find '.'"
}

@test "copy_pod_dir_since falls back to oc cp without a window" {
	run bash -c "
		unset MUST_GATHER_SINCE MUST_GATHER_SINCE_TIME
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		mkdir -p \"$TEST_TMPDIR/mocks\"
		printf '#!/bin/bash\necho \"\$*\" >>\"$TEST_TMPDIR/cmds\"\nexit 0\n' > \"$TEST_TMPDIR/mocks/oc\"
		chmod +x \"$TEST_TMPDIR/mocks/oc\"
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		copy_pod_dir_since ns pod '' /a/b/ \"$TEST_TMPDIR/dest\" lbl >/dev/null 2>&1
		cat \"$TEST_TMPDIR/cmds\"
	"
	assert_success
	# unchanged upstream behaviour, and the trailing slash is dropped for oc cp
	assert_output --partial "cp -n ns pod:/a/b"
	refute_output --partial "find"
}

@test "pod_tar_dirs_since makes several absolute paths relative to /" {
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		export SINCE_REPORT_PATH=\"$TEST_TMPDIR/report.txt\"
		mkdir -p \"$TEST_TMPDIR/mocks\"
		printf '#!/bin/bash\necho \"\$*\" >>\"$TEST_TMPDIR/cmds\"\nexit 0\n' > \"$TEST_TMPDIR/mocks/oc\"
		chmod +x \"$TEST_TMPDIR/mocks/oc\"
		export PATH=\"$TEST_TMPDIR/mocks:\$PATH\"
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		pod_tar_dirs_since ns pod compute \"$TEST_TMPDIR/dest\" lbl \
			/var/log/libvirt/qemu/ /var/run/libvirt/qemu/log/ >/dev/null 2>&1
		cat \"$TEST_TMPDIR/cmds\"
	"
	assert_success
	assert_output --partial "cd / && find 'var/log/libvirt/qemu/' 'var/run/libvirt/qemu/log/'"
	assert_output --partial "-newermt @"
}

@test "every script calling the since helpers also sources common.sh" {
	# gather runs each gather_* as a separate script, so functions defined in
	# a parent shell are not inherited. Forgetting the source is silent: the
	# collection runs, logs nothing unusual, and gathers nothing.
	cd "$SCRIPT_DIR"
	local missing=""
	for f in gather_*; do
		if grep -qE '\b(copy_pod_dir_since|pod_tar_dirs_since|filter_names_since|since_find_predicate|trim_jsonl_since|since_report)\b' "$f"; then
			grep -qE 'common\.sh' "$f" || missing="$missing $f"
		fi
	done
	[ -z "$missing" ] || {
		echo "scripts use the since helpers but never source common.sh:$missing" >&2
		false
	}
}

@test "library functions survive into an xargs/sh -c grandchild" {
	# Collection scripts fan out with `xargs ... sh -c`, where a function from
	# the parent shell is not visible. Without export -f the failure is quiet
	# and expensive: $(since_find_predicate) becomes empty, find runs
	# unfiltered, and the window is silently ignored.
	run bash -c "
		export MUST_GATHER_SINCE_TIME='2026-10-16T12:00:00Z'
		source \"$SCRIPT_DIR/common.sh\"
		get_since_cutoff
		sh -c 'since_find_predicate'
	"
	assert_success
	assert_output --partial "-newermt @1792152000"
}

@test "every public library function is exported" {
	run bash -c "
		source \"$SCRIPT_DIR/common.sh\"
		missing=''
		for f in duration_to_seconds get_since_cutoff \
			since_window_requested since_find_predicate filename_timestamp_epoch \
			filter_names_since copy_pod_dir_since pod_tar_dirs_since \
			trim_jsonl_since since_report since_report_header; do
			declare -F \"\$f\" >/dev/null || { missing=\"\$missing \$f(undefined)\"; continue; }
			[[ \$(declare -pF \"\$f\") == *' -fx '* ]] || missing=\"\$missing \$f\"
		done
		echo \"[\$missing]\"
	"
	assert_success
	assert_output "[]"
}
