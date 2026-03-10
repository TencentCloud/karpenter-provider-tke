#!/bin/bash

# A comprehensive integration test suite for open-karpenter-provider-tke
#
# This script will:
#   1. Set up the environment. On the first run it queries cloud resources via tccli
#      and creates a dedicated test subnet, then saves all discovered values to
#      test/integration/env.cache. On subsequent runs the cache is loaded directly,
#      skipping all tccli calls (no tccli dependency required).
#   2. Optionally build & push a new controller image and (re-)install Karpenter via
#      Helm. This step is controlled by the INSTALL_KARPENTER variable (default: false).
#      Set INSTALL_KARPENTER=true to rebuild the image and reinstall; otherwise the
#      already-running Karpenter deployment is reused and no AK/SK is required.
#   3. Run a series of test cases using the cached subnet.
#   4. Perform cleanup of Kubernetes resources only. The test subnet is kept so it
#      can be reused across runs.
#
# Usage:
#   # Run tests only (no reinstall, no AK/SK needed):
#   ./test/integration/run-test.sh
#
#   # Rebuild image and reinstall Karpenter before running tests:
#   INSTALL_KARPENTER=true ./test/integration/run-test.sh

set -eo pipefail

# --- Logging and Utility Functions ---

log_info() {
    echo -e "\n[INFO] $(date +'%Y-%m-%d %H:%M:%S') - $1" >&2
}

log_pass() {
    echo -e "\n[PASS] $(date +'%Y-%m-%d %H:%M:%S') - $1" >&2
}

log_fail() {
    echo -e "\n[FAIL] $(date +'%Y-%m-%d %H:%M:%S') - $1" >&2
    exit 1
}

run_kubectl() {
    http_proxy="" https_proxy="" kubectl "$@"
}

run_helm() {
    http_proxy="" https_proxy="" helm "$@"
}

# --- Core Setup and Teardown Functions ---

setup_dependencies() {
    log_info "Unsetting proxy and installing dependencies..."
    unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY
    if ! command -v jq &> /dev/null; then
        log_info "jq not found. Attempting to install..."
        if command -v yum &> /dev/null; then
            yum install -y jq
        elif command -v apt-get &> /dev/null; then
            apt-get update && apt-get install -y jq
        else
            log_fail "Could not find a package manager (apt-get or yum) to install jq. Please install it manually."
        fi
    fi
}

discover_and_setup_environment() {
    log_info "Discovering environment variables and cloud provider info..."

    if [ -f "./env" ]; then
        source ./env
    else
        log_fail "./env file not found"
    fi

    if [ -z "${KUBECONFIG}" ]; then
        log_fail "KUBECONFIG not set in ./env"
    fi
    export KUBECONFIG_PATH="${KUBECONFIG}"
    log_info "Using KUBECONFIG: ${KUBECONFIG_PATH}"

    if ! run_kubectl get nodes > /dev/null; then
        log_fail "kubectl failed to connect to cluster using ${KUBECONFIG_PATH}"
    fi

    # Ensure the default namespace is used for test resources (kubeconfig may set a different default)
    run_kubectl config set-context --current --namespace=default > /dev/null

    # Extract CLUSTER_NAME from the kubeconfig server URL (no tccli dependency)
    local server_url
    server_url=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')
    CLUSTER_NAME=$(echo "${server_url}" | grep -oE 'cls-[a-z0-9]+')
    if [ -z "${CLUSTER_NAME}" ]; then
        log_fail "Could not extract cluster ID from server URL: ${server_url}"
    fi
    log_info "Cluster ID: ${CLUSTER_NAME}"

    local cache_file="test/integration/env.cache"

    # ---------- Cache hit: load from cache, skip all tccli calls ----------
    if [ -f "${cache_file}" ]; then
        log_info "Cache file '${cache_file}' found. Loading environment from cache (skipping tccli)."
        source "${cache_file}"
        if [ -z "${REGION}" ] || [ -z "${SUBNET_ID}" ] || [ -z "${SECURITY_GROUP_ID}" ] \
            || [ -z "${SSH_KEY_ID}" ] || [ -z "${ZONE}" ] || [ -z "${ZONE_ID}" ] \
            || [ -z "${CCR_PREFIX}" ]; then
            log_fail "Cache file is incomplete. Delete '${cache_file}' and re-run to regenerate."
        fi
        log_info "Loaded from cache: REGION=${REGION} SUBNET_ID=${SUBNET_ID} ZONE=${ZONE} ZONE_ID=${ZONE_ID} CCR_PREFIX=${CCR_PREFIX}"
        HK2_SUBNET_ID="${SUBNET_ID}"
        TEST_SUBNET_ID="${SUBNET_ID}"
        export REGION SUBNET_ID SECURITY_GROUP_ID SSH_KEY_ID CLUSTER_NAME REGISTRY \
               TEST_SUBNET_ID HK2_SUBNET_ID ZONE ZONE_ID CCR_PREFIX
        return
    fi

    # ---------- Cache miss: query cloud resources via tccli, create subnet, write cache ----------
    log_info "Cache file not found. Querying cloud resources via tccli..."

    local node_name instance_id filters instance_details vpc_id zone
    node_name=$(run_kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
    log_info "Node name: '${node_name}'"
    if [ -z "${node_name}" ]; then
        log_fail "Failed to get node name"
    fi

    instance_id=$(run_kubectl get node "${node_name}" -o jsonpath='{.metadata.labels.cloud\.tencent\.com/node-instance-id}')
    log_info "Instance ID: '${instance_id}'"
    if [ -z "${instance_id}" ]; then
        log_fail "Failed to get instance ID from node '${node_name}'"
    fi

    local node_region
    node_region=$(run_kubectl get node "${node_name}" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/region}')
    log_info "Node Region: ${node_region}"

    local region_map
    case "${node_region}" in
        "cd") region_map="ap-chengdu" ;;
        "gz") region_map="ap-guangzhou" ;;
        "bj") region_map="ap-beijing" ;;
        "sh") region_map="ap-shanghai" ;;
        "cq") region_map="ap-chongqing" ;;
        "hk") region_map="ap-hongkong" ;;
        "sg") region_map="ap-singapore" ;;
        *)    region_map="${node_region}" ;;
    esac
    log_info "Mapping region '${node_region}' to '${region_map}'"
    tccli configure set region "${region_map}"

    filters='[{"Name":"instance-id", "Values":["'${instance_id}'"]}]'
    instance_details=$(tccli cvm DescribeInstances --Filters "${filters}" --output json)
    log_info "Instance details (truncated): $(echo "${instance_details}" | head -c 100)"

    REGION=$(echo "${instance_details}" | jq -r '.InstanceSet[0].Placement.Zone' | sed 's/-\([0-9]\)$//g')
    vpc_id=$(echo "${instance_details}" | jq -r '.InstanceSet[0].VirtualPrivateCloud.VpcId')
    zone=$(echo "${instance_details}" | jq -r '.InstanceSet[0].Placement.Zone')
    log_info "Discovered Region: ${REGION}, VPC: ${vpc_id}, Zone: ${zone}"

    if [ -z "${vpc_id}" ] || [ "${vpc_id}" == "null" ]; then
        log_fail "Failed to discover VPC ID."
    fi

    local vpc_details vpc_cidr
    vpc_details=$(tccli vpc DescribeVpcs --VpcIds "[\"${vpc_id}\"]" --output json)
    vpc_cidr=$(echo "${vpc_details}" | jq -r '.VpcSet[0].CidrBlock')
    log_info "VPC CIDR: ${vpc_cidr}"
    if [ -z "${vpc_cidr}" ] || [ "${vpc_cidr}" == "null" ]; then
        log_fail "Failed to get VPC CIDR."
    fi

    local octet1 octet2 random_octet new_subnet_cidr
    octet1=$(echo "${vpc_cidr}" | cut -d'/' -f1 | cut -d'.' -f1)
    octet2=$(echo "${vpc_cidr}" | cut -d'/' -f1 | cut -d'.' -f2)
    random_octet=$(( ($(date +%s) % 150) + 50 ))
    new_subnet_cidr="${octet1}.${octet2}.${random_octet}.0/24"
    log_info "Generated Subnet CIDR: ${new_subnet_cidr}"

    local new_subnet_name="karpenter-test-subnet-$(date +%s)"
    log_info "Creating subnet '${new_subnet_name}' (${new_subnet_cidr}) in VPC ${vpc_id} Zone ${zone}..."
    local create_subnet_result
    create_subnet_result=$(tccli vpc CreateSubnet --VpcId "${vpc_id}" --SubnetName "${new_subnet_name}" \
        --CidrBlock "${new_subnet_cidr}" --Zone "${zone}" --output json)

    TEST_SUBNET_ID=$(echo "${create_subnet_result}" | jq -r '.Subnet.SubnetId')
    if [ -z "${TEST_SUBNET_ID}" ] || [ "${TEST_SUBNET_ID}" == "null" ]; then
        log_fail "Failed to create subnet. CLI output: ${create_subnet_result}"
    fi
    log_info "Created subnet: ${TEST_SUBNET_ID} (CIDR: ${new_subnet_cidr})"
    SUBNET_ID="${TEST_SUBNET_ID}"

    SECURITY_GROUP_ID=$(echo "${instance_details}" | jq -r '.InstanceSet[0].SecurityGroupIds[0]')
    SSH_KEY_ID=$(tccli cvm DescribeKeyPairs --output json | jq -r '.KeyPairSet[0].KeyId')

    ZONE_ID=$(tccli cvm DescribeZones --output json | jq -r --arg z "${zone}" '.ZoneSet[] | select(.Zone == $z) | .ZoneId')
    if [ -z "${ZONE_ID}" ] || [ "${ZONE_ID}" == "null" ]; then
        log_fail "Could not determine zone ID for zone ${zone}."
    fi
    log_info "Zone: ${zone} (ID: ${ZONE_ID})"

    ZONE="${zone}"
    HK2_SUBNET_ID="${TEST_SUBNET_ID}"

    case "${REGION}" in
        "ap-hongkong")  CCR_PREFIX="hkccr" ;;
        "ap-singapore") CCR_PREFIX="sgccr" ;;
        *)              CCR_PREFIX="ccr" ;;
    esac
    log_info "CCR prefix: ${CCR_PREFIX}"

    # Write cache file for reuse on subsequent runs
    log_info "Saving environment to cache file '${cache_file}'..."
    cat > "${cache_file}" <<CACHE
# Auto-generated by run-test.sh on $(date)
# Delete this file to force re-discovery via tccli.
REGION="${REGION}"
SUBNET_ID="${SUBNET_ID}"
SECURITY_GROUP_ID="${SECURITY_GROUP_ID}"
SSH_KEY_ID="${SSH_KEY_ID}"
ZONE="${ZONE}"
ZONE_ID="${ZONE_ID}"
CCR_PREFIX="${CCR_PREFIX}"
CACHE
    log_info "Cache written to '${cache_file}'."

    export REGION SUBNET_ID SECURITY_GROUP_ID SSH_KEY_ID CLUSTER_NAME REGISTRY \
           TEST_SUBNET_ID HK2_SUBNET_ID ZONE ZONE_ID CCR_PREFIX
}

wait_for_crds_established() {
    log_info "Waiting for Karpenter CRDs to become established..."
    local crds=("nodepools.karpenter.sh" "nodeclaims.karpenter.sh" "tkemachinenodeclasses.karpenter.k8s.tke")
    local max_attempts=30 # 2.5 minutes timeout
    for crd in "${crds[@]}"; do
        local established=false
        for (( i=1; i<=max_attempts; i++ )); do
            local crd_status
            crd_status=$(run_kubectl get crd "$crd" -o jsonpath='{.status.conditions[?(@.type=="Established")].status}' 2>/dev/null)
            if [ "$crd_status" == "True" ]; then
                log_info "CRD '$crd' is established."
                established=true
                break
            fi
            log_info "CRD '$crd' not established yet (attempt $i/$max_attempts). Waiting 5s..."
            sleep 5
        done
        if [ "$established" == false ]; then
            log_fail "Timeout: CRD '$crd' did not become established."
        fi
    done
}

build_and_push_image() {
    log_info "Building and pushing controller image (Tag: ${IMAGE_TAG})..."
    make image TAG="${IMAGE_TAG}" REGISTRY="${REGISTRY}"
}

install_karpenter() {
    log_info "Installing Karpenter..."

    # Read AK/SK (only needed here; no other step depends on it)
    if [ ! -f "./secret" ]; then
        log_fail "./secret file not found. AK/SK is required for Karpenter installation."
    fi
    local secret_id secret_key
    secret_id=$(awk -F: '/SecretId/ {print $2}' ./secret)
    secret_key=$(awk -F: '/SecretKey/ {print $2}' ./secret)
    if [ -z "${secret_id}" ] || [ -z "${secret_key}" ]; then
        log_fail "Could not parse SecretId/SecretKey from ./secret."
    fi

    run_helm uninstall karpenter -n karpenter > /dev/null 2>&1 || true
    log_info "Waiting 15 seconds for old resources to be cleaned up..."
    sleep 15

    run_kubectl create ns karpenter --dry-run=client -o yaml | run_kubectl apply -f -
    run_kubectl create secret generic karpenter-secret -n karpenter \
      --from-literal=secretID="${secret_id}" \
      --from-literal=secretKey="${secret_key}" \
      --dry-run=client -o yaml | run_kubectl apply -f -

    log_info "Attempting Helm install..."
    if ! run_helm install karpenter charts/karpenter -n karpenter \
            --set settings.clusterName="${CLUSTER_NAME}" \
            --set settings.clusterID="${CLUSTER_NAME}" \
            --set settings.region="${REGION}" \
            --set settings.apiKeySecretName=karpenter-secret \
            --set controller.image.repository="${REGISTRY}/karpenter-tke-controller" \
            --set controller.image.tag="${IMAGE_TAG}" \
            --set controller.image.useGlobalRegistry=true \
            --timeout 10m; then
        log_fail "Helm installation failed."
    fi

    wait_for_crds_established

    log_info "Helm install complete. Waiting for deployment to be ready..."
    if ! run_kubectl wait --for=condition=Available deployment/karpenter -n karpenter --timeout=5m; then
        run_kubectl logs -l app.kubernetes.io/name=karpenter -n karpenter
        log_fail "Karpenter deployment did not become available in time."
    fi
}

cleanup_test_resources() {
    log_info "Cleaning up test-specific resources..."
    run_kubectl delete --ignore-not-found -f test/integration/deployment.yaml
    run_kubectl delete --ignore-not-found -f test/integration/nodepool.yaml
    run_kubectl delete --ignore-not-found -f test/integration/nodepool-instance-type.yaml
    if [ -f test/integration/nodepool-zone.yaml ]; then
        run_kubectl delete --ignore-not-found -f test/integration/nodepool-zone.yaml
        rm -f test/integration/nodepool-zone.yaml
    fi
    run_kubectl delete --ignore-not-found -f test/integration/nodepool-expiry.yaml
    run_kubectl delete --ignore-not-found -f test/integration/nodepool-fallback-invalid.yaml
    run_kubectl delete --ignore-not-found -f test/integration/nodepool-fallback-valid.yaml
    run_kubectl delete --ignore-not-found -f test/integration/nodepool-kernel-args.yaml
    if [ -f test/integration/tkemachinenodeclass-datadisk-encrypt.yaml ]; then
        run_kubectl delete --ignore-not-found -f test/integration/tkemachinenodeclass-datadisk-encrypt.yaml
        rm -f test/integration/tkemachinenodeclass-datadisk-encrypt.yaml
    fi
    if [ -f test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml ]; then
        run_kubectl delete --ignore-not-found -f test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml
        rm -f test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml
    fi
    if [ -f test/integration/tkemachinenodeclass.yaml ]; then
        run_kubectl delete --ignore-not-found -f test/integration/tkemachinenodeclass.yaml
        rm -f test/integration/tkemachinenodeclass.yaml
    fi
    # Wait for all NodeClaims to be fully deleted before proceeding
    log_info "Waiting for all NodeClaims to be deleted..."
    local max_attempts=60
    local all_deleted=false
    for (( i=1; i<=max_attempts; i++ )); do
        local nc_count
        nc_count=$(run_kubectl get nodeclaims --no-headers 2>/dev/null | wc -l)
        if [ "${nc_count}" -eq 0 ]; then
            log_info "All NodeClaims deleted."
            all_deleted=true
            break
        fi
        log_info "NodeClaims still exist (attempt $i/$max_attempts, count: ${nc_count}). Waiting 10s..."
        sleep 10
    done
    if [ "${all_deleted}" = false ]; then
        log_info "WARNING: NodeClaims may not have been fully deleted. Proceeding anyway..."
        run_kubectl get nodeclaims 2>/dev/null || true
    fi
}

cleanup_all() {
    log_info "Performing full cleanup of all integration test resources..."
    cleanup_test_resources

    # Only uninstall Karpenter if it was installed by this run
    if [ "${INSTALL_KARPENTER:-false}" = "true" ]; then
        run_helm uninstall karpenter -n karpenter || true
        log_info "Waiting 30 seconds for Helm resources to terminate..."
        sleep 30

        if run_kubectl get ns karpenter > /dev/null 2>&1; then
            log_info "Namespace 'karpenter' still exists. Force deleting..."
            # Remove finalizers from cluster-scoped Karpenter resources
            local cluster_resources="tkemachinenodeclasses.karpenter.k8s.tke nodepools.karpenter.sh nodeclaims.karpenter.sh"
            for resource in $cluster_resources; do
                run_kubectl get "$resource" -o name 2>/dev/null | while read -r res_name; do
                    log_info "Removing finalizers from $res_name"
                    run_kubectl patch "$res_name" -p '{"metadata":{"finalizers":[]}}' --type=merge
                done
            done
            # Remove finalizers from namespaced resources
            local ns_resources="deployments services secrets roles rolebindings serviceaccounts poddisruptionbudgets leases"
            for resource in $ns_resources; do
                run_kubectl get -n karpenter "$resource" -o name 2>/dev/null | while read -r res_name; do
                    log_info "Removing finalizers from $res_name in namespace karpenter"
                    run_kubectl patch -n karpenter "$res_name" -p '{"metadata":{"finalizers":[]}}' --type=merge
                done
            done
            run_kubectl delete ns karpenter --force --grace-period=0 || true
        fi
    fi

    if [ -n "${TEST_SUBNET_ID}" ]; then
        log_info "Test subnet ${TEST_SUBNET_ID} is retained for future runs (cached in test/integration/env.cache)."
    fi

    # Clean up any generated YAML files left over from the test run
    rm -f test/integration/tkemachinenodeclass.yaml
    rm -f test/integration/tkemachinenodeclass-datadisk-encrypt.yaml
    rm -f test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml
    rm -f test/integration/nodepool-zone.yaml

    log_info "Full cleanup complete."
}

# --- Test Case Functions ---

generate_valid_nodeclass() {
    sed -e "s|<YOUR_SUBNET_ID>|${SUBNET_ID}|g" \
        -e "s|<YOUR_HK2_SUBNET_ID>|${HK2_SUBNET_ID}|g" \
        -e "s|<YOUR_SECURITY_GROUP_ID>|${SECURITY_GROUP_ID}|g" \
        -e "s|<YOUR_SSH_KEY_ID>|${SSH_KEY_ID}|g" \
        test/integration/tkemachinenodeclass.yaml.tpl > test/integration/tkemachinenodeclass.yaml
}

generate_zone_nodepool() {
    sed "s|<YOUR_ZONE_ID>|${ZONE_ID}|g" \
        test/integration/nodepool-zone.yaml.tpl > test/integration/nodepool-zone.yaml
}

wait_for_node_ready() {
    local max_attempts=$1
    local node_name=""
    log_info "Waiting for new node to become Ready..."
    for (( i=1; i<=max_attempts; i++ )); do
        node_name=$(run_kubectl get nodes -l karpenter.sh/nodepool -o json | jq -r '.items[] | select(.status.conditions[] | select(.type == "Ready" and .status == "True")) | .metadata.name' 2>/dev/null | head -1)
        if [ -n "${node_name}" ]; then
            log_info "New node ${node_name} is Ready."
            echo "${node_name}"
            return 0
        fi
        log_info "Node not ready yet (attempt $i/$max_attempts). Waiting 5s..."
        run_kubectl get nodes -l karpenter.sh/nodepool >&2 || true
        sleep 5
    done
    log_fail "Timeout: New node did not become ready within $((max_attempts * 5)) seconds."
    return 1
}

wait_for_node_deleted() {
    local node_name=$1
    local max_attempts=$2
    log_info "Waiting for node ${node_name} to be deleted..."
    for (( i=1; i<=max_attempts; i++ )); do
        if ! run_kubectl get node "${node_name}" > /dev/null 2>&1; then
            log_info "Node ${node_name} successfully deleted."
            return 0
        fi
        log_info "Node ${node_name} still exists (attempt $i/$max_attempts). Waiting 10s..."
        sleep 10
    done
    log_fail "Timeout: Node ${node_name} was not deleted within $((max_attempts * 10)) seconds."
    return 1
}

test_happy_path() {
    log_info "--- RUNNING: test_happy_path ---"
    generate_valid_nodeclass
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to 2 replicas to trigger scale-up..."
    run_kubectl scale deployment inflate --replicas=2

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Scaling deployment to 0 replicas to trigger scale-down..."
    run_kubectl scale deployment inflate --replicas=0

    wait_for_node_deleted "${new_node_name}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_happy_path ---"
}

test_invalid_nodeclass() {
    log_info "--- RUNNING: test_invalid_nodeclass ---"
    log_info "Applying TKEMachineNodeClass with a non-existent Subnet ID..."
    sed -e "s|<YOUR_SUBNET_ID>|subnet-invalid|g" \
        -e "s|<YOUR_HK2_SUBNET_ID>|subnet-invalid|g" \
        -e "s|<YOUR_SECURITY_GROUP_ID>|${SECURITY_GROUP_ID}|g" \
        -e "s|<YOUR_SSH_KEY_ID>|${SSH_KEY_ID}|g" \
        test/integration/tkemachinenodeclass.yaml.tpl > test/integration/tkemachinenodeclass.yaml

    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool.yaml

    log_info "Waiting for NodeClass status to report an error..."
    local found_error=false
    for (( i=1; i<=24; i++ )); do
        local nc_status
        nc_status=$(run_kubectl get tkemachinenodeclass default -o json | jq -r '.status.conditions[] | select(.type == "Ready" and .status == "False") | .message' 2>/dev/null)
        if [[ $nc_status == *"Failed to resolve subnets"* ]] || [[ $nc_status == *"subnet"* ]]; then
            log_info "Correctly found error in NodeClass status: $nc_status"
            found_error=true
            break
        fi
        sleep 5
    done

    if [ "$found_error" = false ]; then
        run_kubectl get tkemachinenodeclass default -o yaml
        log_fail "NodeClass did not report the expected error for invalid subnet (expected 'Failed to resolve subnets')."
    fi

    cleanup_test_resources
    log_pass "--- PASSED: test_invalid_nodeclass ---"
}

test_instance_type_constraint() {
    log_info "--- RUNNING: test_instance_type_constraint ---"
    generate_valid_nodeclass
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool-instance-type.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to trigger node creation with specific instance type..."
    run_kubectl scale deployment inflate --replicas=2

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Verifying instance type of the new node..."
    local instance_type
    instance_type=$(run_kubectl get node "${new_node_name}" -o jsonpath='{.metadata.labels.node\.kubernetes\.io/instance-type}')

    if [ "${instance_type}" != "SA2.MEDIUM4" ]; then
        log_fail "Instance type constraint failed. Expected SA2.MEDIUM4, but got ${instance_type}."
    fi
    log_info "Instance type is correct: ${instance_type}"

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${new_node_name}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_instance_type_constraint ---"
}

test_zone_constraint() {
    log_info "--- RUNNING: test_zone_constraint ---"
    generate_valid_nodeclass
    generate_zone_nodepool
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool-zone.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to trigger node creation in a specific zone..."
    run_kubectl scale deployment inflate --replicas=2

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Verifying zone of the new node..."
    local node_zone
    node_zone=$(run_kubectl get node "${new_node_name}" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}')

    if [ "${node_zone}" != "${ZONE_ID}" ]; then
        log_fail "Zone constraint failed. Expected ${ZONE_ID}, but got ${node_zone}."
    fi
    log_info "Zone is correct: ${node_zone}"

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${new_node_name}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_zone_constraint ---"
}

test_expiry() {
    log_info "--- RUNNING: test_expiry ---"
    generate_valid_nodeclass
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool-expiry.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to trigger node creation with a 300s expiry..."
    run_kubectl scale deployment inflate --replicas=1

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Node ${new_node_name} created. Waiting for it to be terminated due to expiry (TTL is 300s)..."

    wait_for_node_deleted "${new_node_name}" 120

    cleanup_test_resources
    log_pass "--- PASSED: test_expiry ---"
}

test_multi_nodepool_fallback() {
    log_info "--- RUNNING: test_multi_nodepool_fallback ---"
    log_info "Scenario: two NodePools coexist - one with an invalid instance type, one valid."
    log_info "Expectation: the invalid NodePool does not block scale-up via the valid NodePool."

    generate_valid_nodeclass
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    # Apply both NodePools simultaneously
    run_kubectl apply -f test/integration/nodepool-fallback-invalid.yaml
    run_kubectl apply -f test/integration/nodepool-fallback-valid.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to trigger scale-up..."
    run_kubectl scale deployment inflate --replicas=2

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Verifying node was provisioned by the valid NodePool..."
    local node_pool_label
    node_pool_label=$(run_kubectl get node "${new_node_name}" -o jsonpath='{.metadata.labels.karpenter\.sh/nodepool}')

    if [ "${node_pool_label}" != "fallback-valid" ]; then
        log_fail "Expected node to be created by 'fallback-valid' NodePool, but got '${node_pool_label}'."
    fi
    log_info "Node ${new_node_name} was correctly provisioned by NodePool '${node_pool_label}'."

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${new_node_name}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_multi_nodepool_fallback ---"
}

test_kernel_args() {
    log_info "--- RUNNING: test_kernel_args ---"
    log_info "Scenario: NodePool sets multiple kernel args via annotations:"
    log_info "  vm.max_map_count=262144, net.core.somaxconn=65535, fs.file-max=1048576"
    log_info "Expectation: all three sysctl values are applied correctly on the provisioned node."

    generate_valid_nodeclass
    run_kubectl apply -f test/integration/tkemachinenodeclass.yaml
    run_kubectl apply -f test/integration/nodepool-kernel-args.yaml
    run_kubectl apply -f test/integration/deployment.yaml

    log_info "Scaling deployment to trigger node creation..."
    run_kubectl scale deployment inflate --replicas=1

    local new_node_name
    new_node_name=$(wait_for_node_ready 60)

    log_info "Verifying kernel args on node ${new_node_name}..."

    # Schedule one pod per sysctl param using direct `cat` to avoid shell expansion issues
    local sysctl_params=(
        "vm.max_map_count:/proc/sys/vm/max_map_count"
        "net.core.somaxconn:/proc/sys/net/core/somaxconn"
        "fs.file-max:/proc/sys/fs/file-max"
    )
    local pod_image="${CCR_PREFIX}.ccs.tencentyun.com/tkeimages/hyperkube:v1.34.1-tke.1-rc3"
    local actual_values=()

    for entry in "${sysctl_params[@]}"; do
        local param_name="${entry%%:*}"
        local proc_path="${entry##*:}"
        local pod_name="sysctl-checker-${param_name//./-}"
        pod_name="${pod_name//_/-}"

        run_kubectl delete pod "${pod_name}" --ignore-not-found --wait=true 2>/dev/null || true
        run_kubectl run "${pod_name}" \
            --image="${pod_image}" \
            --restart=Never \
            --overrides="{
              \"spec\": {
                \"tolerations\": [{\"key\":\"karpenter-test\",\"operator\":\"Equal\",\"value\":\"true\",\"effect\":\"NoSchedule\"}],
                \"nodeSelector\": {\"karpenter-test\": \"true\"},
                \"hostPID\": true,
                \"hostNetwork\": true,
                \"volumes\": [{\"name\":\"proc\",\"hostPath\":{\"path\":\"/proc\"}}],
                \"containers\": [{
                  \"name\": \"checker\",
                  \"image\": \"${pod_image}\",
                  \"command\": [\"cat\", \"/host-proc${proc_path##/proc}\"],
                  \"volumeMounts\": [{\"name\":\"proc\",\"mountPath\":\"/host-proc\"}],
                  \"securityContext\": {\"privileged\": true}
                }]
              }
            }" 2>&1 >/dev/null

        log_info "Waiting for pod ${pod_name} to complete..."
        local pod_done=false
        for (( j=1; j<=30; j++ )); do
            local pod_phase
            pod_phase=$(run_kubectl get pod "${pod_name}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Pending")
            if [ "${pod_phase}" = "Succeeded" ]; then
                pod_done=true
                break
            elif [ "${pod_phase}" = "Failed" ]; then
                run_kubectl logs "${pod_name}" || true
                run_kubectl delete pod "${pod_name}" --ignore-not-found
                log_fail "Pod ${pod_name} failed."
            fi
            log_info "  Pod phase: ${pod_phase} (attempt $j/30). Waiting 5s..."
            sleep 5
        done
        if [ "${pod_done}" = false ]; then
            run_kubectl describe pod "${pod_name}" || true
            run_kubectl delete pod "${pod_name}" --ignore-not-found
            log_fail "Timeout: pod ${pod_name} did not complete."
        fi

        local actual_value
        actual_value=$(run_kubectl logs "${pod_name}" | tr -d '[:space:]')
        log_info "sysctl ${param_name} = ${actual_value}"
        actual_values+=("${param_name}=${actual_value}")
        run_kubectl delete pod "${pod_name}" --ignore-not-found
    done

    declare -A expected_map=(
        ["vm.max_map_count"]="262144"
        ["net.core.somaxconn"]="65535"
        ["fs.file-max"]="1048576"
    )

    local all_passed=true
    for entry in "${actual_values[@]}"; do
        local param="${entry%%=*}"
        local actual="${entry##*=}"
        local expected="${expected_map[$param]}"
        if [ "${actual}" = "${expected}" ]; then
            log_info "  [OK] ${param} = ${actual}"
        else
            log_info "  [FAIL] ${param}: expected '${expected}', got '${actual}'"
            all_passed=false
        fi
    done

    if [ "${all_passed}" = false ]; then
        log_fail "One or more kernel arg verifications failed."
    fi

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${new_node_name}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_kernel_args ---"
}

test_datadisk_encrypt() {
    log_info "--- RUNNING: test_datadisk_encrypt ---"
    log_info "Two sub-scenarios:"
    log_info "  A: data disk with encrypt=ENCRYPT annotation -> Machine providerSpec encrypt=ENCRYPT"
    log_info "  B: data disk without encrypt annotation     -> Machine providerSpec encrypt empty"

    log_info "--- Sub-scenario A: disk WITH encrypt annotation ---"

    sed -e "s|<YOUR_SUBNET_ID>|${SUBNET_ID}|g" \
        -e "s|<YOUR_SECURITY_GROUP_ID>|${SECURITY_GROUP_ID}|g" \
        -e "s|<YOUR_SSH_KEY_ID>|${SSH_KEY_ID}|g" \
        test/integration/tkemachinenodeclass-datadisk-encrypt.yaml.tpl > test/integration/tkemachinenodeclass-datadisk-encrypt.yaml

    run_kubectl apply -f test/integration/tkemachinenodeclass-datadisk-encrypt.yaml
    run_kubectl apply -f test/integration/nodepool.yaml
    run_kubectl apply -f test/integration/deployment.yaml
    run_kubectl scale deployment inflate --replicas=1

    local node_a machine_a disk_id_a
    node_a=$(wait_for_node_ready 60)
    machine_a=$(run_kubectl get nodeclaims -o jsonpath='{.items[0].metadata.annotations.karpenter\.k8s\.tke/owned-machine}')
    [ -z "${machine_a}" ] && log_fail "Sub-scenario A: could not get machine name from NodeClaim."
    log_info "Sub-scenario A: machine=${machine_a}, node=${node_a}"

    disk_id_a=$(run_kubectl get machine "${machine_a}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].diskID}')
    local size_a encrypt_a
    size_a=$(run_kubectl get machine "${machine_a}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].diskSize}')
    encrypt_a=$(run_kubectl get machine "${machine_a}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].encrypt}')
    log_info "Sub-scenario A providerSpec: diskID=${disk_id_a}, diskSize=${size_a}, encrypt=${encrypt_a}"

    [ -z "${disk_id_a}" ]           && log_fail "Sub-scenario A: diskID is empty."
    [ "${size_a}" != "50" ]         && log_fail "Sub-scenario A: expected diskSize=50, got '${size_a}'."
    [ "${encrypt_a}" != "ENCRYPT" ] && log_fail "Sub-scenario A: expected encrypt='ENCRYPT', got '${encrypt_a}'."
    log_info "Sub-scenario A: PASSED (diskID=${disk_id_a}, size=${size_a}GB, encrypt=${encrypt_a})."

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${node_a}" 60
    cleanup_test_resources

    log_info "--- Sub-scenario B: disk WITHOUT encrypt annotation ---"

    sed -e "s|<YOUR_SUBNET_ID>|${SUBNET_ID}|g" \
        -e "s|<YOUR_SECURITY_GROUP_ID>|${SECURITY_GROUP_ID}|g" \
        -e "s|<YOUR_SSH_KEY_ID>|${SSH_KEY_ID}|g" \
        test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml.tpl > test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml

    run_kubectl apply -f test/integration/tkemachinenodeclass-datadisk-noencrypt.yaml
    run_kubectl apply -f test/integration/nodepool.yaml
    run_kubectl apply -f test/integration/deployment.yaml
    run_kubectl scale deployment inflate --replicas=1

    local node_b machine_b disk_id_b
    node_b=$(wait_for_node_ready 60)
    machine_b=$(run_kubectl get nodeclaims -o jsonpath='{.items[0].metadata.annotations.karpenter\.k8s\.tke/owned-machine}')
    [ -z "${machine_b}" ] && log_fail "Sub-scenario B: could not get machine name from NodeClaim."
    log_info "Sub-scenario B: machine=${machine_b}, node=${node_b}"

    disk_id_b=$(run_kubectl get machine "${machine_b}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].diskID}')
    local size_b encrypt_b
    size_b=$(run_kubectl get machine "${machine_b}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].diskSize}')
    encrypt_b=$(run_kubectl get machine "${machine_b}" -o jsonpath='{.spec.providerSpec.value.dataDisks[0].encrypt}')
    log_info "Sub-scenario B providerSpec: diskID=${disk_id_b}, diskSize=${size_b}, encrypt=${encrypt_b}"

    [ -z "${disk_id_b}" ]   && log_fail "Sub-scenario B: diskID is empty."
    [ "${size_b}" != "50" ] && log_fail "Sub-scenario B: expected diskSize=50, got '${size_b}'."
    [ -n "${encrypt_b}" ]   && log_fail "Sub-scenario B: expected encrypt to be empty (no annotation), got '${encrypt_b}'."
    log_info "Sub-scenario B: PASSED (diskID=${disk_id_b}, size=${size_b}GB, encrypt field absent as expected)."

    run_kubectl scale deployment inflate --replicas=0
    wait_for_node_deleted "${node_b}" 60

    cleanup_test_resources
    log_pass "--- PASSED: test_datadisk_encrypt ---"
}

# --- Main Execution ---

main() {
    trap cleanup_all EXIT

    setup_dependencies
    discover_and_setup_environment

    # INSTALL_KARPENTER=true  -> rebuild image and reinstall Karpenter (requires AK/SK in ./secret)
    # INSTALL_KARPENTER=false -> skip build/install, reuse the running Karpenter deployment (default)
    if [ "${INSTALL_KARPENTER:-false}" = "true" ]; then
        export IMAGE_TAG="test-$(date +%s)"
        build_and_push_image
        install_karpenter
    else
        log_info "Skipping Karpenter installation (INSTALL_KARPENTER is not 'true'). Reusing existing deployment."
        if ! run_kubectl get deployment karpenter -n karpenter > /dev/null 2>&1; then
            log_fail "No running Karpenter deployment found. Set INSTALL_KARPENTER=true to install."
        fi
        wait_for_crds_established
    fi

    # Run all test cases. Each test generates its own required YAML files.
    test_happy_path
    test_invalid_nodeclass
    test_instance_type_constraint
    test_zone_constraint
    test_expiry
    test_multi_nodepool_fallback
    test_kernel_args
    test_datadisk_encrypt

    log_pass "All integration tests completed successfully!"
}

main "$@"
