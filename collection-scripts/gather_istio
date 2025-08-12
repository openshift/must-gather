#!/bin/bash
# Copyright 2025 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


BASE_COLLECTION_PATH="/must-gather"

# make sure we honor --since and --since-time args passed to oc must-gather
# since OCP 4.16
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
}

# Get Istio, sail operator, kiali, gateway.networking.k8s.io and inference.networking.x-k8s.io group CRDs
function getCRDs() {
  local result=()
  local output
  output=$(oc get crds -o custom-columns=NAME:metadata.name --no-headers | grep -e '\.istio\.io' -e '\.sailoperator\.io' -e '\.kiali\.io' -e '\.gateway\.networking\.k8s\.io' -e 'inference\.networking\.x-k8s\.io')
  for crd in ${output}; do
    result+=("${crd}")
  done

  echo "${result[@]}"
}

# getSynchronization dumps the synchronization status for the specified control plane revision
# to a file in the control plane revision directory of the control plane namespace
# Arguments:
#   namespace of the control plane
#   revision of the control plane
# Returns:
#   nothing
function getSynchronization() {
  local namespace="${1}"
  local revision="${2}"

  local istiodName
  istiodName=$(oc get pod -n "${namespace}" -l "app=istiod,istio.io/rev=${revision}" -o jsonpath="{.items[0].metadata.name}")

  echo
  echo "Collecting /debug/syncz from ${istiodName} in namespace ${namespace}"

  local logPath=${BASE_COLLECTION_PATH}/namespaces/${namespace}/${revision}
  mkdir -p "${logPath}"
  oc exec "${istiodName}" -n "${namespace}" -c discovery -- /usr/local/bin/pilot-discovery request GET /debug/syncz > "${logPath}/debug-syncz.json" 2>&1
}

# getEnvoyConfigForPodsInNamespace dumps the envoy config for the specified namespace and
# control plane revision to a file in the must-gather directory for each pod
# Arguments:
#   namespace of the control plane
#   revision of the control plane
#   namespace to dump
# Returns:
#   nothing
function getEnvoyConfigForPodsInNamespace() {
  local controlPlaneNamespace="${1}"
  local revisionName="${2}"
  local podNamespace="${3}"

  local istiodName
  istiodName=$(oc get pod -n "${controlPlaneNamespace}" -l "app=istiod,istio.io/rev=${revisionName}" -o jsonpath="{.items[0].metadata.name}")

  echo
  echo "Collecting Envoy config for pods in ${podNamespace} pointing to ${revisionName} revision"

  local pods
  pods="$(oc get pods -n "${podNamespace}" -o jsonpath='{ .items[*].metadata.name }')"
  for podName in ${pods}; do
    if oc get pod -o yaml "${podName}" -n "${podNamespace}" | grep -q proxyv2; then
      echo "Collecting config_dump and stats for pod ${podName}.${podNamespace}"

      local logPath=${BASE_COLLECTION_PATH}/namespaces/${podNamespace}/pods/${podName}
      mkdir -p "${logPath}"

      oc exec "${istiodName}" -n "${controlPlaneNamespace}" -c discovery -- bash -c "/usr/local/bin/pilot-discovery request GET /debug/config_dump?proxyID=${podName}.${podNamespace}" > "${logPath}/config_dump_istiod.json" 2>&1
      oc exec -n "${podNamespace}" "${podName}" -c istio-proxy -- /usr/local/bin/pilot-agent request GET config_dump > "${logPath}/config_dump_proxy.json" 2>&1
      oc exec -n "${podNamespace}" "${podName}" -c istio-proxy -- /usr/local/bin/pilot-agent request GET stats > "${logPath}/proxy_stats" 2>&1
    fi
  done
}

function version() {
  if [[ -n $OSSM_MUST_GATHER_VERSION ]] ; then
    echo "${OSSM_MUST_GATHER_VERSION}"
  else
    echo "0.0.0-unknown"
  fi
}

# Inspect given resource in given namespace (optional)
# It's using 'oc adm inspect' which will get debug information for given and
# related resources. Including pod logs.
# Since OCP 4.16 it honors '--since' and '--since-time' args
function inspect() {
  local resource ns
  resource=$1
  ns=$2

  echo
  if [ -n "$ns" ]; then
    echo "Inspecting resource ${resource} in namespace ${ns}"
    # it's here just to make the linter happy (we have to use double quotes arround the variable)
    if [ -n "${log_collection_args}" ]
    then
      oc adm inspect "${log_collection_args}" "--dest-dir=${BASE_COLLECTION_PATH}" "${resource}" -n "${ns}"
    else
      oc adm inspect "--dest-dir=${BASE_COLLECTION_PATH}" "${resource}" -n "${ns}"
    fi
  else
    echo "Inspecting resource ${resource}"
    # it's here just to make the linter happy
    if [ -n "${log_collection_args}" ]
    then
      oc adm inspect "${log_collection_args}" "--dest-dir=${BASE_COLLECTION_PATH}" "${resource}"
    else
      oc adm inspect "--dest-dir=${BASE_COLLECTION_PATH}" "${resource}"
    fi
  fi
}

function inspectNamespace() {
  local ns
  ns=$1

  inspect "ns/$ns"
  for crd in $crds; do
    inspect "$crd" "$ns"
  done
  inspect net-attach-def,roles,rolebindings "$ns"
}

function main() {
  local crds
  echo
  echo "Executing Istio gather script"
  echo

  versionFile="${BASE_COLLECTION_PATH}/version"
  echo "openshift-service-mesh/must-gather"> "$versionFile"
  version >> "$versionFile"

  # set global variable which is used when calling 'oc adm inspect'
  get_log_collection_args

  operatorNamespace=$(oc get pods --all-namespaces -l app.kubernetes.io/created-by=servicemeshoperator3 -o jsonpath="{.items[0].metadata.namespace}")
  # this gets also logs for all pods in that namespace
  inspect "ns/$operatorNamespace"
  inspect clusterserviceversion "${operatorNamespace}"

  inspect nodes

  # TODO: we have to add a new label in the helm postrenderer section in the operator the same way like 'metadata.ownerReferences' is added
  # 'install.operator.istio.io/owning-resource' should not be used as it could interfere with istioctl
  # using 'metadata.ownerReferences' is not a good solution, we should use the new label when ready
  # this needs to be revisited when working on https://issues.redhat.com/browse/OSSM-6804
  # Following label could be used - https://github.com/istio-ecosystem/sail-operator/pull/783
  for r in $(oc get clusterroles,clusterrolebindings -l install.operator.istio.io/owning-resource -oname); do
    inspect "$r"
  done
  for r in $(oc get clusterroles,clusterrolebindings -l 'app in (istiod,istio-reader)' -oname); do
    inspect "$r"
  done

  # inspect all istio, sail operator, kiali and gateway API+ CRDs
  # this will also collect instances of those CRDs
  crds="$(getCRDs)"
  for crd in ${crds}; do
    inspect "crd/${crd}"
  done

  # inspect all controlled mutatingwebhookconfiguration
  for mwc in $(oc get mutatingwebhookconfiguration -l app=sidecar-injector -o name);do
    inspect "${mwc}"
  done
  # inspect all controlled validatingwebhookconfiguration
  for vwc in $(oc get validatingwebhookconfiguration -l app=istiod -o name);do
    inspect "${vwc}"
  done

  # this will just store all CRs as those are cluster-scoped resources
  inspect "Istio"
  inspect "IstioRevision"
  inspect "IstioCNI"
  inspect "IstioRevisionTag"
  inspect "ZTunnel"

  istioCniNamespace=$(oc get IstioCNI -A -o jsonpath="{.items[0].spec.namespace}")
  if [ -n "$istioCniNamespace" ]
  then
    inspectNamespace "${istioCniNamespace}"
  fi

  # iterate over all Istio revisions
  for ir in $(oc get IstioRevision -o jsonpath="{.items[*].metadata.name}"); do
    echo
    echo "Inspecting ${ir} IstioRevision"
    cpNamespace=$(oc get IstioRevision "${ir}" -o jsonpath="{.spec.namespace}")
    inspectNamespace "$cpNamespace"

    getSynchronization "${cpNamespace}" "${ir}"

    # iterate over all namespaces which have pods with proxy pointing to this control plane revision
    for dpn in $(oc get pods -A -o=jsonpath="{.items[?(@.metadata.annotations.istio\.io/rev==\"${ir}\")].metadata.namespace}" | tr ' ' '\n' | sort -u); do
      echo
      echo "Inspecting ${dpn} data plane namespace"
      inspectNamespace "${dpn}"
      getEnvoyConfigForPodsInNamespace "${cpNamespace}" "${ir}" "${dpn}"
    done
  done

  # inspect Kiali if it's installed
  if oc get crd kialis.kiali.io &> /dev/null; then
    # Inspect all Kialis
    for kialiNS in $(oc get Kiali -A -o jsonpath="{.items[*].metadata.namespace}"); do
      echo
      echo "Inspecting Kialis in ${kialiNS} namespace"
      inspect "Kiali" "${kialiNS}"
      inspectNamespace "$kialiNS"
    done

    # Inspect all ossmconsoles
    for consoleNS in $(oc get ossmconsole -A -o jsonpath="{.items[*].metadata.namespace}"); do
      echo
      echo "Inspecting ossmconsoles in ${consoleNS} namespace"
      inspect "ossmconsole" "${consoleNS}"
      inspectNamespace "$consoleNS"
    done
  fi

echo
echo
echo "Done"
echo
}

main "$@"
