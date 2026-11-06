#!/bin/bash
#
# Helpers for applying MUST_GATHER_SINCE / MUST_GATHER_SINCE_TIME to collection
# that copies files. oc only forwards --since to `oc adm inspect` and to
# `oc adm node-logs` journal queries; file copies are unbounded unless the
# script applies the window itself. All helpers are no-ops with no window set.
#
#   filter_names_since     listing whose names carry timestamps
#   since_find_predicate   find(1) predicate for mtime / live-vs-rotated
#   copy_pod_dir_since     time-bounded replacement for `oc cp` of a directory
#   pod_tar_dirs_since     same for several directories as one tar stream
#   trim_jsonl_since       drop JSONL events older than the cutoff

# Parse a Go duration (MUST_GATHER_SINCE: 30m0s, 2h30m0s) into whole seconds.
# Sub-second units are dropped. Echoes 0 when nothing parses.
duration_to_seconds() {
	local rest="${1:-}" total=0 value unit

	# Strip sub-second units first so the 'm' of '500ms' is not read as minutes.
	rest="$(echo "${rest}" | sed -E 's/[0-9]+(\.[0-9]+)?(ns|us|µs|ms)//g')"
	# Truncate fractions on the remaining units: 1.5s counts as 1 second.
	rest="$(echo "${rest}" | sed -E 's/([0-9]+)\.[0-9]+/\1/g')"

	while [[ "${rest}" =~ ^([0-9]+)([dhms])(.*)$ ]]; do
		value="${BASH_REMATCH[1]}"
		unit="${BASH_REMATCH[2]}"
		rest="${BASH_REMATCH[3]}"
		case "${unit}" in
		d) total=$((total + value * 86400)) ;;
		h) total=$((total + value * 3600)) ;;
		m) total=$((total + value * 60)) ;;
		s) total=$((total + value)) ;;
		esac
	done

	echo "${total}"
}

# Resolve MUST_GATHER_SINCE / MUST_GATHER_SINCE_TIME to an absolute cutoff.
#
# Sets (exported):
#   since_cutoff_epoch   - seconds since the epoch, empty when no window
#   since_cutoff_rfc3339 - the same instant, RFC3339 UTC
#
# MUST_GATHER_SINCE_TIME wins over MUST_GATHER_SINCE, matching get_log_collection_args.
get_since_cutoff() {
	local seconds
	since_cutoff_epoch=""
	since_cutoff_rfc3339=""

	if [ -n "${MUST_GATHER_SINCE_TIME:-}" ]; then
		since_cutoff_epoch="$(date -u -d "${MUST_GATHER_SINCE_TIME}" +%s 2>/dev/null)" ||
			since_cutoff_epoch=""
	elif [ -n "${MUST_GATHER_SINCE:-}" ]; then
		seconds="$(duration_to_seconds "${MUST_GATHER_SINCE}")"
		if [ "${seconds}" -gt 0 ]; then
			since_cutoff_epoch="$(($(date -u +%s) - seconds))"
		fi
	fi

	if [ -n "${since_cutoff_epoch}" ]; then
		since_cutoff_rfc3339="$(date -u -d "@${since_cutoff_epoch}" +%Y-%m-%dT%H:%M:%SZ)"
	fi

	export since_cutoff_epoch since_cutoff_rfc3339
}

SINCE_REPORT_PATH="${SINCE_REPORT_PATH:-${BASE_COLLECTION_PATH:-/must-gather}/since-report.txt}"

since_cutoff_epoch="${since_cutoff_epoch:-}"
since_cutoff_rfc3339="${since_cutoff_rfc3339:-}"

# True when get_since_cutoff resolved a window.
since_window_requested() {
	[ -n "${since_cutoff_epoch:-}" ]
}

# find(1) predicate that keeps files in the window, or empty with no window.
# Intended for unquoted expansion:
#
#   find /var/log/ovnkube -type f $(since_find_predicate) -name 'libovsdb*log*'
#
# A file is kept when it was written inside the window *or* it is not an
# archived copy (no date / .gz / .N in the name). Live logs have no timestamp
# in the name; dropping them when idle would leave nothing for that service.
since_find_predicate() {
	if ! since_window_requested; then
		return 0
	fi

	printf '%s' \
		"'(' -newermt @${since_cutoff_epoch} -o '!' '(' " \
		"-name '*[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*' " \
		"-o -name '*.gz' -o -name '*.[0-9]' -o -name '*.[0-9][0-9]' " \
		"')' ')'"
}

# Epoch seconds encoded in a log file name, or empty if none.
# Understands audit-...T00-15-40... and insights-...-181710... styles.
filename_timestamp_epoch() {
	local name="$1" stamp=""

	# 2026-10-16T00-15-40 (audit style: 'T' separator, '-' in the time)
	stamp="$(echo "${name}" |
		sed -nE 's/.*([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2})[-:]([0-9]{2})[-:]([0-9]{2}).*/\1 \2:\3:\4/p')"

	# 2026-10-16-181710 (insights style: '-' separator, no punctuation in time)
	if [ -z "${stamp}" ]; then
		stamp="$(echo "${name}" |
			sed -nE 's/.*([0-9]{4}-[0-9]{2}-[0-9]{2})-([0-9]{2})([0-9]{2})([0-9]{2}).*/\1 \2:\3:\4/p')"
	fi

	if [ -z "${stamp}" ]; then
		return 0
	fi

	date -u -d "${stamp} UTC" +%s 2>/dev/null || true
}

# Filter a listing on stdin, dropping entries whose name-timestamp is before the cutoff.
# Names with no timestamp always pass (live audit.log, termination.log, .lock, .terminating).
#
# Usage: ... | filter_names_since [label]
filter_names_since() {
	local label="${1:-collection}"
	local line name epoch kept=0 skipped=0

	if ! since_window_requested; then
		cat
		return 0
	fi

	while IFS= read -r line; do
		# Listings from --role=<role> are prefixed with the node name.
		name="${line##* }"
		epoch="$(filename_timestamp_epoch "${name}")"

		if [ -z "${epoch}" ] || [ "${epoch}" -ge "${since_cutoff_epoch}" ]; then
			printf '%s\n' "${line}"
			kept=$((kept + 1))
		else
			skipped=$((skipped + 1))
		fi
	done

	since_report "${label}" "filename-timestamp" "${kept}" "${skipped}"
}

# Copy a directory out of a pod, limited to files in the window.
# Replaces `oc cp <ns>/<pod>:<src> <dest>`, which cannot express a time bound.
# Falls back to plain `oc cp` when find/tar are missing in the container.
#
# Trailing-slash layout matches rsync / oc cp:
#   src = /a/b   ->  <dest>/b/...
#   src = /a/b/  ->  <dest>/...
#
# Usage: copy_pod_dir_since <namespace> <pod> <container> <src-dir> <dest-dir> [label]
# Pass "" for container to use the pod default.
copy_pod_dir_since() {
	local ns="$1" pod="$2" container="$3" src="$4" dest="$5"
	local label="${6:-${pod}}"
	local predicate before after total chdir target
	local -a c_arg=()

	[ -n "${container}" ] && c_arg=(-c "${container}")

	mkdir -p "${dest}"

	if ! since_window_requested; then
		oc cp -n "${ns}" "${c_arg[@]}" "${pod}:${src%/}" "${dest}"
		return $?
	fi

	if [ "${src}" != "${src%/}" ]; then
		chdir="${src%/}"
		target="."
	else
		chdir="$(dirname "${src}")"
		target="$(basename "${src}")"
	fi

	if ! oc exec -n "${ns}" "${pod}" "${c_arg[@]}" -- \
		sh -c 'command -v find >/dev/null 2>&1 && command -v tar >/dev/null 2>&1' 2>/dev/null; then
		echo "INFO: ${label}: find/tar unavailable in ${pod}; copying ${src} whole" >&2
		oc cp -n "${ns}" "${c_arg[@]}" "${pod}:${src%/}" "${dest}"
		since_report "${label}" "unfiltered-fallback" "all" "0"
		return $?
	fi

	total="$(oc exec -n "${ns}" "${pod}" "${c_arg[@]}" -- \
		sh -c "cd '${chdir}' 2>/dev/null && find '${target}' -type f 2>/dev/null | wc -l" 2>/dev/null | tr -d '[:space:]')"
	before="$(find "${dest}" -type f 2>/dev/null | wc -l)"

	predicate="$(since_find_predicate)"

	# shellcheck disable=SC2086
	oc exec -n "${ns}" "${pod}" "${c_arg[@]}" -- \
		sh -c "cd '${chdir}' 2>/dev/null && find '${target}' -type f ${predicate} -print0 2>/dev/null | tar czf - --null -T - 2>/dev/null" |
		tar xzf - -C "${dest}" 2>/dev/null

	after="$(find "${dest}" -type f 2>/dev/null | wc -l)"
	since_report "${label}" "find-newermt" "$((after - before))" \
		"$((${total:-0} - (after - before)))"
}

# Stream several directories out of a pod as one tarball, limited to the window.
# Missing directories are skipped (like tar --ignore-failed-read).
#
# Usage: pod_tar_dirs_since <namespace> <pod> <container> <dest-dir> <label> <src-dir>...
pod_tar_dirs_since() {
	local ns="$1" pod="$2" container="$3" dest="$4" label="$5"
	shift 5
	local predicate quoted=""
	local -a c_arg=()

	[ -n "${container}" ] && c_arg=(-c "${container}")
	mkdir -p "${dest}"

	local dir
	for dir in "$@"; do
		# relative to / so tar records paths without a leading slash
		quoted+="${quoted:+ }'${dir#/}'"
	done

	predicate="$(since_find_predicate)"

	# shellcheck disable=SC2086
	oc exec -n "${ns}" "${pod}" "${c_arg[@]}" -- \
		sh -c "cd / && find ${quoted} -type f ${predicate} -print0 2>/dev/null | tar czf - --null -T - 2>/dev/null" |
		tar xzf - -C "${dest}" 2>/dev/null

	if since_window_requested; then
		since_report "${label}" "find-newermt" \
			"$(find "${dest}" -type f 2>/dev/null | wc -l)" "pre-cutoff"
	else
		since_report "${label}" "no-window" \
			"$(find "${dest}" -type f 2>/dev/null | wc -l)" "0"
	fi
}

# Drop JSONL records older than the cutoff by an RFC3339 field (default
# requestReceivedTimestamp). Non-JSON lines and objects without the field pass
# through. No-op with no window or when jq is missing.
#
# Usage: ... | trim_jsonl_since [field]
trim_jsonl_since() {
	local field="${1:-requestReceivedTimestamp}"

	if ! since_window_requested || ! command -v jq >/dev/null 2>&1; then
		cat
		return 0
	fi

	jq -crR --arg cutoff "${since_cutoff_rfc3339}" --arg field "${field}" '
		. as $line
		| (try fromjson catch null) as $o
		| if $o == null or ($o | type) != "object" then $line
		  elif (($o[$field] // "") == "") then $o
		  elif ($o[$field] >= $cutoff) then $o
		  else empty
		  end' 2>/dev/null
}

# Append one line to the --since report for a collection site.
since_report() {
	local label="$1" mechanism="$2" kept="$3" skipped="$4"

	mkdir -p "$(dirname "${SINCE_REPORT_PATH}")" 2>/dev/null || return 0
	printf '%s\t%s\t%s\tkept=%s\tskipped=%s\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${label}" "${mechanism}" "${kept}" "${skipped}" \
		>>"${SINCE_REPORT_PATH}" 2>/dev/null || true
}

# Write the --since report header once.
since_report_header() {
	mkdir -p "$(dirname "${SINCE_REPORT_PATH}")" 2>/dev/null || return 0
	if [ -s "${SINCE_REPORT_PATH}" ]; then
		return 0
	fi
	{
		echo "# must-gather --since report"
		if since_window_requested; then
			echo "# window: everything at or after ${since_cutoff_rfc3339} (epoch ${since_cutoff_epoch})"
			echo "# requested as: MUST_GATHER_SINCE='${MUST_GATHER_SINCE:-}' MUST_GATHER_SINCE_TIME='${MUST_GATHER_SINCE_TIME:-}'"
		else
			echo "# window: none requested; everything collected"
		fi
		printf '# %s\t%s\t%s\t%s\t%s\n' "when" "what" "mechanism" "kept" "skipped"
	} >>"${SINCE_REPORT_PATH}" 2>/dev/null || true
}

export SINCE_REPORT_PATH

# Export functions so they survive `xargs ... bash -c` / `sh -c` fan-out.
# Without this, $(since_find_predicate) expands empty and the window is ignored.
if [ -n "${BASH_VERSION:-}" ]; then
	export -f duration_to_seconds get_since_cutoff
	export -f since_window_requested since_find_predicate
	export -f filename_timestamp_epoch filter_names_since
	export -f copy_pod_dir_since pod_tar_dirs_since trim_jsonl_since
	export -f since_report since_report_header
fi

function get_operator_ns() {
	# shellcheck disable=SC2155,SC2116,SC2086
	local operator_name=$(echo \"$1\")
	# shellcheck disable=SC2034,SC2116,SC2028
	cmd="$(echo "oc get subs -A -o template --template '{{range .items}}{{if eq .spec.name ""${operator_name}""}}{{.metadata.namespace}}{{\"\\n\"}}{{end}}{{end}}'")"
	operator_ns="$(eval "$cmd")"

	if [ -z "${operator_ns}" ]; then
		echo "INFO: ${operator_name} not detected. Skipping."
		exit 0
	fi

	if [[ "$(echo "${operator_ns}" | wc -l)" -gt 1 ]]; then
		echo "ERROR: found more than one ${operator_name} subscription. Exiting."
		exit 1
	fi
}

get_log_collection_args() {
	# validation of MUST_GATHER_SINCE and MUST_GATHER_SINCE_TIME is done by the
	# caller (oc adm must-gather) so it's safe to use the values as they are.
	log_collection_args=""

	if [ -n "${MUST_GATHER_SINCE:-}" ]; then
		log_collection_args=--since="${MUST_GATHER_SINCE}"
	fi
	if [ -n "${MUST_GATHER_SINCE_TIME:-}" ]; then
		log_collection_args=--since-time="${MUST_GATHER_SINCE_TIME}"
	fi


	# REDUCE_LOGS: unset = defaults. Comma or space separated list of:
	#   skip_rotated_logs     - omit --rotated-pod-logs from oc adm inspect
	#   compress_service_logs - gzip host service / Windows node logs (see gather_service_logs_util, gather_windows_node_logs)
	#   compress_logs         - gzip collected .log files >=10MB after gather (before rsync)

	rotated_pod_logs_arg="--rotated-pod-logs"
	compress_service_logs=""
	compress_after_gather=""

	if [ -n "${REDUCE_LOGS:-}" ]; then
		# Normalize commas to spaces, then validate each option.
		# Intentional word-splitting of comma/space separated options.
		# shellcheck disable=SC2086
		for reduce_logs_option in ${REDUCE_LOGS//,/ }; do
			case "${reduce_logs_option}" in
			skip_rotated_logs)
				rotated_pod_logs_arg=""
				;;
			compress_service_logs)
				compress_service_logs=true
				;;
			compress_logs)
				compress_after_gather=true
				;;
			"")
				;;
			*)
				echo "ERROR: REDUCE_LOGS unknown value '${reduce_logs_option}'. Allowed: skip_rotated_logs, compress_service_logs, compress_logs (got: [${REDUCE_LOGS}])." >&2
				exit 1
				;;
			esac
		done
	fi

	# oc adm node-logs `--since` parameter is not the same as oc adm inspect `--since`.
	# it takes a simplified duration in the form of '(+|-)[0-9]+(s|m|h|d)' or
	# an ISO formatted time. since MUST_GATHER_SINCE and MUST_GATHER_SINCE_TIME
	# are formatted differently, we re-format them so they can be used
	# transparently by node-logs invocations.
	node_log_collection_args=""

	# shellcheck disable=SC2001
	if [ -n "${MUST_GATHER_SINCE:-}" ]; then
		since=$(echo "${MUST_GATHER_SINCE:-}" | sed 's/\([0-9]*[dhms]\).*/\1/')
		node_log_collection_args=--since="-${since}"
	fi
	if [ -n "${MUST_GATHER_SINCE_TIME:-}" ]; then
		iso_time=$(echo "${MUST_GATHER_SINCE_TIME}" | sed 's/T/ /; s/Z//')
		node_log_collection_args=--since="${iso_time}"
	fi

	# Export globals used by gather_* scripts that source this file (also satisfies ShellCheck SC2034):
	# log_collection_args, node_log_collection_args, rotated_pod_logs_arg, compress_service_logs, compress_after_gather.
	export log_collection_args node_log_collection_args rotated_pod_logs_arg compress_service_logs compress_after_gather
}

# Compress collected .log / .log.* files larger than 10MB (skip already-gzipped).
# Uses gzip -1 (fastest) with 2 parallel jobs.
compress_logs() {
	local target_dir="${1:-/must-gather}"

	echo "Compressing collected logs in parallel (jobs=2)..."
	# Find large logs, then gzip up to 2 at a time.
	# -print0 / xargs -0: safe with spaces in paths
	# xargs -r: skip gzip if find matches nothing
	find "${target_dir}" \
		\( -name '*.log' -o -name '*.log.*' \) \
		! -name '*.gz' \
		-size +10M \
		-print0 |
		xargs -0 -r -P 2 gzip -1
	echo "Compression complete."
}
