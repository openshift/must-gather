#!/bin/bash

# safeguards
set -o nounset
set -o errexit
set -o pipefail

# global readonly constants
declare -r METRICS_PATH="../must-gather/monitoring/metrics"

get_first_ready_prom_pod() {
  readarray -t READY_PROM_PODS < <(
    oc get pods -n openshift-monitoring  -l prometheus=k8s --field-selector=status.phase==Running \
      --no-headers -o custom-columns=":metadata.name"
  )
  echo "${READY_PROM_PODS[0]}"
}

get_first_ready_alertmanager_pod() {
  readarray -t READY_AM_PODS < <(
    oc get pods -n openshift-monitoring  -l alertmanager=main --field-selector=status.phase==Running \
      --no-headers -o custom-columns=":metadata.name"
  )
  echo "${READY_AM_PODS[0]}"
}

metrics_get() {
  mkdir -p "${METRICS_PATH}"

  prometheus_pod=$(get_first_ready_prom_pod)
  echo "INFO: Getting metrics from ${prometheus_pod}"

  oc exec "${prometheus_pod}" \
    -c prometheus \
    -n openshift-monitoring \
    -- promtool tsdb dump-openmetrics /prometheus --sandbox-dir-root="/prometheus" "$@" \
       > "$METRICS_PATH/metrics.openmetrics" \
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
