#!/bin/bash

set -e

function get_operator_ns() {
    local operator_name=$(echo \"$1\")
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
