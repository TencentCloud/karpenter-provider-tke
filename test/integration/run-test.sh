#!/bin/bash

# A comprehensive integration test suite for open-karpenter-provider-tke
#
# This script will:
#   1. Set up the environment, discover cloud resources, and CREATE A NEW SUBNET.
#   2. Build and push a new controller image.
#   3. Install Karpenter using Helm and wait for CRDs to be ready.
#   4. Run a series of test cases using the new subnet.
#   5. Perform a full cleanup, including DELETING THE CREATED SUBNET.

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
    
    # Verify kubectl connectivity immediately
    if ! run_kubectl get nodes > /dev/null; then
        log_fail "kubectl failed to connect to cluster using ${KUBECONFIG_PATH}"
    fi

    export TENCENT_CLOUD_SECRET_ID=$(awk -F: '/SecretId/ {print $2}' ./secret)
    export TENCENT_CLOUD_SECRET_KEY=$(awk -F: '/SecretKey/ {print $2}' ./secret)
    export IMAGE_TAG="test-$(date +%s)"

    local node_name instance_id filters instance_details vpc_id
    local zone
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
        *) region_map="${node_region}" ;;
    esac
    log_info "Mapping region '${node_region}' to '${region_map}'"
    tccli configure set region "${region_map}"

    filters='[{"Name":"instance-id", "Values":["'${instance_id}'"]}]'
    log_info "Using filters: ${filters}"

    instance_details=$(tccli cvm DescribeInstances --Filters "${filters}" --output json)
    # Log first 100 chars of instance_details to check if it worked
    log_info "Instance details (truncated): $(echo "${instance_details}" | head -c 100)"

    REGION=$(echo "${instance_details}" | jq -r '.InstanceSet[0].Placement.Zone' | sed 's/-\([0-9]\)$//g')
    vpc_id=$(echo "${instance_details}" | jq -r '.InstanceSet[0].VirtualPrivateCloud.VpcId')
    zone=$(echo "${instance_details}" | jq -r '.InstanceSet[0].Placement.Zone')

    log_info "Discovered Region: ${REGION}, VPC: ${vpc_id}, Zone: ${zone}"

    if [ -z "${vpc_id}" ] || [ "${vpc_id}" == "null" ]; then
        log_fail "Failed to discover VPC ID. tccli output might be empty or invalid."
    fi

    # Create a new subnet for this test run to avoid IP exhaustion.
    # Use a unique CIDR derived from the timestamp to avoid conflicts across parallel runs.
    # Get VPC CIDR to determine valid subnet range
    local vpc_details
    vpc_details=$(tccli vpc DescribeVpcs --VpcIds "[\"${vpc_id}\"]" --output json)
    local vpc_cidr
    vpc_cidr=$(echo "${vpc_details}" | jq -r '.VpcSet[0].CidrBlock')
    log_info "VPC CIDR: ${vpc_cidr}"

    if [ -z "${vpc_cidr}" ] || [ "${vpc_cidr}" == "null" ]; then
        log_fail "Failed to get VPC CIDR."
    fi

    local vpc_prefix=$(echo "${vpc_cidr}" | cut -d'/' -f1)
    # Extract first two octets
    local octet1=$(echo "${vpc_prefix}" | cut -d'.' -f1)
    local octet2=$(echo "${vpc_prefix}" | cut -d'.' -f2)
    
    # Generate a random 3rd octet to avoid conflicts
    local random_octet=$(( ($(date +%s) % 150) + 50 ))
    local new_subnet_cidr="${octet1}.${octet2}.${random_octet}.0/24"
    log_info "Generated Subnet CIDR: ${new_subnet_cidr}"

    local new_subnet_name="karpenter-test-subnet-${IMAGE_TAG}"
    log_info "Creating a new subnet '${new_subnet_name}' with CIDR ${new_subnet_cidr} in VPC ${vpc_id} and Zone ${zone}..."
    local create_subnet_result
    create_subnet_result=$(tccli vpc CreateSubnet --VpcId "${vpc_id}" --SubnetName "${new_subnet_name}" --CidrBlock "${new_subnet_cidr}" --Zone "${zone}" --output json)

    TEST_SUBNET_ID=$(echo "${create_subnet_result}" | jq -r '.Subnet.SubnetId')
    if [ -z "${TEST_SUBNET_ID}" ] || [ "${TEST_SUBNET_ID}" == "null" ]; then
        log_fail "Failed to create new subnet. CLI output: ${create_subnet_result}"
    fi
    log_info "Successfully created subnet with ID: ${TEST_SUBNET_ID} (CIDR: ${new_subnet_cidr})"

    # Use this new subnet for the tests
    SUBNET_ID=${TEST_SUBNET_ID}

    SECURITY_GROUP_ID=$(echo "${instance_details}" | jq -r '.InstanceSet[0].SecurityGroupIds[0]')
    SSH_KEY_ID=$(tccli cvm DescribeKeyPairs --output json | jq -r '.KeyPairSet[0].KeyId')
    # Extract the real cluster ID from the server URL (e.g. https://cls-xxxx.xxx.tke.woa.com)
    local server_url
    server_url=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')
    CLUSTER_NAME=$(echo "${server_url}" | grep -oE 'cls-[a-z0-9]+')
    if [ -z "${CLUSTER_NAME}" ]; then
        log_fail "Could not extract cluster ID from server URL: ${server_url}"
    fi
    log_info "Cluster ID: ${CLUSTER_NAME}"

    # Derive the zone ID for the zone constraint test (uses same zone as the cluster)
    ZONE_ID=$(tccli cvm DescribeZones --output json | jq -r --arg z "${zone}" '.ZoneSet[] | select(.Zone == $z) | .ZoneId')
    if [ -z "${ZONE_ID}" ] || [ "${ZONE_ID}" == "null" ]; then
        log_fail "Could not determine zone ID for zone ${zone}."
    fi
    log_info "Cluster zone: ${zone} (ID: ${ZONE_ID})"

    # Use the same subnet as SUBNET_ID for the zone-test nodeclass (same zone)
    HK2_SUBNET_ID=${TEST_SUBNET_ID}

    ZONE="${zone}"
    export REGION SUBNET_ID SECURITY_GROUP_ID SSH_KEY_ID CLUSTER_NAME REGISTRY TEST_SUBNET_ID HK2_SUBNET_ID ZONE ZONE_ID
}

build_and_push_image() {
    log_info "Building and pushing controller image (Tag: ${IMAGE_TAG})..."
    make image TAG="${IMAGE_TAG}" REGISTRY="${REGISTRY}"
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

install_karpenter() {
    log_info "Installing Karpenter..."
    run_helm uninstall karpenter -n karpenter > /dev/null 2>&1 || true
    log_info "Waiting 15 seconds for old resources to be cleaned up..."
    sleep 15

    run_kubectl create ns karpenter --dry-run=client -o yaml | run_kubectl apply -f -
    run_kubectl create secret generic karpenter-secret -n karpenter \
      --from-literal=secretID="${TENCENT_CLOUD_SECRET_ID}" \
      --from-literal=secretKey="${TENCENT_CLOUD_SECRET_KEY}" \
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

    if [ -n "${TEST_SUBNET_ID}" ]; then
        log_info "Deleting dynamically created subnet ${TEST_SUBNET_ID}..."
        local subnet_deleted=false
        for (( i=1; i<=6; i++ )); do
            if tccli vpc DeleteSubnet --SubnetId "${TEST_SUBNET_ID}" > /dev/null 2>&1; then
                log_info "Successfully deleted subnet ${TEST_SUBNET_ID}."
                subnet_deleted=true
                break
            fi
            log_info "Subnet deletion attempt $i/6 failed (may have lingering ENIs). Waiting 30s..."
            sleep 30
        done
        if [ "${subnet_deleted}" = false ]; then
            log_info "WARNING: Could not delete subnet ${TEST_SUBNET_ID} after 6 attempts. Please delete it manually."
        fi
    fi

    # Clean up any generated YAML files left over from the test run
    rm -f test/integration/tkemachinenodeclass.yaml
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

# --- Main Execution ---

main() {
    trap cleanup_all EXIT

    setup_dependencies
    discover_and_setup_environment
    build_and_push_image
    install_karpenter

    # Run all test cases. Each test generates its own required YAML files.
    test_happy_path
    test_invalid_nodeclass
    test_instance_type_constraint
    test_zone_constraint
    test_expiry
    test_multi_nodepool_fallback

    log_pass "All integration tests completed successfully!"
}

main "$@"
