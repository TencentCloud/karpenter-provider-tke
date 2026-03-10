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
	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var _ = Describe("HappyPath", func() {
	It("should provision a node and then scale down when the pod is deleted", func() {
		// Create NodeClass and NodePool
		env.ExpectCreated(nodeClass, nodePool)

		// Create a test pod that triggers node provisioning
		pod := env.TestPod()
		env.ExpectCreated(pod)

		// Wait for the pod to be healthy
		env.EventuallyExpectHealthy(pod)

		// Verify exactly 1 new node was created
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))

		// Wait for the node to be initialized
		env.EventuallyExpectInitializedNodeCount("==", 1)

		// Delete the pod and wait for the node to be reclaimed
		env.ExpectDeleted(pod)
		env.EventuallyExpectNotFound(pod)

		// The scale down will happen in Cleanup()
	})
})
