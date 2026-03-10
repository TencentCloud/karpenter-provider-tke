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

package lifecycle_test

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var _ = Describe("Expiry", func() {
	It("should terminate the node after TTL expiration", func() {
		// Set NodePool expireAfter to 300s (5 minutes)
		nodePool.Spec.Template.Spec.ExpireAfter = karpv1.MustParseNillableDuration("300s")

		env.ExpectCreated(nodeClass, nodePool)

		// Create a test pod to trigger provisioning
		pod := env.TestPod()
		env.ExpectCreated(pod)

		// Wait for the pod to be healthy and a node to be created
		env.EventuallyExpectHealthy(pod)
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))
		env.EventuallyExpectInitializedNodeCount("==", 1)

		createdNode := nodes[0]

		// Wait for the node to be deleted due to TTL expiration
		// The node should be removed after ~5 minutes (300s) + buffer for drain
		Eventually(func(g Gomega) {
			n := &corev1.Node{}
			err := env.Client.Get(env.Context, client.ObjectKey{Name: createdNode.Name}, n)
			g.Expect(err).ToNot(Succeed(), "expected node %s to be deleted due to TTL expiry", createdNode.Name)
		}).WithTimeout(10 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})
})
