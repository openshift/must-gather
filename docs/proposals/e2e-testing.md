# Proposal: E2E Testing for must-gather Collection Scripts

**Status:** Draft
**Authors:** TBD
**Reviewers:** TBD
**Creation Date:** 2026-02-17
**Last Updated:** 2026-02-19

---

## Summary

Add a CI job that runs `oc adm must-gather` on a real OCP cluster and validates the output directory structure. Today, must-gather runs in CI only during artifact collection as a best-effort step with no output validation. If a collection script silently produces empty files, generates malformed JSON, or crashes, nobody catches it. This proposal introduces structural validation (file existence, non-emptiness, format checks) across 3 CI jobs in 5 implementation phases that progressively cover 24 of 32 collection scripts.

## Motivation

### User Stories

- As a **must-gather developer**, I want CI to tell me if my change broke an existing collection script, so that I don't merge regressions.
- As a **support engineer**, I want confidence that the must-gather image collects all expected data, so that I don't receive incomplete diagnostic bundles from customers.
- As a **CI maintainer**, I want must-gather validation to reuse existing Prow infrastructure (IPI workflows, cluster profiles, install-operators step), so that I don't need to build new CI tooling.

### Goals

1. Validate that always-present scripts (etcd, monitoring, network, OLM, etc.) produce non-empty, correctly formatted output on a real cluster.
2. Validate that conditional scripts (metallb, nmstate, sriov, etc.) exit cleanly when their operator is absent, and produce real output when present.
3. Validate that the `gather` orchestrator completes within a timeout without hanging.
4. Clean up any cluster resources leaked by scripts that create DaemonSets or pods.
5. Fit into the existing Prow CI infrastructure with no new tooling.

### Non-Goals

- Golden input/output comparison. Cluster state is non-deterministic (pod names, timestamps, IPs, log content all vary between runs). Golden files would be stale within a week.
- Content correctness validation (e.g., "does member_list.json contain the right etcd members?"). Add this incrementally, driven by real bugs.
- Full coverage of all 32 scripts on day one. Platform-specific scripts (vSphere, ARO, Windows) and hardware-dependent scripts (SR-IOV) are covered in later phases or explicitly deferred.

## Proposal

### Approach: Structural Validation

Must-gather output is non-deterministic but structurally predictable. For any healthy OCP cluster, `gather_etcd` should always produce `etcd_info/member_list.json` as valid JSON. The test asserts structure, not content:

```
Level 1: Directory/file exists      →  test -d / test -f
Level 2: File is non-empty          →  test -s
Level 3: File has valid format      →  jq empty (for JSON), gzip -t (for gzip)
```

The test adapts to each cluster by first discovering what operators/platforms are present, then running only the applicable validators.

### Two Invocation Groups

The `gather` orchestrator calls 23 sub-scripts. Eight more are separate entry points requiring explicit invocation:

- **Default** (`oc adm must-gather`): 23 sub-scripts run in parallel, ~15-25 min
- **Separate** (`oc adm must-gather -- /usr/bin/<script>`): gather_audit_logs, gather_apirequestcounts, gather_etcd_more, gather_metrics, gather_ingress_node_firewall, gather_core_dumps, gather_profiling_node, gather_network_logs (~10-15 min each)

Note: `gather_ingress_node_firewall` is present in the must-gather image but is not invoked by the default `gather` orchestrator. It must be called explicitly as a separate entry point.

Note: `gather_network_logs` internally calls `/usr/bin/gather` (the full orchestrator) plus extras. It should not be run alongside the default gather.

### Honest Coverage Analysis

On a **vanilla AWS cluster** (no extra operators):

| Category | Scripts | E2E coverage | New value beyond UTs? |
|----------|---------|-------------|----------------------|
| Always-present, meaningful output (default) | 12 | Structure, format, non-emptiness | **Yes** |
| Always-present, likely empty on healthy cluster | 2 | No-crash only | Marginal |
| Conditional, self-skipping in default gather | 10 | Early-exit path | **No** -- same as BATS UTs |
| Separate entry points (Phase 3) | 4 | Structure, format | **Yes** |
| Separate entry points (Phase 4, with operators) | 1 | Structure, format (gather_ingress_node_firewall) | **Yes** |
| Separate entry points (deferred) | 3 | Not tested (core_dumps, profiling_node, network_logs) | No |

**Day-one (Phase 1-3):** 18 scripts tested (14 from default gather + 4 separate entry points).
**With operator job (Phase 4):** 23 scripts (adds metallb, nmstate, frrk8s, olm_v1 in default gather + gather_ingress_node_firewall as separate entry point).
**With vSphere job (Phase 5):** 24 scripts.
**Remaining 8:** gather_sriov (hardware), gather_osus (no operator installed), gather_istio (no operator installed), gather_aro (ARO only), gather_windows_node_logs (Windows only), gather_core_dumps (deferred), gather_profiling_node (deferred), gather_network_logs (deferred).

### CI Integration Strategy

**Precedent:** [kubevirt/must-gather](https://github.com/openshift/release/blob/master/ci-operator/config/kubevirt/must-gather/kubevirt-must-gather-main.yaml) already validates must-gather output in Prow CI. They provision Azure/GCP clusters, install an operator, run must-gather, and validate output. We follow the same model.

**Key CI primitives used:**
- `ipi-aws` / `ipi-vsphere` workflows for cluster provisioning (already used by existing `e2e-aws` job)
- [install-operators](https://github.com/openshift/release/blob/master/ci-operator/step-registry/install-operators/install-operators-ref.yaml) step for pre-installing operators via OperatorHub JSON (used by dozens of projects: stolostron, rhobs, kubevirt)
- `dependencies` block to inject the PR's CI-built must-gather image
- `vsphere-elastic` profile for vSphere platform testing

**Three CI jobs (added progressively):**

| Job | Cluster | Pre-steps | Runs on | Coverage |
|-----|---------|-----------|---------|----------|
| `e2e-must-gather-validate` | AWS (vanilla) | None | Every PR | 18 scripts (always-present + separate entries) |
| `e2e-must-gather-validate-operators` | AWS + operators | `install-operators` (MetalLB, NMState, Ingress NF) | Optional / nightly | +5 scripts (4 conditional in default gather + gather_ingress_node_firewall as separate invocation) |
| `e2e-must-gather-validate-vsphere` | vSphere | None | Weekly cron | +1 (gather_vsphere) |

### Phased Rollout

| Phase | What | Coverage | Success criteria |
|-------|------|----------|-----------------|
| 1 | Add `test/e2e/` scripts + `make test-e2e`, validate locally | 14 scripts | All validators pass on a dev cluster; no false failures on 3 consecutive runs |
| 2 | Add Job 1 to ci-operator config; mark `optional: true` | 14 scripts in CI | 2 weeks of green runs, then promote to required |
| 3 | Add separate entry points (audit_logs, apirequestcounts, etcd_more, metrics) | 18 scripts | Separate invocations complete within 15 min each |
| 4 | Add Job 2 with `install-operators` pre-step + gather_ingress_node_firewall as separate invocation | 23 scripts | Conditional validators assert real output, not just skip |
| 5 | Add Job 3 on vSphere weekly cron | 24 scripts | gather_vsphere produces non-empty CSI CRD data |

### Scripts That Create Cluster Resources (Teardown)

| Script | Resources created | If interrupted |
|--------|------------------|----------------|
| `gather_ppc` (default) | DaemonSet `perf-node-gather-daemonset` | Leaked DS in must-gather namespace |
| `gather_core_dumps` (separate, deferred) | `oc debug` pods in `default` ns | Leaked `node-debugger-*` pods |
| `gather_profiling_node` (separate, deferred) | Pods with label `name=gather-profiling-crio` | Leaked profiling pods |

The e2e test registers a `trap` on EXIT/INT/TERM that cleans up all three.

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| **Golden input/output comparison** | Cluster state is non-deterministic. Pod names, timestamps, IPs, metrics all change between runs. Golden files would require constant updates and give false confidence. |
| **Go test binary (like kubevirt/must-gather)** | The project is entirely bash with an empty go.mod (only build-machinery-go). The validation logic is `test -s` and `jq empty`. Go would add ~200 client-go dependencies to shell out to the same `oc` commands. Shell is proportional to the project. |
| **Validate the existing `gather-must-gather` artifact** | The existing gather step in the `e2e-aws` post phase uses the released image, not the PR's image. It also runs best-effort and may not complete. A dedicated job with the PR's image is more reliable. |
| **Parameterized BATS tests instead of custom shell** | BATS discovers `@test` blocks at parse time, not runtime. You can't dynamically generate tests from cluster state. A plain shell script with `assert_*` helpers is simpler for adaptive validation. |

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **CI flakes from cluster instability** | Medium | Spurious failures block PRs | Cluster readiness check (`oc wait clusteroperators`); mark job `optional: true` initially; retry logic on timeout |
| **Resource cost of additional AWS cluster** | Medium | Increased CI spend | Job 1 uses the same `aws` profile as existing `e2e-aws`; Jobs 2-3 are optional/weekly |
| **Maintenance burden of validators** | Low | Validators break when scripts change output paths | Each validator is 5-15 lines. Output paths are stable. Update validators alongside script changes in the same PR. |
| **`jq` not available in src image** | Certain | JSON validation fails | Install `jq` from GitHub releases at runtime (2 lines); the `install-operators` step already uses the `cli-jq` image which has it |
| **`gather_vsphere` uses `/usr/bin/oc` (absolute path)** | Certain | Happy path untestable via UT | Addressed by e2e Job 3 on real vSphere cluster |

## Infrastructure Needed

- **Job 1:** AWS cluster profile `aws` (already in use by `e2e-aws`)
- **Job 2:** Same AWS profile + `install-operators` step (no new infra)
- **Job 3:** vSphere cluster profile `vsphere-elastic` (already available in CI step registry)
- **ci-operator config change:** In `openshift/release` repo for `openshift-must-gather-main.yaml`
- **No new step registry entries.** All steps (ipi-aws, ipi-vsphere, install-operators) already exist.

## Open Questions

1. Should Job 1 (`e2e-must-gather-validate`) run on every PR or only when `collection-scripts/` files change? Running on every PR catches oc/cluster-side regressions but costs an AWS cluster per PR.
2. For Job 2, which operators should be included? Starting with MetalLB + NMState + Ingress Node Firewall covers 5 additional scripts (metallb, nmstate, frrk8s, olm_v1 on 4.16+ in default gather, plus gather_ingress_node_firewall as separate invocation). Adding Istio/Sail would cover 1 more but installation is heavier. Adding OSUS would cover 1 more.
3. Should we produce JUnit XML output for Prow test result integration, or is plain PASS/FAIL logging sufficient for the initial phases?

---

## Appendix A: Complete Script Classification

### Always present -- meaningful output (14 scripts)

| Script | Invoked by | Expected output | Side effects |
|--------|-----------|----------------|-------------|
| `gather` (orchestrator) | default | `must-gather/version` containing "openshift/must-gather" | None |
| `gather_etcd` | default | `etcd_info/member_list.json`, `endpoint_status.json`, `endpoint_health.json`, `alarm_list.json`, `object_count.json` -- valid JSON | None |
| `gather_monitoring` | default | `monitoring/` directory with alerts, rules, targets | None |
| `gather_service_logs` | default | `host_service_logs/masters/*.log` | None |
| `gather_haproxy_config` | default | haproxy config files under ingress paths | None |
| `gather_kas_startup_termination_logs` | default | gzipped kube-apiserver log files | None |
| `gather_network_logs_basics` | default | `network_logs/` directory tree | None |
| `gather_olm` | default | OLM namespace data | None |
| `gather_podnetworkconnectivitycheck` | default | JSON files in network diagnostics path | None |
| `gather_ppc` | default | `nodes/<node>/` with hardware data | Creates + deletes DaemonSet |
| `gather_priority_and_fairness` | default | APF debug endpoint data | None |
| `gather_insights` | default | `insights-data/` directory | None |
| `gather_audit_logs` | **separate** | gzipped audit log files | None |
| `gather_apirequestcounts` | **separate** | `requests/apirequestscounts.json`, top-20 files | None |

### Always present -- likely empty on healthy cluster (2 scripts)

| Script | Invoked by | Why likely empty |
|--------|-----------|-----------------|
| `gather_machineconfig_ondisk` | default | Only collects from degraded nodes |
| `gather_machineconfigdaemon_termination_logs` | default | Needs MCD pod restart for previous logs |

### Conditional -- self-skipping, called by default gather (10 scripts)

| Script | Detection method | Operator needed |
|--------|-----------------|----------------|
| `gather_metallb` | `get_operator_ns "metallb-operator"` | MetalLB Operator |
| `gather_nmstate` | `get_operator_ns "kubernetes-nmstate-operator"` | NMState Operator |
| `gather_sriov` | `get_operator_ns "sriov-network-operator"` | SR-IOV Operator (needs NICs) |
| `gather_frrk8s` | `oc get ns openshift-frr-k8s` | FRR-K8s (with MetalLB) |
| `gather_osus` | `oc get csv -A \| awk` | OSUS Operator |
| `gather_olm_v1` | `oc get crds \| grep olm.operatorframework.io` | OLM v1 (OCP 4.16+) |
| `gather_istio` | pods + CRDs checks | Sail/Istio Operator |
| `gather_vsphere` | `/usr/bin/oc get clustercsidriver` | vSphere platform |
| `gather_aro` | `oc get clusters.aro.openshift.io` | ARO cluster |
| `gather_windows_node_logs` | `oc get no -l kubernetes.io/os=windows` | Windows nodes |

### Separate entry points (6 scripts)

| Script | Expected output | Side effects | Notes |
|--------|----------------|-------------|-------|
| `gather_ingress_node_firewall` | Ingress NF CRDs and namespace data | None | Conditional: self-skips if operator absent. Present in image but **not called by default gather**. |
| `gather_etcd_more` | etcd metrics, interface stats, pprof | None | |
| `gather_metrics` | `monitoring/metrics/metrics.openmetrics` | None | |
| `gather_network_logs` | Full gather + extra OVN logs | Runs full gather internally | Do not run alongside default gather |
| `gather_core_dumps` | Core dumps from nodes | Creates debug pods | |
| `gather_profiling_node` | kubelet/CRI-O pprof data | Creates profiling pods | |

---

## Appendix B: Implementation Details

### Language choice

Shell. The project is entirely bash. The validation is `test -s` and `jq empty`. Go would add dependency overhead for zero gain.

### File layout

```
test/
  e2e/
    must_gather_e2e.sh          # Main entry point
    lib/
      assertions.sh             # assert_dir_exists, assert_file_not_empty, assert_valid_json
      discovery.sh              # Probe cluster state
      teardown.sh               # Cleanup leaked resources
    validators/
      core.sh                   # Always-present validators
      conditional.sh            # Operator-conditional validators
      separate_entries.sh       # Separate entry point validators
```

### Assertion helpers

```bash
PASSES=0; FAILURES=0

assert_dir_exists() {
    if [[ ! -d "$1" ]]; then
        echo "FAIL: directory not found: $1"; FAILURES=$((FAILURES + 1))
    else
        echo "PASS: $1"; PASSES=$((PASSES + 1))
    fi
}

assert_file_not_empty() {
    if [[ ! -s "$1" ]]; then
        echo "FAIL: file missing or empty: $1"; FAILURES=$((FAILURES + 1))
    else
        echo "PASS: $1 ($(wc -c < "$1") bytes)"; PASSES=$((PASSES + 1))
    fi
}

assert_valid_json() {
    if ! jq empty "$1" 2>/dev/null; then
        echo "FAIL: invalid JSON: $1"; FAILURES=$((FAILURES + 1))
    else
        echo "PASS: valid JSON $1"; PASSES=$((PASSES + 1))
    fi
}
```

### Example validator

```bash
validate_etcd() {
    local base="$1"
    assert_dir_exists "${base}/etcd_info"
    for f in member_list.json endpoint_status.json endpoint_health.json \
             alarm_list.json object_count.json; do
        assert_file_not_empty "${base}/etcd_info/${f}"
        assert_valid_json "${base}/etcd_info/${f}"
    done
}
```

### Teardown

```bash
teardown() {
    oc delete ds perf-node-gather-daemonset --all-namespaces --ignore-not-found=true 2>/dev/null || true
    for pod in $(oc get pods -n default -o name 2>/dev/null | grep node-debugger); do
        oc delete "$pod" -n default --ignore-not-found=true 2>/dev/null || true
    done
    oc delete pods -l name=gather-profiling-crio --all-namespaces --ignore-not-found=true 2>/dev/null || true
}
trap teardown EXIT INT TERM
```

---

## Appendix C: CI Job Configurations

### Job 1: `e2e-must-gather-validate` (vanilla AWS, every PR)

```yaml
- as: e2e-must-gather-validate
  steps:
    cluster_profile: aws
    test:
    - as: validate
      cli: latest
      commands: |
        curl -sL -o /tmp/jq https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64
        chmod +x /tmp/jq
        export PATH="/tmp:${PATH}"
        ./test/e2e/must_gather_e2e.sh "${MUST_GATHER_IMAGE}"
      dependencies:
      - env: MUST_GATHER_IMAGE
        name: must-gather
      from: src
      resources:
        requests:
          cpu: 100m
          memory: 256Mi
    workflow: ipi-aws
```

### Job 2: `e2e-must-gather-validate-operators` (AWS + operators, optional)

```yaml
- as: e2e-must-gather-validate-operators
  optional: true
  steps:
    cluster_profile: aws
    env:
      OPERATORS: |
        [
          {"name": "metallb-operator", "source": "redhat-operators", "channel": "stable",
           "install_namespace": "openshift-metallb-system",
           "target_namespaces": "openshift-metallb-system",
           "operator_group": "metallb-og"},
          {"name": "kubernetes-nmstate-operator", "source": "redhat-operators", "channel": "stable",
           "install_namespace": "openshift-nmstate",
           "target_namespaces": "openshift-nmstate",
           "operator_group": "nmstate-og"},
          {"name": "ingress-node-firewall-operator", "source": "redhat-operators", "channel": "stable",
           "install_namespace": "openshift-ingress-node-firewall",
           "target_namespaces": "openshift-ingress-node-firewall",
           "operator_group": "ingress-nf-og"}
        ]
    test:
    - ref: install-operators
    - as: validate
      cli: latest
      commands: |
        curl -sL -o /tmp/jq https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64
        chmod +x /tmp/jq
        export PATH="/tmp:${PATH}"
        # Default gather (tests conditional scripts with operators present)
        ./test/e2e/must_gather_e2e.sh "${MUST_GATHER_IMAGE}"
        # gather_ingress_node_firewall is not called by default gather;
        # invoke it explicitly now that its operator is installed
        ./test/e2e/must_gather_e2e.sh --separate=gather_ingress_node_firewall "${MUST_GATHER_IMAGE}"
      dependencies:
      - env: MUST_GATHER_IMAGE
        name: must-gather
      from: src
      resources:
        requests:
          cpu: 100m
          memory: 256Mi
    workflow: ipi-aws
```

### Job 3: `e2e-must-gather-validate-vsphere` (vSphere, weekly cron)

```yaml
- as: e2e-must-gather-validate-vsphere
  optional: true
  cron: 0 6 * * 1
  steps:
    cluster_profile: vsphere-elastic
    test:
    - as: validate
      cli: latest
      commands: |
        curl -sL -o /tmp/jq https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64
        chmod +x /tmp/jq
        export PATH="/tmp:${PATH}"
        ./test/e2e/must_gather_e2e.sh "${MUST_GATHER_IMAGE}"
      dependencies:
      - env: MUST_GATHER_IMAGE
        name: must-gather
      from: src
      resources:
        requests:
          cpu: 100m
          memory: 256Mi
    workflow: ipi-vsphere
```

### Prow flow diagram

```
PR opened to openshift/must-gather
  |
  +- unit                                    (BATS, no cluster)
  +- images                                  (build must-gather image)
  +- e2e-aws                                 (existing, unchanged)
  +- e2e-must-gather-validate                (NEW: vanilla AWS, always runs)
  |    +- ipi-aws-pre -> test/validate -> ipi-aws-post
  +- e2e-must-gather-validate-operators      (NEW: AWS + operators, optional)
  |    +- ipi-aws-pre -> install-operators -> test/validate -> ipi-aws-post
  +- e2e-must-gather-validate-vsphere        (NEW: vSphere, weekly cron)
       +- ipi-vsphere-pre -> test/validate -> ipi-vsphere-post
```

### Makefile targets

```makefile
.PHONY: test-e2e
test-e2e:
	@test -n "$(MUST_GATHER_IMAGE)" || (echo "ERROR: set MUST_GATHER_IMAGE"; exit 1)
	./test/e2e/must_gather_e2e.sh "$(MUST_GATHER_IMAGE)"

.PHONY: test-e2e-full
test-e2e-full:
	@test -n "$(MUST_GATHER_IMAGE)" || (echo "ERROR: set MUST_GATHER_IMAGE"; exit 1)
	./test/e2e/must_gather_e2e.sh --full "$(MUST_GATHER_IMAGE)"
```
