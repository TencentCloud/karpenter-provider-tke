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
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

// spotInterruptionTaintKey mirrors the taint key that
// pkg/controllers/interruption watches for. Keeping it as a literal here (not
// importing from the controller package) makes the test double as a guard
// against accidental drift in either place.
const spotInterruptionTaintKey = "tke-node-termination-handler/spot-itn"

var _ = Describe("SpotInterruption", func() {
	// This test exercises the interruption controller end-to-end:
	//   1. Provision a node via a NodePool.
	//   2. Simulate what tke-node-termination-handler does on real spot
	//      reclaim: patch the taint tke-node-termination-handler/spot-itn
	//      onto the node.
	//   3. Assert the controller deletes the NodeClaim → Karpenter core's
	//      termination controller cordons and drains → NodeClaim disappears.
	//
	// We use an on-demand NodePool for E2E hygiene (spot availability isn't
	// guaranteed and would make the test flaky); the controller only gates
	// on the taint, not on capacity type, so on-demand exercises the same
	// code path.
	It("should delete the NodeClaim when the spot-itn taint appears on its Node", func() {
		env.ExpectCreated(nodeClass, nodePool)

		// Provision a node by scheduling a pod.
		pod := env.TestPod()
		env.ExpectCreated(pod)
		env.EventuallyExpectHealthy(pod)
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))
		nodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		Expect(nodeClaims).To(HaveLen(1))
		createdNodeClaim := nodeClaims[0]
		createdNode := nodes[0]

		By("applying the spot-itn taint to simulate a TKE spot reclaim signal")
		Eventually(func(g Gomega) {
			n := &corev1.Node{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKey{Name: createdNode.Name}, n)).To(Succeed())
			// Idempotency: skip if the taint is already present. In practice
			// this branch only matters on retries — the first attempt should
			// always add it.
			for _, t := range n.Spec.Taints {
				if t.Key == spotInterruptionTaintKey {
					return
				}
			}
			stored := n.DeepCopy()
			n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{
				Key:    spotInterruptionTaintKey,
				Effect: corev1.TaintEffectNoSchedule,
			})
			g.Expect(env.Client.Patch(env.Context, n, client.MergeFrom(stored))).To(Succeed())
		}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		By("waiting for the NodeClaim to be deleted by the interruption controller")
		// The interruption controller should:
		//   - See the taint via its Node watch predicate
		//   - Look up the NodeClaim by providerID
		//   - Call Delete on the NodeClaim
		// Karpenter core's termination controller then cordons+drains and
		// finalizes the NodeClaim after the cloud instance is torn down.
		// Give the whole flow the default timeout (16 min) — drain of a
		// single test pod without a PDB should finish well within that.
		Eventually(func(g Gomega) {
			nc := &karpv1.NodeClaim{}
			err := env.Client.Get(env.Context, client.ObjectKey{Name: createdNodeClaim.Name}, nc)
			if errors.IsNotFound(err) {
				return // finalized and removed
			}
			g.Expect(err).ToNot(HaveOccurred())
			// If it still exists, at minimum a DeletionTimestamp must have
			// been set — that's the immediate observable effect of our
			// controller calling Delete.
			g.Expect(nc.DeletionTimestamp.IsZero()).To(BeFalse(),
				"expected NodeClaim %s to be deleted or have a DeletionTimestamp after spot-itn taint was applied",
				nc.Name)
		}).WithTimeout(env.DefaultTimeout).WithPolling(10 * time.Second).Should(Succeed())

		By("verifying the SpotInterrupted event was recorded on the NodeClaim")
		// The interruption controller publishes a Warning event with reason
		// "SpotInterrupted" before deleting the NodeClaim. Because event
		// recording happens synchronously in the same reconcile, and events
		// are namespaced (default), we look them up via a list. Events for
		// NodeClaim survive past NodeClaim deletion for their default TTL
		// (1h), so this check remains valid even after the NodeClaim itself
		// is gone.
		Eventually(func(g Gomega) {
			eventList := &corev1.EventList{}
			g.Expect(env.Client.List(env.Context, eventList,
				client.MatchingFields{"involvedObject.kind": "NodeClaim"},
			)).To(Succeed())
			found := false
			for i := range eventList.Items {
				e := &eventList.Items[i]
				if e.InvolvedObject.Name != createdNodeClaim.Name {
					continue
				}
				if e.Reason == "SpotInterrupted" {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"expected a SpotInterrupted event on NodeClaim %s", createdNodeClaim.Name)
		}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})

	// Negative check: an unrelated taint must NOT trigger drain. Guards
	// against an over-broad predicate.
	It("should not delete a NodeClaim when an unrelated taint is applied", func() {
		env.ExpectCreated(nodeClass, nodePool)

		pod := env.TestPod()
		env.ExpectCreated(pod)
		env.EventuallyExpectHealthy(pod)
		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))
		nodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		Expect(nodeClaims).To(HaveLen(1))
		createdNodeClaim := nodeClaims[0]
		createdNode := nodes[0]

		By("applying an unrelated taint")
		Eventually(func(g Gomega) {
			n := &corev1.Node{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKey{Name: createdNode.Name}, n)).To(Succeed())
			stored := n.DeepCopy()
			n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{
				Key:    "example.com/some-other-taint",
				Effect: corev1.TaintEffectNoSchedule,
			})
			g.Expect(env.Client.Patch(env.Context, n, client.MergeFrom(stored))).To(Succeed())
		}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		By("consistently verifying the NodeClaim is not deleted for 90 seconds")
		Consistently(func(g Gomega) {
			nc := &karpv1.NodeClaim{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKey{Name: createdNodeClaim.Name}, nc)).To(Succeed())
			g.Expect(nc.DeletionTimestamp.IsZero()).To(BeTrue(),
				"NodeClaim %s should not be deleted for an unrelated taint", nc.Name)
		}, 90*time.Second, 10*time.Second).Should(Succeed())
	})
})
