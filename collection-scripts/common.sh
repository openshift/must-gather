#!/bin/bash

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
		# shellcheck disable=SC2034
		log_collection_args=--since-time="${MUST_GATHER_SINCE_TIME}"
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
	# shellcheck disable=SC2034
	if [ -n "${MUST_GATHER_SINCE_TIME:-}" ]; then
		iso_time=$(echo "${MUST_GATHER_SINCE_TIME}" | sed 's/T/ /; s/Z//')
		node_log_collection_args=--since="${iso_time}"
	fi
}

# Gzip pod log files under must-gather (oc adm inspect layout: .../namespaces/<ns>/pods/<pod>/.../logs/
# e.g. current.log, previous.log, rotated/*.log). Reduces must-gather size and frees disk during
# collection so the node does not run out of space. Call from gather scripts after they write logs.
# Each file is gzipped only if it still exists when reached (avoids errors when parallel compress
# runs overlap). Skipped when OPENSHIFT_CI or ARTIFACTS_DIR is set (CI environments).
# Usage: compress_pod_logs_must_gather [root]  (default: /must-gather)
compress_pod_logs_must_gather() {
	if [ -n "${OPENSHIFT_CI:-}" ] || [ -n "${ARTIFACTS_DIR:-}" ]; then
		echo "Skipping pod log compression: OPENSHIFT_CI=${OPENSHIFT_CI:-} ARTIFACTS_DIR=${ARTIFACTS_DIR:-}"
		return 0
	fi
	local _root="${1:-/must-gather}"
	if [ ! -d "${_root}" ]; then
		return 0
	fi
	local pod_log_count
	pod_log_count=$(find "${_root}" -path '*/namespaces/*/pods/*/logs/*' -type f ! -name '*.gz' 2>/dev/null | wc -l)
	if [ "${pod_log_count}" -gt 0 ]; then
		echo "Compressing ${pod_log_count} pod log file(s) under ${_root} to reduce must-gather size."
		find "${_root}" -path '*/namespaces/*/pods/*/logs/*' -type f ! -name '*.gz' -print0 | xargs -0 -P 8 -r sh -c 'for f; do [ -f "$f" ] && gzip -f "$f" 2>/dev/null; done' _
	fi
}
