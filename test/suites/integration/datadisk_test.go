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

package integration_test

import (
	"fmt"
	"time"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var _ = Describe("DataDisk", func() {
	Context("with encryption annotation", func() {
		It("should create a Machine with encrypted data disk", func() {
			// Add data disk to NodeClass
			nodeClass.Spec.DataDisks = []v1beta1.DataDisk{
				{
					Size: 50,
					Type: v1beta1.DiskTypeCloudPremium,
				},
			}
			// Add encrypt annotation to NodeClass
			annotations := nodeClass.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[v1beta1.AnnotationDataDisksEncryptKey] = "0=ENCRYPT"
			nodeClass.SetAnnotations(annotations)

			env.ExpectCreated(nodeClass, nodePool)

			// Create a test pod to trigger provisioning
			pod := env.TestPod()
			env.ExpectCreated(pod)

			// Wait for the pod to be healthy and a node to be created
			env.EventuallyExpectHealthy(pod)
			nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
			env.EventuallyExpectInitializedNodeCount("==", 1)

			// Find the Machine name from the NodeClaim
			machineName := findMachineNameFromNode(nodes[0].Name)
			Expect(machineName).ToNot(BeEmpty(), "could not find Machine name for node %s", nodes[0].Name)

			// Verify the Machine has encrypted data disk
			env.ExpectMachineDataDiskEncryption(machineName, "ENCRYPT")
		})
	})

	Context("without encryption annotation", func() {
		It("should create a Machine with unencrypted data disk", func() {
			// Add data disk to NodeClass without encrypt annotation
			nodeClass.Spec.DataDisks = []v1beta1.DataDisk{
				{
					Size: 50,
					Type: v1beta1.DiskTypeCloudPremium,
				},
			}

			env.ExpectCreated(nodeClass, nodePool)

			// Create a test pod to trigger provisioning
			pod := env.TestPod()
			env.ExpectCreated(pod)

			// Wait for the pod to be healthy and a node to be created
			env.EventuallyExpectHealthy(pod)
			nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
			env.EventuallyExpectInitializedNodeCount("==", 1)

			// Find the Machine name from the NodeClaim
			machineName := findMachineNameFromNode(nodes[0].Name)
			Expect(machineName).ToNot(BeEmpty(), "could not find Machine name for node %s", nodes[0].Name)

			// Verify the Machine has no encryption
			env.ExpectMachineDataDiskEncryption(machineName, "")
		})
	})
})

// findMachineNameFromNode looks up the Machine name from the node's annotations or
// from NodeClaim references.
func findMachineNameFromNode(nodeName string) string {
	// Look for the machine name from node annotation
	nodeObj := &v1beta1.TKEMachineNodeClass{}
	_ = nodeObj // suppress unused

	// Try to find Machine name from NodeClaim associated with the node
	ncList := &karpv1.NodeClaimList{}
	if err := env.Client.List(env.Context, ncList); err != nil {
		return ""
	}
	for i := range ncList.Items {
		nc := &ncList.Items[i]
		if nc.Status.NodeName == nodeName {
			// The machine name is often the same as the NodeClaim name or stored in annotations
			if machName, ok := nc.Annotations[v1beta1.AnnotationOwnedMachine]; ok {
				return machName
			}
			return nc.Name
		}
	}
	return ""
}

// findMachineNameWithRetry retries finding the machine name
func findMachineNameWithRetry(nodeName string) string {
	var machineName string
	Eventually(func(g Gomega) {
		machineName = findMachineNameFromNode(nodeName)
		g.Expect(machineName).ToNot(BeEmpty(),
			fmt.Sprintf("Machine name not found for node %s", nodeName))
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	return machineName
}

// keep imports used
var _ client.Client
