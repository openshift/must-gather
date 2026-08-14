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
	# -print0 / xargs -0: keep path names with spaces/quotes intact
	# -r: do not run gzip when find matches nothing (GNU xargs)
	find "${target_dir}" \( -name '*.log' -o -name '*.log.*' \) ! -name '*.gz' -size +10M -print0 \
		| xargs -0 -r -P 2 gzip -1
	echo "Compression complete."
}
