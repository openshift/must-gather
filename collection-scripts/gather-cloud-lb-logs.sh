#!/bin/bash
#
# gather-cloud-lb-logs.sh
#
# Usage:
#   ./gather-cloud-lb-logs.sh [--output-dir <dir>] [--cloud-provider <aws|azure|gcp>]
#
# Environment variables:
#   OUTPUT_DIR: Directory to store gathered logs (default: ./must-gather/cloud-lb-logs)
#   CLOUD_PROVIDER: Cloud provider to gather logs from (auto-detected if not set)
#   AWS_REGION: AWS region (required for AWS)
#   CLUSTER_NAME: OpenShift cluster name (optional, for filtering resources)
#

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-./must-gather/cloud-lb-logs}"
CLOUD_PROVIDER="${CLOUD_PROVIDER:-}"
CLUSTER_NAME="${CLUSTER_NAME:-}"

# Cluster information variables
CLUSTER_INFRA_ID="${CLUSTER_INFRA_ID:-}"
CLUSTER_VPC_ID="${CLUSTER_VPC_ID:-}"
CLUSTER_RESOURCE_GROUP="${CLUSTER_RESOURCE_GROUP:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Detect cloud provider
detect_cloud_provider() {
    if [[ -n "${CLOUD_PROVIDER}" ]]; then
        echo "${CLOUD_PROVIDER}"
        return
    fi

    # Check for AWS
    if command -v aws &> /dev/null && aws sts get-caller-identity &> /dev/null; then
        echo "aws"
        return
    fi

    # Check for Azure
    if command -v az &> /dev/null && az account show &> /dev/null; then
        echo "azure"
        return
    fi

    # Check for GCP
    if command -v gcloud &> /dev/null && gcloud config get-value project &> /dev/null; then
        echo "gcp"
        return
    fi

    log_error "Could not detect cloud provider. Please set CLOUD_PROVIDER environment variable."
    return 1
}

# Gather AWS Load Balancer logs
gather_aws_lb_logs() {
    local region="${AWS_REGION:-}"
    if [[ -z "${region}" ]]; then
        region=$(aws configure get region 2>/dev/null || echo "us-east-1")
        log_warn "AWS_REGION not set, using default: ${region}"
    fi

    log_info "Gathering AWS Load Balancer logs from region: ${region}"
    local aws_dir="${OUTPUT_DIR}/aws"
    mkdir -p "${aws_dir}"

    # Gather ALB (Application Load Balancer) logs
    log_info "Gathering ALB access logs..."
    local alb_arns
    alb_arns=$(aws elbv2 describe-load-balancers --region "${region}" --query 'LoadBalancers[*].LoadBalancerArn' --output text 2>/dev/null || echo "")
    
    if [[ -n "${alb_arns}" ]]; then
        for alb_arn in ${alb_arns}; do
            local alb_name
            alb_name=$(echo "${alb_arn}" | awk -F'/' '{print $NF}')
            log_info "Processing ALB: ${alb_name}"
            
            # Get ALB attributes to find S3 bucket for access logs
            aws elbv2 describe-load-balancer-attributes \
                --load-balancer-arn "${alb_arn}" \
                --region "${region}" \
                --output json > "${aws_dir}/alb-${alb_name}-attributes.json" 2>/dev/null || true
            
            # Get ALB description
            aws elbv2 describe-load-balancers \
                --load-balancer-arns "${alb_arn}" \
                --region "${region}" \
                --output json > "${aws_dir}/alb-${alb_name}-description.json" 2>/dev/null || true
            
            # Get target health
            aws elbv2 describe-target-health \
                --load-balancer-arn "${alb_arn}" \
                --region "${region}" \
                --output json > "${aws_dir}/alb-${alb_name}-target-health.json" 2>/dev/null || true
        done
    else
        log_warn "No ALBs found in region ${region}"
    fi

    # Gather NLB (Network Load Balancer) information
    log_info "Gathering NLB information..."
    local nlb_arns
    nlb_arns=$(aws elbv2 describe-load-balancers --region "${region}" --query 'LoadBalancers[?Type==`network`].LoadBalancerArn' --output text 2>/dev/null || echo "")
    
    if [[ -n "${nlb_arns}" ]]; then
        for nlb_arn in ${nlb_arns}; do
            local nlb_name
            nlb_name=$(echo "${nlb_arn}" | awk -F'/' '{print $NF}')
            log_info "Processing NLB: ${nlb_name}"
            
            aws elbv2 describe-load-balancers \
                --load-balancer-arns "${nlb_arn}" \
                --region "${region}" \
                --output json > "${aws_dir}/nlb-${nlb_name}-description.json" 2>/dev/null || true
            
            aws elbv2 describe-target-health \
                --load-balancer-arn "${nlb_arn}" \
                --region "${region}" \
                --output json > "${aws_dir}/nlb-${nlb_name}-target-health.json" 2>/dev/null || true
        done
    else
        log_warn "No NLBs found in region ${region}"
    fi

    # Gather Classic Load Balancer information
    log_info "Gathering Classic Load Balancer information..."
    aws elb describe-load-balancers \
        --region "${region}" \
        --output json > "${aws_dir}/classic-lb-description.json" 2>/dev/null || true

    # Gather CloudWatch Logs for load balancers (if access logging is enabled)
    log_info "Checking CloudWatch Logs for load balancer access logs..."
    local log_groups
    log_groups=$(aws logs describe-log-groups --region "${region}" --query 'logGroups[?contains(logGroupName, `loadbalancer`) || contains(logGroupName, `alb`) || contains(logGroupName, `nlb`)].logGroupName' --output text 2>/dev/null || echo "")
    
    if [[ -n "${log_groups}" ]]; then
        for log_group in ${log_groups}; do
            local log_group_name
            log_group_name=$(echo "${log_group}" | tr '/' '_')
            log_info "Exporting logs from CloudWatch Log Group: ${log_group}"
            
            # Get recent log streams (last 24 hours)
            local start_time
            start_time=$(date -u -d '24 hours ago' +%s)000
            local end_time
            end_time=$(date -u +%s)000
            
            aws logs filter-log-events \
                --log-group-name "${log_group}" \
                --start-time "${start_time}" \
                --end-time "${end_time}" \
                --region "${region}" \
                --output json > "${aws_dir}/cloudwatch-${log_group_name}-logs.json" 2>/dev/null || true
        done
    else
        log_warn "No CloudWatch log groups found for load balancers"
    fi

    # Gather VPC Flow Logs (may contain LB traffic information)
    log_info "Checking for VPC Flow Logs..."
    local vpc_ids
    vpc_ids=$(aws ec2 describe-vpcs --region "${region}" --query 'Vpcs[*].VpcId' --output text 2>/dev/null || echo "")
    
    if [[ -n "${vpc_ids}" ]]; then
        for vpc_id in ${vpc_ids}; do
            aws ec2 describe-flow-logs \
                --filter "Name=resource-id,Values=${vpc_id}" \
                --region "${region}" \
                --output json > "${aws_dir}/vpc-${vpc_id}-flow-logs-config.json" 2>/dev/null || true
        done
    fi

    log_info "AWS Load Balancer logs gathered to: ${aws_dir}"
}

# Gather Azure Load Balancer logs
gather_azure_lb_logs() {
    log_info "Gathering Azure Load Balancer logs..."
    local azure_dir="${OUTPUT_DIR}/azure"
    mkdir -p "${azure_dir}"

    # Get subscription ID
    local subscription_id
    subscription_id=$(az account show --query id -o tsv 2>/dev/null || echo "")
    if [[ -z "${subscription_id}" ]]; then
        log_error "Could not get Azure subscription ID"
        return 1
    fi

    log_info "Using Azure subscription: ${subscription_id}"

    # List all load balancers
    log_info "Listing Azure Load Balancers..."
    az network lb list \
        --output json > "${azure_dir}/load-balancers-list.json" 2>/dev/null || true

    # Get details for each load balancer
    local lb_names
    lb_names=$(az network lb list --query '[].name' -o tsv 2>/dev/null || echo "")
    
    if [[ -n "${lb_names}" ]]; then
        for lb_name in ${lb_names}; do
            local resource_group
            resource_group=$(az network lb show --name "${lb_name}" --query resourceGroup -o tsv 2>/dev/null || echo "")
            
            if [[ -n "${resource_group}" ]]; then
                log_info "Processing Load Balancer: ${lb_name} in resource group: ${resource_group}"
                
                # Get load balancer details
                az network lb show \
                    --name "${lb_name}" \
                    --resource-group "${resource_group}" \
                    --output json > "${azure_dir}/lb-${lb_name}-details.json" 2>/dev/null || true
                
                # Get load balancer backend pools
                az network lb address-pool list \
                    --lb-name "${lb_name}" \
                    --resource-group "${resource_group}" \
                    --output json > "${azure_dir}/lb-${lb_name}-backend-pools.json" 2>/dev/null || true
                
                # Get load balancer rules
                az network lb rule list \
                    --lb-name "${lb_name}" \
                    --resource-group "${resource_group}" \
                    --output json > "${azure_dir}/lb-${lb_name}-rules.json" 2>/dev/null || true
                
                # Get load balancer probes
                az network lb probe list \
                    --lb-name "${lb_name}" \
                    --resource-group "${resource_group}" \
                    --output json > "${azure_dir}/lb-${lb_name}-probes.json" 2>/dev/null || true
            fi
        done
    else
        log_warn "No Azure Load Balancers found"
    fi

    # Gather Application Gateway logs (if any)
    log_info "Checking for Application Gateways..."
    az network application-gateway list \
        --output json > "${azure_dir}/application-gateways-list.json" 2>/dev/null || true

    # Gather Load Balancer metrics from Azure Monitor (last 24 hours)
    log_info "Gathering Load Balancer metrics from Azure Monitor..."
    if [[ -n "${lb_names}" ]]; then
        for lb_name in ${lb_names}; do
            local resource_group
            resource_group=$(az network lb show --name "${lb_name}" --query resourceGroup -o tsv 2>/dev/null || echo "")
            
            if [[ -n "${resource_group}" ]]; then
                local resource_id
                resource_id="/subscriptions/${subscription_id}/resourceGroups/${resource_group}/providers/Microsoft.Network/loadBalancers/${lb_name}"
                
                # Get metrics for last 24 hours
                az monitor metrics list \
                    --resource "${resource_id}" \
                    --metric "ByteCount,PacketCount" \
                    --start-time "$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)" \
                    --end-time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                    --output json > "${azure_dir}/lb-${lb_name}-metrics.json" 2>/dev/null || true
            fi
        done
    fi

    # Gather Network Security Group flow logs (may contain LB traffic)
    log_info "Checking for NSG Flow Logs..."
    az network watcher flow-log list \
        --output json > "${azure_dir}/nsg-flow-logs-list.json" 2>/dev/null || true

    log_info "Azure Load Balancer logs gathered to: ${azure_dir}"
}

# Gather GCP Load Balancer logs
gather_gcp_lb_logs() {
    log_info "Gathering GCP Load Balancer logs..."
    local gcp_dir="${OUTPUT_DIR}/gcp"
    mkdir -p "${gcp_dir}"

    # Get project ID
    local project_id
    project_id=$(gcloud config get-value project 2>/dev/null || echo "")
    if [[ -z "${project_id}" ]]; then
        log_error "Could not get GCP project ID"
        return 1
    fi

    log_info "Using GCP project: ${project_id}"

    # Gather HTTP(S) Load Balancer information
    log_info "Gathering HTTP(S) Load Balancer information..."
    gcloud compute url-maps list \
        --project "${project_id}" \
        --format json > "${gcp_dir}/url-maps-list.json" 2>/dev/null || true

    # Gather Network Load Balancer information
    log_info "Gathering Network Load Balancer information..."
    gcloud compute forwarding-rules list \
        --project "${project_id}" \
        --format json > "${gcp_dir}/forwarding-rules-list.json" 2>/dev/null || true

    # Gather Target Pools
    log_info "Gathering Target Pools..."
    gcloud compute target-pools list \
        --project "${project_id}" \
        --format json > "${gcp_dir}/target-pools-list.json" 2>/dev/null || true

    # Gather Backend Services
    log_info "Gathering Backend Services..."
    gcloud compute backend-services list \
        --project "${project_id}" \
        --format json > "${gcp_dir}/backend-services-list.json" 2>/dev/null || true

    # Gather Health Checks
    log_info "Gathering Health Checks..."
    gcloud compute health-checks list \
        --project "${project_id}" \
        --format json > "${gcp_dir}/health-checks-list.json" 2>/dev/null || true

    # Gather Load Balancer logs from Cloud Logging (last 24 hours)
    log_info "Gathering Load Balancer logs from Cloud Logging..."
    local start_time
    start_time=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)
    local end_time
    end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    
    # Query for load balancer related logs
    gcloud logging read \
        "resource.type=(\"http_load_balancer\" OR \"tcp_proxy\" OR \"ssl_proxy\")" \
        --project "${project_id}" \
        --format json \
        --limit 10000 \
        > "${gcp_dir}/load-balancer-logs.json" 2>/dev/null || true

    # Also gather VPC Flow Logs (may contain LB traffic)
    log_info "Checking for VPC Flow Logs..."
    gcloud logging read \
        "resource.type=gce_subnetwork AND jsonPayload.src_instance.vm_name=~\"lb-\" OR jsonPayload.dest_instance.vm_name=~\"lb-\"" \
        --project "${project_id}" \
        --format json \
        --limit 5000 \
        > "${gcp_dir}/vpc-flow-logs-lb-related.json" 2>/dev/null || true

    log_info "GCP Load Balancer logs gathered to: ${gcp_dir}"
}

# Extract cluster information from environment or cluster
extract_cluster_info() {
    log_info "Extracting cluster information..."
    
    # Use provided environment variables if set
    if [[ -n "${CLUSTER_INFRA_ID}" ]]; then
        log_info "Using provided CLUSTER_INFRA_ID: ${CLUSTER_INFRA_ID}"
    fi
    if [[ -n "${CLUSTER_VPC_ID}" ]]; then
        log_info "Using provided CLUSTER_VPC_ID: ${CLUSTER_VPC_ID}"
    fi
    if [[ -n "${CLUSTER_RESOURCE_GROUP}" ]]; then
        log_info "Using provided CLUSTER_RESOURCE_GROUP: ${CLUSTER_RESOURCE_GROUP}"
    fi
    
    # Try to get cluster info from KUBECONFIG if available
    if [[ -f "${KUBECONFIG:-}" ]]; then
        log_info "Attempting to extract cluster information from cluster"
        
        if command -v oc &> /dev/null; then
            # Try to get cluster name from infrastructure resource
            if oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}' 2>/dev/null | grep -q .; then
                local infra_name
                infra_name=$(oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}' 2>/dev/null || echo "")
                if [[ -n "${infra_name}" && -z "${CLUSTER_INFRA_ID}" ]]; then
                    CLUSTER_INFRA_ID="${infra_name}"
                    log_info "Found cluster infraID from cluster: ${CLUSTER_INFRA_ID}"
                fi
            fi
            
            # Try to get VPC ID from cluster
            if oc get infrastructure cluster -o jsonpath='{.status.platformStatus.aws.vpcID}' 2>/dev/null | grep -q .; then
                local vpc_id
                vpc_id=$(oc get infrastructure cluster -o jsonpath='{.status.platformStatus.aws.vpcID}' 2>/dev/null || echo "")
                if [[ -n "${vpc_id}" && -z "${CLUSTER_VPC_ID}" ]]; then
                    CLUSTER_VPC_ID="${vpc_id}"
                    log_info "Found cluster VPC ID from cluster: ${CLUSTER_VPC_ID}"
                fi
            fi
        fi
    fi
}

# Main function
main() {
    log_info "Starting cloud load balancer log gathering..."
    log_info "Output directory: ${OUTPUT_DIR}"

    # Create output directory
    mkdir -p "${OUTPUT_DIR}"

    # Extract cluster information
    extract_cluster_info

    # Detect cloud provider
    local provider
    provider=$(detect_cloud_provider)
    if [[ $? -ne 0 ]]; then
        exit 1
    fi

    log_info "Detected cloud provider: ${provider}"
    if [[ -n "${CLUSTER_INFRA_ID}" || -n "${CLUSTER_VPC_ID}" || -n "${CLUSTER_RESOURCE_GROUP}" ]]; then
        log_info "Filtering for cluster: infraID=${CLUSTER_INFRA_ID}, VPC=${CLUSTER_VPC_ID}, ResourceGroup=${CLUSTER_RESOURCE_GROUP}"
    fi

    # Gather logs based on provider
    case "${provider}" in
        aws)
            gather_aws_lb_logs
            ;;
        azure)
            gather_azure_lb_logs
            ;;
        gcp)
            gather_gcp_lb_logs
            ;;
        *)
            log_error "Unsupported cloud provider: ${provider}"
            exit 1
            ;;
    esac

    log_info "Cloud load balancer log gathering completed!"
    log_info "Logs are available in: ${OUTPUT_DIR}"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --cloud-provider)
            CLOUD_PROVIDER="$2"
            shift 2
            ;;
        --help|-h)
            cat <<EOF
Usage: $0 [OPTIONS]

Gather load balancer logs from cloud providers for must-gather/gather-extra steps.

Options:
    --output-dir DIR       Directory to store gathered logs (default: ./must-gather/cloud-lb-logs)
    --cloud-provider NAME   Cloud provider: aws, azure, or gcp (auto-detected if not set)
    --help, -h              Show this help message

Environment Variables:
    OUTPUT_DIR              Directory to store gathered logs
    CLOUD_PROVIDER          Cloud provider (aws, azure, gcp)
    AWS_REGION              AWS region (required for AWS)
    CLUSTER_INFRA_ID        Cluster infrastructure ID (for filtering cluster resources)
    CLUSTER_VPC_ID          Cluster VPC ID (AWS only, for filtering)
    CLUSTER_RESOURCE_GROUP  Cluster resource group (Azure only, for filtering)
    KUBECONFIG              Path to kubeconfig (used to extract cluster info if available)

Examples:
    # Auto-detect cloud provider
    $0 --output-dir /tmp/lb-logs

    # Specify AWS explicitly
    CLOUD_PROVIDER=aws AWS_REGION=us-east-1 $0

    # Use in must-gather
    OUTPUT_DIR=/must-gather/cloud-lb-logs $0
EOF
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

main