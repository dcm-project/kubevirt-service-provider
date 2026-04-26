#!/usr/bin/env bash
#
# deploy-cnv.sh - Deploy OpenShift Virtualization (CNV) on an OpenShift cluster
#
# Applies OLM manifests (Namespace, OperatorGroup, Subscription), waits for
# the operator CSV to succeed, then creates the HyperConverged CR.
#
# Usage:
#   ./hack/deploy-cnv.sh
#
# Environment variables:
#   CNV_CHANNEL    - override subscription channel (default: auto-detect)
#   CSV_TIMEOUT    - seconds to wait for CSV to reach Succeeded (default: 300)
#   DEPLOY_TIMEOUT - seconds to wait for HyperConverged CR to become Available (default: 1200)
#
# Exit codes:
#   0 - Deployment completed successfully
#   1 - Error (missing prereqs, timeout)

set -euo pipefail

# Colors for output (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
fi

CSV_TIMEOUT="${CSV_TIMEOUT:-300}"
DEPLOY_TIMEOUT="${DEPLOY_TIMEOUT:-1200}"

die() { echo -e "${RED}ERROR: $*${NC}" >&2; exit 1; }
info() { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }

apply_manifest() {
    if ! oc apply -f - <<< "$1"; then
        die "Failed to apply manifest"
    fi
}

get_default_channel() {
    local package_name="$1"
    local channel
    channel=$(oc get packagemanifest "${package_name}" -o jsonpath='{.status.defaultChannel}' 2>/dev/null || echo "")
    if [[ -z "${channel}" ]]; then
        die "PackageManifest '${package_name}' not found. Is the redhat-operators CatalogSource available?"
    fi
    echo "${channel}"
}

wait_for_csv() {
    local namespace="$1"
    local subscription_name="$2"
    local elapsed=0
    local interval=10

    info "Waiting for subscription '${subscription_name}' CSV in namespace '${namespace}' (timeout: ${CSV_TIMEOUT}s)..."

    # Poll until OLM resolves a CSV name on the subscription
    local csv_name=""
    while [[ ${elapsed} -lt ${CSV_TIMEOUT} ]]; do
        csv_name=$(oc get subscription.operators.coreos.com -n "${namespace}" "${subscription_name}" -o jsonpath='{.status.currentCSV}' 2>/dev/null || true)
        if [[ -n "${csv_name}" ]]; then
            break
        fi
        warn "  No CSV resolved yet for subscription '${subscription_name}' (${elapsed}s elapsed)"
        sleep "${interval}"
        elapsed=$((elapsed + interval))
    done
    [[ -z "${csv_name}" ]] && die "Timed out after ${CSV_TIMEOUT}s waiting for subscription '${subscription_name}' to resolve a CSV"

    local remaining=$(( CSV_TIMEOUT - elapsed ))
    info "CSV '${csv_name}' resolved — waiting for Succeeded phase (${remaining}s remaining)..."
    if ! oc wait csv/"${csv_name}" -n "${namespace}" --for=jsonpath='{.status.phase}'=Succeeded --timeout="${remaining}s" 2>/dev/null; then
        local phase
        phase=$(oc get csv -n "${namespace}" "${csv_name}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        local message
        message=$(oc get csv -n "${namespace}" "${csv_name}" -o jsonpath='{.status.message}' 2>/dev/null || echo "")
        die "CSV '${csv_name}' did not reach Succeeded (phase: ${phase}): ${message:-timeout}"
    fi
    info "CSV '${csv_name}' reached Succeeded phase"
}

deploy_cnv() {
    local namespace="openshift-cnv"
    local channel="${CNV_CHANNEL:-}"

    # Skip if CNV is already installed and ready
    local current_status
    current_status=$(oc get hyperconverged kubevirt-hyperconverged -n "${namespace}" -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "")
    if [[ "${current_status}" == "True" ]]; then
        info "OpenShift Virtualization is already installed and Available — skipping"
        return 0
    fi

    info "Deploying OpenShift Virtualization (CNV)..."

    if [[ -z "${channel}" ]]; then
        info "Detecting default channel from PackageManifest..."
        channel=$(get_default_channel "kubevirt-hyperconverged")
    fi
    info "Using channel: ${channel}"

    info "Creating namespace '${namespace}'..."
    apply_manifest "
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
"

    # Only create OperatorGroup if none exists (OLM forbids multiple per namespace)
    local existing_og
    existing_og=$(oc get operatorgroup -n "${namespace}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [[ -z "${existing_og}" ]]; then
        info "Creating OperatorGroup..."
        apply_manifest "
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kubevirt-hyperconverged-group
  namespace: ${namespace}
spec: {}
"
    else
        info "OperatorGroup '${existing_og}' already exists — skipping"
    fi

    # Only create Subscription if none exists for this package
    local existing_sub
    existing_sub=$(oc get subscription.operators.coreos.com -n "${namespace}" -o jsonpath='{.items[?(@.spec.name=="kubevirt-hyperconverged")].metadata.name}' 2>/dev/null || echo "")
    if [[ -z "${existing_sub}" ]]; then
        info "Creating Subscription..."
        apply_manifest "
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kubevirt-hyperconverged
  namespace: ${namespace}
spec:
  channel: ${channel}
  installPlanApproval: Automatic
  name: kubevirt-hyperconverged
  source: redhat-operators
  sourceNamespace: openshift-marketplace
"
        existing_sub="kubevirt-hyperconverged"
    else
        info "Subscription '${existing_sub}' already exists — skipping"
    fi

    wait_for_csv "${namespace}" "${existing_sub}"

    info "Creating HyperConverged CR (minimal profile — dev/test only)..."
    apply_manifest "
apiVersion: hco.kubevirt.io/v1beta1
kind: HyperConverged
metadata:
  name: kubevirt-hyperconverged
  namespace: ${namespace}
  annotations:
    deployOVS: 'false'
    networkaddonsconfigs.kubevirt.io/jsonpatch: |
      [
        {\"op\": \"replace\", \"path\": \"/spec/kubeMacPool\", \"value\": null},
        {\"op\": \"replace\", \"path\": \"/spec/ovs\", \"value\": null},
        {\"op\": \"replace\", \"path\": \"/spec/macvtap\", \"value\": null},
        {\"op\": \"replace\", \"path\": \"/spec/kubeSecondaryDNS\", \"value\": null},
        {\"op\": \"replace\", \"path\": \"/spec/kubevirtIpamController\", \"value\": null}
      ]
spec:
  enableCommonBootImageImport: false
  deployVmConsoleProxy: false
  enableApplicationAwareQuota: false
  featureGates:
    deployKubeSecondaryDNS: false
    disableMDevConfiguration: true
"

    info "Waiting for HyperConverged to become Available (timeout: ${DEPLOY_TIMEOUT}s)..."
    if ! oc wait hyperconverged/kubevirt-hyperconverged -n "${namespace}" --for=condition=Available --timeout="${DEPLOY_TIMEOUT}s" 2>/dev/null; then
        die "Timed out after ${DEPLOY_TIMEOUT}s waiting for HyperConverged to become Available"
    fi
    info "HyperConverged is Available"

    info ""
    info "========================================"
    info "  OpenShift Virtualization deployed successfully"
    info "========================================"
}

# --- Main ---

if ! command -v oc &>/dev/null; then
    die "'oc' CLI not found. Install it from https://console.redhat.com/openshift/downloads"
fi

if ! oc whoami --show-server &>/dev/null; then
    die "Cannot reach OpenShift cluster. Log in with 'oc login' first."
fi

deploy_cnv
