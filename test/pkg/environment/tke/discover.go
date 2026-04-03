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
	"context"
	"encoding/json"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
)

// discoverFromCluster uses in-cluster Kubernetes resources to auto-discover
// the cloud configuration needed for E2E tests.
//
// Discovery strategy (no cloud API calls, no credentials required):
//
//	Region   : node label "topology.kubernetes.io/region" (short code mapped to full name)
//	ZoneID   : node label "topology.kubernetes.io/zone"   (TKE stores numeric zone ID here)
//	Zone     : Machine.Spec.Zone
//	SubnetID : Machine.Spec.SubnetID
//	SecurityGroupID : Machine.Spec.ProviderSpec (CXMMachineProviderSpec).SecurityGroupIDs[0]
//	SSHKeyID        : Machine.Spec.ProviderSpec (CXMMachineProviderSpec).KeyIDs[0]
//	CCRPrefix: derived from Region via local lookup table
func (env *Environment) discoverFromCluster() {
	ctx := context.Background()

	// ── 1. Region / ZoneID from node labels ──────────────────────────────────
	kubeClient, err := kubernetes.NewForConfig(env.Config)
	if err != nil {
		panic(fmt.Sprintf("auto-discover: failed to create kubernetes client: %v", err))
	}
	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil || len(nodes.Items) == 0 {
		panic(fmt.Sprintf("auto-discover: failed to list nodes: %v", err))
	}
	node := nodes.Items[0]

	shortRegion := node.Labels["topology.kubernetes.io/region"]
	// In TKE clusters the "zone" label carries the numeric zone ID (e.g. "100006")
	zoneID := node.Labels["topology.kubernetes.io/zone"]
	if shortRegion == "" {
		panic("auto-discover: node missing label topology.kubernetes.io/region")
	}
	fullRegion := mapShortRegion(shortRegion)
	log.Printf("auto-discover: node=%s region=%s zoneID=%s", node.Name, fullRegion, zoneID)

	// ── 2. SubnetID / Zone / SecurityGroupID / SSHKeyID from Machine objects ─
	machineList := &capiv1beta1.MachineList{}
	if listErr := env.Client.List(ctx, machineList, &client.ListOptions{}); listErr != nil {
		panic(fmt.Sprintf("auto-discover: failed to list Machines: %v", listErr))
	}
	if len(machineList.Items) == 0 {
		panic("auto-discover: no Machine objects found in cluster; " +
			"ensure the cluster has at least one existing node managed by the machine controller")
	}

	// Pick first Machine that has a SubnetID set.
	var machine *capiv1beta1.Machine
	for i := range machineList.Items {
		if machineList.Items[i].Spec.SubnetID != "" {
			machine = &machineList.Items[i]
			break
		}
	}
	if machine == nil {
		machine = &machineList.Items[0]
	}
	log.Printf("auto-discover: using Machine %q", machine.Name)

	subnetID := machine.Spec.SubnetID
	zoneName := machine.Spec.Zone

	// Decode ProviderSpec to extract SecurityGroupIDs and KeyIDs
	var securityGroupID, sshKeyID string
	if machine.Spec.ProviderSpec.Value != nil && len(machine.Spec.ProviderSpec.Value.Raw) > 0 {
		var providerSpec capiv1beta1.CXMMachineProviderSpec
		if jsonErr := json.Unmarshal(machine.Spec.ProviderSpec.Value.Raw, &providerSpec); jsonErr != nil {
			log.Printf("auto-discover: WARNING: failed to decode ProviderSpec: %v", jsonErr)
		} else {
			if len(providerSpec.SecurityGroupIDs) > 0 {
				securityGroupID = providerSpec.SecurityGroupIDs[0]
			}
			if len(providerSpec.KeyIDs) > 0 {
				sshKeyID = providerSpec.KeyIDs[0]
			}
		}
	}

	ccrPrefix := ccrPrefixForRegion(fullRegion)
	log.Printf("auto-discover: region=%s zoneName=%s zoneID=%s subnetID=%s securityGroupID=%s sshKeyID=%s ccrPrefix=%s",
		fullRegion, zoneName, zoneID, subnetID, securityGroupID, sshKeyID, ccrPrefix)

	if subnetID == "" || securityGroupID == "" {
		panic("auto-discover: could not extract subnetID or securityGroupID from Machine objects; " +
			"check that Machine resources have Spec.SubnetID and ProviderSpec populated")
	}

	// ── 3. Populate environment fields (only if not already set) ─────────────
	if env.Region == "" {
		env.Region = fullRegion
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

	log.Printf("auto-discover: discovery complete (no cloud API calls required)")
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
