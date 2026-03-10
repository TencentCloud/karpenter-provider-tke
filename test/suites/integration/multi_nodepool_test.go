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
	corev1 "k8s.io/api/core/v1"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var _ = Describe("MultiNodePool", func() {
	It("should fall back to a valid NodePool when the first one uses a non-existent instance type", func() {
		// Create an invalid NodePool with a non-existent instance type
		invalidNodePool := env.DefaultNodePool(nodeClass)
		invalidNodePool = coretest.ReplaceRequirements(invalidNodePool, karpv1.NodeSelectorRequirementWithMinValues{
			Key:      "node.kubernetes.io/instance-type",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"NONEXISTENT.FAKETYPE"},
		})

		// Create the valid NodePool (the default one)
		validNodePool := nodePool

		env.ExpectCreated(nodeClass, invalidNodePool, validNodePool)

		// Create a test pod to trigger provisioning
		pod := env.TestPod()
		env.ExpectCreated(pod)

		// Wait for the pod to be healthy
		env.EventuallyExpectHealthy(pod)

		// Verify a node was created by the valid NodePool
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))
		Expect(nodes[0].Labels[karpv1.NodePoolLabelKey]).To(Equal(validNodePool.Name))
	})
})
