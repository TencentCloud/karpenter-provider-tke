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
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	v1beta1api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

// This suite verifies the fix for the Machine ↔ Node deletion deadlock:
// karpenter adds karpenter.sh/termination to the Node during registration; the TKE
// Machine controller blocks Machine finalization until the Node is deleted. Without
// the provider-side fix, the karpenter finalizer would only be removed after the
// cloudprovider reports the instance as gone — which itself was blocked on Node
// deletion. The provider's Delete path now strips this finalizer up front, and the
// GC controller cleans up any historical stragglers.
var _ = Describe("StuckDeletion", func() {
	It("should drop karpenter.sh/termination from the Node and finish deletion without deadlocking", func() {
		env.ExpectCreated(nodeClass, nodePool)

		// Trigger provisioning with a single pod.
		pod := env.TestPod()
		env.ExpectCreated(pod)
		env.EventuallyExpectHealthy(pod)

		nodes := env.EventuallyExpectCreatedNodeCount("==", 1)
		Expect(nodes).To(HaveLen(1))
		env.EventuallyExpectInitializedNodeCount("==", 1)

		createdNode := nodes[0]

		// karpenter registers the node with the termination finalizer — sanity-check it's present
		// before we tear things down. If this expectation ever fails it likely means upstream
		// karpenter changed its finalizer string, in which case our provider-side fix
		// (which targets the same constant) also needs to be revisited.
		Eventually(func(g Gomega) {
			n := &corev1.Node{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(createdNode), n)).To(Succeed())
			g.Expect(slices.Contains(n.Finalizers, karpv1.TerminationFinalizer)).To(BeTrue(),
				"expected karpenter.sh/termination on Node %s, got finalizers=%v", n.Name, n.Finalizers)
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		// Resolve the underlying Machine via the NodeClaim's owned-machine annotation so we can
		// also observe the Machine side of the deadlock window.
		var ownedMachineName string
		Eventually(func(g Gomega) {
			ncList := &karpv1.NodeClaimList{}
			g.Expect(env.Client.List(env.Context, ncList)).To(Succeed())
			for i := range ncList.Items {
				if ncList.Items[i].Status.NodeName == createdNode.Name {
					ownedMachineName = ncList.Items[i].Annotations[v1beta1api.AnnotationOwnedMachine]
					break
				}
			}
			g.Expect(ownedMachineName).ToNot(BeEmpty(), "could not resolve owned-machine for node %s", createdNode.Name)
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		// Trigger deletion by removing the NodeClaim — this is the canonical karpenter teardown
		// path and exercises both the NodeClaim lifecycle controller and the Node termination
		// controller, both of which transitively call cloudProvider.Delete.
		ncList := &karpv1.NodeClaimList{}
		Expect(env.Client.List(env.Context, ncList)).To(Succeed())
		var targetNC *karpv1.NodeClaim
		for i := range ncList.Items {
			if ncList.Items[i].Status.NodeName == createdNode.Name {
				targetNC = &ncList.Items[i]
				break
			}
		}
		Expect(targetNC).ToNot(BeNil(), "expected to find NodeClaim for node %s", createdNode.Name)
		env.ExpectDeleted(targetNC)

		// The deadlock would have caused Machine + Node to linger indefinitely; with the fix
		// applied, deletion should finish well inside 5 minutes (the GC stuckMachineDeletionThreshold).
		Eventually(func(g Gomega) {
			n := &corev1.Node{}
			err := env.Client.Get(env.Context, client.ObjectKeyFromObject(createdNode), n)
			g.Expect(errors.IsNotFound(err)).To(BeTrue(),
				"expected Node %s to be deleted within deadline; still present (deletionTimestamp=%v, finalizers=%v)",
				createdNode.Name, n.DeletionTimestamp, n.Finalizers)
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			m := &capiv1beta1.Machine{}
			err := env.Client.Get(env.Context, client.ObjectKey{Name: ownedMachineName}, m)
			g.Expect(errors.IsNotFound(err)).To(BeTrue(),
				"expected Machine %s to be deleted within deadline; still present (deletionTimestamp=%v)",
				ownedMachineName, m.DeletionTimestamp)
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		env.ExpectNoCrashes()
	})
})
