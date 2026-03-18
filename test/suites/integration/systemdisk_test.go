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
	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var _ = Describe("SystemDisk", func() {
	Context("with encryption annotation", func() {
		It("should create a Machine with encrypted system disk", func() {
			// Add encrypt annotation to NodeClass
			annotations := nodeClass.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[v1beta1.AnnotationSystemDiskEncryptKey] = "ENCRYPT"
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

			// Verify the Machine has encrypted system disk
			env.ExpectMachineSystemDiskEncryption(machineName, "ENCRYPT")
		})
	})

	Context("without encryption annotation", func() {
		It("should create a Machine with unencrypted system disk", func() {
			// No encrypt annotation on NodeClass

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

			// Verify the Machine has no system disk encryption
			env.ExpectMachineSystemDiskEncryption(machineName, "")
		})
	})
})
