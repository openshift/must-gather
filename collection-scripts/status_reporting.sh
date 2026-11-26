#!/bin/bash
# Source-only library; collector shell options and exit codes stay unchanged.
# Framework codes: COLLECTOR_EXIT_NONZERO, INTERRUPTED, STATUS_RESULT_MISSING.

_status_failure() {
	_STATUS_FRAMEWORK_ERROR=1
	local dir="${MUST_GATHER_STATUS_WORK_DIR:-${_STATUS_DIR:-}}"
	if [[ -n $dir ]]; then
		: >"${dir}/framework-error" 2>/dev/null || :
	fi
	printf '%s\n' 'ERROR: Must-gather status reporting failed; diagnostic collection will continue.' >&2
}

status_init() {
	declare -ga _STATUS_NAMES=() _STATUS_PIDS=()
	declare -gA _STATUS_PID_NAMES=() _STATUS_STARTED=() _STATUS_EPOCHS=()
	_STATUS_FRAMEWORK_ERROR=0 _STATUS_SIGNAL_EXIT=0
	_STATUS_INTERRUPTED=false _STATUS_LAUNCHING=false _STATUS_FINISHING=false
	_STATUS_DIR="" _STATUS_GROUPS=false
	_STATUS_ROOT=${1:-/must-gather}
	_STATUS_VERSION=${2:-${OS_GIT_VERSION:-0.0.0-unknown}}
	_STATUS_STARTED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	if command -v setsid >/dev/null; then _STATUS_GROUPS=true; fi
	if [[ $_STATUS_GROUPS == false ]] || ! command -v jq >/dev/null ||
		! mkdir -p -- "${_STATUS_ROOT}/.status" ||
		! _STATUS_ROOT=$(cd -- "${_STATUS_ROOT}" && pwd -P) ||
		! _STATUS_DIR=$(mktemp -d "${_STATUS_ROOT}/.status/run.XXXXXXXX"); then
		_status_failure
		_STATUS_DIR=""
		return 1
	fi
}

status_register() {
	[[ $1 =~ ^[a-z][a-z0-9_]*$ ]] || {
		_status_failure
		return 0
	}
	_STATUS_NAMES+=("$1")
	if [[ -n $_STATUS_DIR ]]; then mkdir -p "$_STATUS_DIR/$1/events" || _status_failure; fi
	return 0
}

report_error() { _status_report error "${3:-warning}" "$1" "$2"; }
report_skip() { _status_report skipped warning "$1" "$2"; }

_status_report() {
	local outcome=$1 severity=$2 code=$3 message=$4 slot index
	[[ -n ${MUST_GATHER_STATUS_UNIT_DIR:-} ]] || return 0
	# Outcome markers are independent of event capacity: the eleventh event can
	# still change a successful or skipped unit to error.
	if ! : >"${MUST_GATHER_STATUS_UNIT_DIR}/${outcome}"; then
		_status_failure
		return 0
	fi
	if [[ ! $code =~ ^[A-Z][A-Z0-9_]{0,127}$ || ! $severity =~ ^(warning|error)$ ]]; then
		_status_failure
		return 0
	fi
	for ((index = 0; index < 10; index++)); do
		slot="${MUST_GATHER_STATUS_UNIT_DIR}/events/${index}"
		if mkdir "$slot" 2>/dev/null; then
			# Bound the input before invoking jq, then redact before applying the
			# public character limit. Never pass raw command output to this API.
			if ! jq -n --arg severity "$severity" --arg code "$code" --arg message "${message:0:16384}" '
				def redact:
					gsub("(?i)(bearer|basic)[[:space:]]+[A-Za-z0-9._~+/-]+=*"; "[REDACTED]")
					| gsub("(?i)(?<key>access[_-]?token|refresh[_-]?token|id[_-]?token|token|password|passwd|secret|client[_-]?secret|api[_-]?key|authorization|cookie)[\"\\x27]?[[:space:]]*[:=][[:space:]]*(\"[^\"]*\"|\\x27[^\\x27]*\\x27|[^[:space:],;]+)"; "\(.key)=[REDACTED]")
					| gsub("(?<scheme>https?://)[^/@[:space:]]+:[^/@[:space:]]+@"; "\(.scheme)[REDACTED]@")
					| gsub("[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}"; "[REDACTED]");
				{severity: $severity, code: $code, message: ($message | redact | .[0:500])}
			' >"${slot}/message.tmp" || ! mv "${slot}/message.tmp" "${slot}/message.json"; then
				_status_failure
			fi
			return 0
		elif [[ ! -d $slot ]]; then
			_status_failure
			return 0
		fi
	done
	return 0
}

_status_capture_stderr() (
	local reader rc=0
	# Keep the console stream intact. Draining after head reaches its limit
	# prevents SIGPIPE from changing the collector's exit code. Redirect tee's
	# stdout to the inherited descriptor: opening /dev/stderr would truncate a
	# regular-file gather log and discard output from other collectors.
	if ! tee >({
		head -c 65536 >"$1" || _status_failure
		cat >/dev/null
	}) >&2; then
		_status_failure
		rc=1
	fi
	reader=$!
	if ! wait "$reader"; then
		_status_failure
		rc=1
	fi
	return "$rc"
)

# Each wrapper publishes its completion before exiting, independently of wait
# order. Process substitution keeps stderr streaming without named FIFO files.
_status_run() {
	local unit_dir=$1 rc reader log_fd
	shift
	if [[ -d $unit_dir ]]; then
		export MUST_GATHER_STATUS_UNIT_DIR="$unit_dir" MUST_GATHER_STATUS_WORK_DIR="${unit_dir%/*}"
		# Capture $! before the command, including readers that finish quickly.
		exec {log_fd}> >(_status_capture_stderr "$unit_dir/stderr")
		reader=$!
		if "$@" 2>&"$log_fd" {log_fd}>&-; then rc=0; else rc=$?; fi
		exec {log_fd}>&-
		wait "$reader" || _status_failure
		if ! jq -n --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson epoch "$(date +%s)" --argjson rc "$rc" \
			'{timestamp: $timestamp, epoch: $epoch, exitCode: $rc}' >"$unit_dir/completed.tmp" ||
			! mv "$unit_dir/completed.tmp" "$unit_dir/completed.json"; then _status_failure; fi
	else
		if "$@"; then rc=0; else rc=$?; fi
	fi
	return "$rc"
}

status_start() {
	local name=$1 pid
	shift
	_STATUS_LAUNCHING=true
	_STATUS_STARTED[$name]=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	_STATUS_EPOCHS[$name]=$(date +%s)
	if [[ $_STATUS_GROUPS == true ]]; then
		setsid bash "${BASH_SOURCE[0]}" --status-run "${_STATUS_DIR:+$_STATUS_DIR/$name}" "$@" &
	else
		"$@" &
	fi
	pid=$!
	_STATUS_PIDS+=("$pid")
	_STATUS_PID_NAMES[$pid]=$name
	_STATUS_LAST_PID=$pid
	_STATUS_LAUNCHING=false
	if ((_STATUS_SIGNAL_EXIT != 0)); then status_interrupt "$_STATUS_SIGNAL_EXIT"; fi
	return 0
}

status_wait() {
	wait "$1" || :
	unset '_STATUS_PID_NAMES[$1]'
}

status_wait_all() {
	local pid
	for pid in "${_STATUS_PIDS[@]}"; do
		[[ -z ${_STATUS_PID_NAMES[$pid]:-} ]] || status_wait "$pid"
	done
}

status_run() {
	status_start "$@"
	status_wait "$_STATUS_LAST_PID"
}

status_interrupt() {
	local pid name deadline
	local -a pending=()
	_STATUS_SIGNAL_EXIT=$1 _STATUS_INTERRUPTED=true
	# Defer cancellation across fork/registration and avoid recursive finalizers.
	[[ $_STATUS_LAUNCHING == false && $_STATUS_FINISHING == false ]] || return 0
	trap '' TERM INT
	for pid in "${_STATUS_PIDS[@]}"; do
		[[ -n ${_STATUS_PID_NAMES[$pid]:-} ]] || continue
		name=${_STATUS_PID_NAMES[$pid]}
		[[ -z $_STATUS_DIR || ! -s $_STATUS_DIR/$name/completed.json ]] || continue
		pending+=("$pid")
		if [[ -n $_STATUS_DIR ]]; then : >"$_STATUS_DIR/$name/interrupted" || _status_failure; fi
		if [[ $_STATUS_GROUPS == true ]]; then kill -TERM -- "-$pid" 2>/dev/null || :; fi
		kill -TERM "$pid" 2>/dev/null || :
	done
	deadline=$((SECONDS + 5))
	while ((SECONDS < deadline)) && [[ -n $(jobs -pr) ]]; do sleep 0.05; done
	for pid in "${pending[@]}"; do
		if [[ $_STATUS_GROUPS == true ]]; then kill -KILL -- "-$pid" 2>/dev/null || :; fi
		kill -KILL "$pid" 2>/dev/null || :
		wait "$pid" 2>/dev/null || :
	done
	exit "$_STATUS_SIGNAL_EXIT"
}

status_finalize() {
	local name unit completed
	local now epoch
	local interrupted error skipped has_stderr overall=success event
	local -a events=() results=()
	[[ -n $_STATUS_DIR ]] || return 1
	now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	epoch=$(date +%s)
	for name in "${_STATUS_NAMES[@]}"; do
		unit="$_STATUS_DIR/$name" completed=/dev/null
		interrupted=false error=false skipped=false has_stderr=false events=()
		[[ ! -s $unit/completed.json ]] || completed="$unit/completed.json"
		if [[ -f $unit/interrupted || ($_STATUS_INTERRUPTED == true && $completed == /dev/null) ]]; then interrupted=true; fi
		if [[ -f $unit/error ]]; then error=true; fi
		if [[ -f $unit/skipped ]]; then skipped=true; fi
		if [[ -s $unit/stderr ]]; then has_stderr=true; fi
		if [[ $completed == /dev/null && $interrupted == false ]]; then _status_failure; fi
		for event in "$unit/events/"*/message.json; do [[ ! -f $event ]] || events+=("$event"); done
		if ! jq -s --slurpfile completed "$completed" --arg name "$name" --arg now "$now" --argjson epoch "$epoch" \
			--arg start "${_STATUS_STARTED[$name]:-}" --argjson startEpoch "${_STATUS_EPOCHS[$name]:-null}" \
			--argjson interrupted "$interrupted" --argjson error "$error" --argjson skipped "$skipped" \
			--argjson hasStderr "$has_stderr" --arg stderr "${unit#"$_STATUS_ROOT/"}/stderr" '
			($completed[0] // {}) as $c | (if $interrupted then null else $c.exitCode // null end) as $rc |
			{ name: $name,
			  status: (if $rc == null or $rc != 0 or $error then "error" elif $skipped then "skipped" else "success" end),
			  exitCode: $rc, startedAt: (if $start == "" then null else $start end), completedAt: ($c.timestamp // $now),
			  durationSeconds: (if $startEpoch == null then null else [0, (($c.epoch // $epoch) - $startEpoch)] | max end),
			  messages: ((if $interrupted then
			    [{severity: "error", code: "INTERRUPTED", message: "Collection interrupted before this unit completed"}] + .
			    elif $rc == null then
			    [{severity: "error", code: "STATUS_RESULT_MISSING", message: "A trustworthy collector result could not be recorded"}] + .
			    elif $rc != 0 and length == 0 then
			    [{severity: "error", code: "COLLECTOR_EXIT_NONZERO", message: "Collector exited unsuccessfully; inspect its artifacts and gather log"}]
			    else . end) | .[0:10]),
			  artifacts: (if $hasStderr then [$stderr] else [] end) }
		' "${events[@]}" /dev/null >"$unit/result.json"; then
			_status_failure
			return 1
		fi
		results+=("$unit/result.json")
	done
	if [[ $_STATUS_FRAMEWORK_ERROR != 0 || -f $_STATUS_DIR/framework-error || $_STATUS_INTERRUPTED == true ]]; then overall=error; fi
	if ! jq -s --arg version "$_STATUS_VERSION" --arg started "$_STATUS_STARTED_AT" --arg completed "$now" \
		--argjson interrupted "$_STATUS_INTERRUPTED" --arg overall "$overall" '
		{schemaVersion: 1, mustGatherVersion: $version, startedAt: $started, completedAt: $completed, interrupted: $interrupted,
		 overallStatus: (if $overall == "error" then "error" elif any(.[]; .status == "error") then "degraded" else "success" end),
		 collectors: ., framework: {}}
	' "${results[@]}" /dev/null >"$_STATUS_DIR/status.tmp" || ! mv "$_STATUS_DIR/status.tmp" "$_STATUS_ROOT/status.json"; then
		_status_failure
		return 1
	fi
	[[ $overall != error ]]
}

if [[ ${BASH_SOURCE[0]} == "$0" && ${1:-} == --status-run ]]; then
	shift
	_status_run "$@"
	exit $?
fi
