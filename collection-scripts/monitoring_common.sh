#!/bin/bash

# safeguards
set -o nounset
set -o errexit
set -o pipefail

# global readonly constants
declare -r METRICS_PATH="../must-gather/monitoring/metrics"

get_metrics_min_time() {
  # default to the last 2h as Prometheus servs that from memory.
  promtool_min_time=$(date --date='2 hours ago' +%s%3N)
  # takes MUST_GATHER_SINCE into account.
  # TODO: update description in "oc"
  if [ -n "${MUST_GATHER_SINCE:-}" ]; then
    promtool_min_time=$(date --date=${MUST_GATHER_SINCE} +%s%3N)
  fi
}

metrics_get() {
  mkdir -p "${METRICS_PATH}"

  readarray -t READY_PROM_PODS < <(
    oc get pods -n openshift-monitoring  -l prometheus=k8s --field-selector=status.phase==Running \
      --no-headers -o custom-columns=":metadata.name"
  )
  prometheus_pod=${READY_PROM_PODS[0]}
  echo "INFO: Getting metrics from ${prometheus_pod}"

  oc exec ${prometheus_pod} \
    -c prometheus \
    -n openshift-monitoring \
    -- promtool tsdb dump-openmetrics /prometheus "$@" \
       > "$METRICS_PATH/metrics.txt" \
       2> "$METRICS_PATH/metrics.stderr"
}

# metrics_gather dumps metrics in OpenMetrics format at $METRICS_PATH.
metrics_gather() {
  if [ $# -eq 0 ]; then
    echo "ERROR: Not setting any arguments will result in dumping all the metrics from the Prometheus instance.
This script is not meant to do that, as it may negatively impact the Prometheus instance and the client running the script.

At least one of the following arguments should be set:

--min-time: Minimum timestamp to dump in ms. Defaults to -9223372036854775808.
--max-time: Maximum timestamp to dump in ms. Defaults to 9223372036854775807.
--match: Series selector. Can be specified multiple times. Defaults to {__name__=~'(?s:.*)'}.

Please set them wisely."
    exit 1
  fi

  metrics_get "$@" || true

  # Force disk flush to ensure that all gathered data is written.
  sync
}
