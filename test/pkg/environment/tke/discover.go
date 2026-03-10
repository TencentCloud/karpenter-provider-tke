/*
Copyright (C) 2012-2025 Tencent. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tke

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	vpc2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
)

// discoverFromCluster uses K8s node labels and Tencent Cloud SDK to auto-discover
// the cloud resource configuration needed for E2E tests.
func (env *Environment) discoverFromCluster() {
	secretID, secretKey, err := loadCredentials()
	if err != nil {
		panic(fmt.Sprintf("auto-discover: failed to load credentials: %v", err))
	}

	// Create K8s client from the existing rest.Config
	kubeClient, err := kubernetes.NewForConfig(env.Config)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: failed to create kubernetes client: %v", err))
	}

	// Get the first node
	nodes, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{Limit: 1})
	if err != nil || len(nodes.Items) == 0 {
		panic(fmt.Sprintf("auto-discover: failed to list nodes: %v", err))
	}
	node := nodes.Items[0]

	// Read labels
	instanceID := node.Labels["cloud.tencent.com/node-instance-id"]
	shortRegion := node.Labels["topology.kubernetes.io/region"]
	if instanceID == "" {
		panic("auto-discover: node missing label cloud.tencent.com/node-instance-id")
	}
	if shortRegion == "" {
		panic("auto-discover: node missing label topology.kubernetes.io/region")
	}
	log.Printf("auto-discover: node=%s instanceID=%s shortRegion=%s", node.Name, instanceID, shortRegion)

	// Map short region to full region name
	fullRegion := mapShortRegion(shortRegion)
	log.Printf("auto-discover: mapped region %q -> %q", shortRegion, fullRegion)

	// Create SDK clients
	credential := common.NewCredential(secretID, secretKey)
	pf := profile.NewClientProfile()
	pf.Language = "en-US"

	cvmClient, err := cvm2017.NewClient(credential, fullRegion, pf)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: failed to create CVM client: %v", err))
	}

	// DescribeInstances to get VPC ID, Zone, SecurityGroup, SubnetId
	descReq := cvm2017.NewDescribeInstancesRequest()
	descReq.Filters = []*cvm2017.Filter{
		{
			Name:   strPtr("instance-id"),
			Values: []*string{&instanceID},
		},
	}
	descResp, err := cvmClient.DescribeInstances(descReq)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: DescribeInstances failed: %v", err))
	}
	if len(descResp.Response.InstanceSet) == 0 {
		panic(fmt.Sprintf("auto-discover: instance %s not found", instanceID))
	}
	inst := descResp.Response.InstanceSet[0]

	zoneName := ptrStr(inst.Placement.Zone)
	vpcID := ptrStr(inst.VirtualPrivateCloud.VpcId)
	subnetID := ptrStr(inst.VirtualPrivateCloud.SubnetId)

	var securityGroupID string
	if len(inst.SecurityGroupIds) > 0 {
		securityGroupID = ptrStr(inst.SecurityGroupIds[0])
	}
	log.Printf("auto-discover: zone=%s vpcID=%s subnetID=%s securityGroupID=%s",
		zoneName, vpcID, subnetID, securityGroupID)

	// Derive region from zone (e.g. "ap-hongkong-3" -> "ap-hongkong")
	region := zoneToRegion(zoneName)

	// Get Zone ID using zone provider
	zoneProvider := zone.NewDefaultProvider(context.Background())
	zoneID, err := zoneProvider.IDFromZone(zoneName)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: IDFromZone(%s) failed: %v", zoneName, err))
	}
	log.Printf("auto-discover: region=%s zoneID=%s", region, zoneID)

	// DescribeKeyPairs to get the first SSH Key ID
	kpReq := cvm2017.NewDescribeKeyPairsRequest()
	kpResp, err := cvmClient.DescribeKeyPairs(kpReq)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: DescribeKeyPairs failed: %v", err))
	}
	var sshKeyID string
	if len(kpResp.Response.KeyPairSet) > 0 {
		sshKeyID = ptrStr(kpResp.Response.KeyPairSet[0].KeyId)
	}
	if sshKeyID == "" {
		panic("auto-discover: no SSH key pairs found")
	}
	log.Printf("auto-discover: sshKeyID=%s", sshKeyID)

	// If the node instance doesn't have a SubnetId, query VPC for one
	if subnetID == "" {
		vpcClient, err := vpc2017.NewClient(credential, fullRegion, pf)
		if err != nil {
			panic(fmt.Sprintf("auto-discover: failed to create VPC client: %v", err))
		}
		subReq := vpc2017.NewDescribeSubnetsRequest()
		subReq.Filters = []*vpc2017.Filter{
			{
				Name:   strPtr("vpc-id"),
				Values: []*string{&vpcID},
			},
		}
		subResp, err := vpcClient.DescribeSubnets(subReq)
		if err != nil {
			panic(fmt.Sprintf("auto-discover: DescribeSubnets failed: %v", err))
		}
		if len(subResp.Response.SubnetSet) == 0 {
			panic(fmt.Sprintf("auto-discover: no subnets found in VPC %s", vpcID))
		}
		subnetID = ptrStr(subResp.Response.SubnetSet[0].SubnetId)
		log.Printf("auto-discover: discovered subnetID=%s from VPC", subnetID)
	}

	ccrPrefix := ccrPrefixForRegion(region)
	log.Printf("auto-discover: ccrPrefix=%s", ccrPrefix)

	// Populate environment fields
	if env.Region == "" {
		env.Region = region
	}
	if env.SubnetID == "" {
		env.SubnetID = subnetID
	}
	if env.SecurityGroupID == "" {
		env.SecurityGroupID = securityGroupID
	}
	if env.SSHKeyID == "" {
		env.SSHKeyID = sshKeyID
	}
	if env.ZoneID == "" {
		env.ZoneID = zoneID
	}
	if env.Zone == "" {
		env.Zone = zoneName
	}
	if env.CCRPrefix == "" {
		env.CCRPrefix = ccrPrefix
	}

	// Write cache for subsequent runs
	env.writeCacheFile()
	log.Printf("auto-discover: discovery complete")
}

// loadCredentials reads AK/SK from environment variables or the ./secret file.
// Priority: SECRET_ID/SECRET_KEY env vars > ./secret file.
func loadCredentials() (secretID, secretKey string, err error) {
	secretID = os.Getenv("SECRET_ID")
	secretKey = os.Getenv("SECRET_KEY")
	if secretID != "" && secretKey != "" {
		return secretID, secretKey, nil
	}

	// Try reading from secret file at candidate paths
	for _, candidate := range []string{
		"secret",
		"../../../secret",
		"../../secret",
		"../../../../secret",
	} {
		sid, skey, ferr := parseSecretFile(candidate)
		if ferr == nil && sid != "" && skey != "" {
			return sid, skey, nil
		}
	}

	return "", "", fmt.Errorf("credentials not found: set SECRET_ID/SECRET_KEY environment variables or create a ./secret file with SecretId:xxx and SecretKey:xxx")
}

// parseSecretFile parses a file with lines like "SecretId:xxx" / "SecretKey:xxx".
func parseSecretFile(path string) (secretID, secretKey string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "SecretId":
			secretID = value
		case "SecretKey":
			secretKey = value
		}
	}
	return secretID, secretKey, scanner.Err()
}

// mapShortRegion maps short region codes (from node labels) to full Tencent Cloud region names.
func mapShortRegion(short string) string {
	m := map[string]string{
		"cd": "ap-chengdu",
		"gz": "ap-guangzhou",
		"bj": "ap-beijing",
		"sh": "ap-shanghai",
		"cq": "ap-chongqing",
		"hk": "ap-hongkong",
		"sg": "ap-singapore",
	}
	if full, ok := m[short]; ok {
		return full
	}
	// If not in the map, assume it's already a full region name
	return short
}

// ccrPrefixForRegion returns the CCR image registry prefix for the given region.
func ccrPrefixForRegion(region string) string {
	switch region {
	case "ap-hongkong":
		return "hkccr"
	case "ap-singapore":
		return "sgccr"
	default:
		return "ccr"
	}
}

// zoneToRegion extracts the region from a zone name by removing the trailing "-N" suffix.
// e.g. "ap-hongkong-3" -> "ap-hongkong", "ap-beijing-7" -> "ap-beijing"
func zoneToRegion(zoneName string) string {
	idx := strings.LastIndex(zoneName, "-")
	if idx == -1 {
		return zoneName
	}
	// Verify the suffix is numeric
	suffix := zoneName[idx+1:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return zoneName
		}
	}
	return zoneName[:idx]
}

// writeCacheFile writes the discovered environment to test/integration/env.cache.
func (env *Environment) writeCacheFile() {
	// Try multiple candidate paths (same as loadFromCacheFile)
	candidates := []string{
		"test/integration/env.cache",
		"../../../test/integration/env.cache",
		"../../test/integration/env.cache",
		"../../../../test/integration/env.cache",
	}

	content := fmt.Sprintf(`# Auto-generated by Go E2E test auto-discovery.
# Delete this file to force re-discovery.
REGION="%s"
SUBNET_ID="%s"
SECURITY_GROUP_ID="%s"
SSH_KEY_ID="%s"
ZONE="%s"
ZONE_ID="%s"
CCR_PREFIX="%s"
`, env.Region, env.SubnetID, env.SecurityGroupID, env.SSHKeyID, env.Zone, env.ZoneID, env.CCRPrefix)

	for _, candidate := range candidates {
		if err := os.WriteFile(candidate, []byte(content), 0644); err == nil {
			log.Printf("auto-discover: wrote cache file to %s", candidate)
			return
		}
	}
	log.Printf("auto-discover: WARNING: could not write cache file to any candidate path")
}

func strPtr(s string) *string { return &s }

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
