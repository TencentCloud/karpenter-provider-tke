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

package common

import (
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

// ExpectCreated adds the DiscoveryLabel and creates the objects in the cluster.
func (env *Environment) ExpectCreated(objects ...client.Object) {
	GinkgoHelper()
	for _, obj := range objects {
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[coretest.DiscoveryLabel] = "unspecified"
		obj.SetLabels(labels)
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Create(env.Context, obj)).To(Succeed())
		}).WithTimeout(10 * time.Second).Should(Succeed())
	}
}

// ExpectDeleted deletes the objects with foreground cascade deletion.
func (env *Environment) ExpectDeleted(objects ...client.Object) {
	GinkgoHelper()
	for _, obj := range objects {
		Eventually(func(g Gomega) {
			g.Expect(client.IgnoreNotFound(env.Client.Delete(env.Context, obj,
				client.PropagationPolicy(metav1.DeletePropagationForeground),
				&client.DeleteOptions{GracePeriodSeconds: lo.ToPtr(int64(0))},
			))).To(Succeed())
		}).WithTimeout(10 * time.Second).Should(Succeed())
	}
}

// ExpectUpdated updates the objects in the cluster. It refreshes the resource version
// from the server before updating to avoid conflicts.
func (env *Environment) ExpectUpdated(objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		Eventually(func(g Gomega) {
			current := o.DeepCopyObject().(client.Object)
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(current), current)).To(Succeed())
			o.SetResourceVersion(current.GetResourceVersion())
			g.Expect(env.Client.Update(env.Context, o)).To(Succeed())
		}).WithTimeout(10 * time.Second).Should(Succeed())
	}
}

// EventuallyExpectHealthy waits for all given pods to be Ready.
func (env *Environment) EventuallyExpectHealthy(pods ...*corev1.Pod) {
	GinkgoHelper()
	for _, pod := range pods {
		Eventually(func(g Gomega) {
			p := &corev1.Pod{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(pod), p)).To(Succeed())
			g.Expect(p.Status.Conditions).To(ContainElement(And(
				HaveField("Type", Equal(corev1.PodReady)),
				HaveField("Status", Equal(corev1.ConditionTrue)),
			)))
		}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	}
}

// EventuallyExpectHealthyPodCount waits for a specific number of healthy pods matching a selector.
func (env *Environment) EventuallyExpectHealthyPodCount(selector labels.Selector, numPods int) []*corev1.Pod {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for %d pods matching selector %s to be ready", numPods, selector.String()))
	var pods []*corev1.Pod
	Eventually(func(g Gomega) {
		pods = env.Monitor.RunningPods(selector)
		g.Expect(pods).To(HaveLen(numPods))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return pods
}

// EventuallyExpectBound waits for all given pods to be bound to a node.
func (env *Environment) EventuallyExpectBound(pods ...*corev1.Pod) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, pod := range pods {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			g.Expect(pod.Spec.NodeName).ToNot(BeEmpty())
		}
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
}

// ExpectCreatedNodeCount validates the number of newly created karpenter nodes
// using the Monitor snapshot.
func (env *Environment) ExpectCreatedNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	createdNodes := env.Monitor.CreatedNodes()
	Expect(len(createdNodes)).To(BeNumerically(comparator, count),
		fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
	return createdNodes
}

// EventuallyExpectCreatedNodeCount waits for the created node count to match.
func (env *Environment) EventuallyExpectCreatedNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for created nodes to be %s to %d", comparator, count))
	var createdNodes []*corev1.Node
	Eventually(func(g Gomega) {
		createdNodes = env.Monitor.CreatedNodes()
		g.Expect(len(createdNodes)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return createdNodes
}

// EventuallyExpectInitializedNodeCount waits for the initialized node count to match.
func (env *Environment) EventuallyExpectInitializedNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for initialized nodes to be %s to %d", comparator, count))
	var nodes []*corev1.Node
	Eventually(func(g Gomega) {
		nodes = env.Monitor.InitializedNodes()
		g.Expect(len(nodes)).To(BeNumerically(comparator, count))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return nodes
}

// EventuallyExpectNodeCount waits for the total test node count to match.
func (env *Environment) EventuallyExpectNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for nodes to be %s to %d", comparator, count))
	nodeList := &corev1.NodeList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d nodes, had %d (%v)", count, len(nodeList.Items), NodeNames(lo.ToSlicePtr(nodeList.Items))))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return lo.ToSlicePtr(nodeList.Items)
}

// EventuallyExpectNodeCountWithSelector waits for nodes matching a label selector.
func (env *Environment) EventuallyExpectNodeCountWithSelector(comparator string, count int, selector labels.Selector) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for nodes with selector %v to be %s to %d", selector, comparator, count))
	nodeList := &corev1.NodeList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel}, client.MatchingLabelsSelector{Selector: selector})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d nodes, had %d (%v)", count, len(nodeList.Items), NodeNames(lo.ToSlicePtr(nodeList.Items))))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return lo.ToSlicePtr(nodeList.Items)
}

// ExpectNodeCount asserts the test node count immediately.
func (env *Environment) ExpectNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	nodeList := &corev1.NodeList{}
	Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
	Expect(len(nodeList.Items)).To(BeNumerically(comparator, count))
	return lo.ToSlicePtr(nodeList.Items)
}

// ExpectNodeClaimCount validates the number of NodeClaim objects with the discovery label.
func (env *Environment) ExpectNodeClaimCount(comparator string, count int) []*karpv1.NodeClaim {
	GinkgoHelper()
	ncList := &karpv1.NodeClaimList{}
	Expect(env.Client.List(env.Context, ncList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
	Expect(len(ncList.Items)).To(BeNumerically(comparator, count))
	return lo.ToSlicePtr(ncList.Items)
}

// EventuallyExpectCreatedNodeClaimCount waits for the NodeClaim count to match.
func (env *Environment) EventuallyExpectCreatedNodeClaimCount(comparator string, count int) []*karpv1.NodeClaim {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for created nodeclaims to be %s to %d", comparator, count))
	nodeClaimList := &karpv1.NodeClaimList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeClaimList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeClaimList.Items)).To(BeNumerically(comparator, count))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return lo.Map(nodeClaimList.Items, func(nc karpv1.NodeClaim, _ int) *karpv1.NodeClaim {
		return &nc
	})
}

// EventuallyExpectNodeClaimsReady waits for all given NodeClaims to become ready.
func (env *Environment) EventuallyExpectNodeClaimsReady(nodeClaims ...*karpv1.NodeClaim) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, nc := range nodeClaims {
			temp := &karpv1.NodeClaim{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nc), temp)).Should(Succeed())
			g.Expect(temp.StatusConditions().Root().IsTrue()).To(BeTrue())
		}
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
}

// EventuallyExpectLaunchedNodeClaimCount waits for launched NodeClaim count.
func (env *Environment) EventuallyExpectLaunchedNodeClaimCount(comparator string, count int) []*karpv1.NodeClaim {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for launched nodeclaims to be %s to %d", comparator, count))
	nodeClaimList := &karpv1.NodeClaimList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeClaimList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(lo.CountBy(nodeClaimList.Items, func(nc karpv1.NodeClaim) bool {
			return nc.StatusConditions().IsTrue(karpv1.ConditionTypeLaunched)
		})).To(BeNumerically(comparator, count))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return lo.ToSlicePtr(nodeClaimList.Items)
}

// ConsistentlyExpectNodeCount asserts node count remains stable.
func (env *Environment) ConsistentlyExpectNodeCount(comparator string, count int, duration time.Duration) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("expecting nodes to be %s to %d for %s", comparator, count, duration))
	nodeList := &corev1.NodeList{}
	Consistently(func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(BeNumerically(comparator, count))
	}, duration.String()).Should(Succeed())
	return lo.ToSlicePtr(nodeList.Items)
}

// ConsistentlyExpectNoDisruptions asserts that node count and NodeClaim count stay the same
// and no disruption taints are present.
func (env *Environment) ConsistentlyExpectNoDisruptions(nodeCount int, duration time.Duration) {
	GinkgoHelper()
	Consistently(func(g Gomega) {
		nodeClaimList := &karpv1.NodeClaimList{}
		g.Expect(env.Client.List(env.Context, nodeClaimList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(nodeClaimList.Items).To(HaveLen(nodeCount))
		nodeList := &corev1.NodeList{}
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(nodeList.Items).To(HaveLen(nodeCount))
		nodeList.Items = lo.Filter(nodeList.Items, func(n corev1.Node, _ int) bool {
			_, ok := lo.Find(n.Spec.Taints, func(t corev1.Taint) bool {
				return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
			})
			return ok
		})
		g.Expect(nodeList.Items).To(HaveLen(0))
	}, duration).Should(Succeed())
}

// EventuallyExpectTaintedNodeCount waits for a number of disruption-tainted nodes.
func (env *Environment) EventuallyExpectTaintedNodeCount(comparator string, count int) []*corev1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for tainted nodes to be %s to %d", comparator, count))
	var taintedNodes []*corev1.Node
	Eventually(func(g Gomega) {
		nodeList := &corev1.NodeList{}
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		taintedNodes = nil
		for i := range nodeList.Items {
			_, ok := lo.Find(nodeList.Items[i].Spec.Taints, func(t corev1.Taint) bool {
				return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
			})
			if ok {
				taintedNodes = append(taintedNodes, &nodeList.Items[i])
			}
		}
		g.Expect(len(taintedNodes)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d tainted nodes, had %d", count, len(taintedNodes)))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return taintedNodes
}

// EventuallyExpectDrifted waits for NodeClaims to be marked as drifted.
func (env *Environment) EventuallyExpectDrifted(nodeClaims ...*karpv1.NodeClaim) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, nc := range nodeClaims {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nc), nc)).To(Succeed())
			g.Expect(nc.StatusConditions().Get(karpv1.ConditionTypeDrifted).IsTrue()).To(BeTrue())
		}
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
}

// EventuallyExpectConsolidatable waits for NodeClaims to be consolidatable.
func (env *Environment) EventuallyExpectConsolidatable(nodeClaims ...*karpv1.NodeClaim) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, nc := range nodeClaims {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nc), nc)).To(Succeed())
			g.Expect(nc.StatusConditions().Get(karpv1.ConditionTypeConsolidatable).IsTrue()).To(BeTrue())
		}
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
}

// EventuallyExpectNotFound waits for the given objects to be deleted.
func (env *Environment) EventuallyExpectNotFound(objects ...client.Object) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, object := range objects {
			err := env.Client.Get(env.Context, client.ObjectKeyFromObject(object), object)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
}

// ConsistentlyExpectPendingPods asserts that pods remain in Pending phase.
func (env *Environment) ConsistentlyExpectPendingPods(duration time.Duration, pods ...*corev1.Pod) {
	GinkgoHelper()
	By(fmt.Sprintf("expecting %d pods to be pending for %s", len(pods), duration))
	Consistently(func(g Gomega) {
		for _, pod := range pods {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodPending))
		}
	}, duration.String()).Should(Succeed())
}

// ExpectNoCrashes checks that no karpenter controller pods have restarted.
func (env *Environment) ExpectNoCrashes() {
	GinkgoHelper()
	for k, v := range env.Monitor.RestartCount("kube-system") {
		if strings.Contains(k, "karpenter") && v > 0 {
			Fail("expected karpenter containers to not crash")
		}
	}
}

// ExpectExists verifies that the object exists in the cluster and returns it.
func (env *Environment) ExpectExists(obj client.Object) client.Object {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).WithTimeout(5 * time.Second).Should(Succeed())
	return obj
}

// GetNode returns the node with the given name.
func (env *Environment) GetNode(nodeName string) corev1.Node {
	GinkgoHelper()
	var node corev1.Node
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: nodeName}, &node)).To(Succeed())
	return node
}

// ExpectActivePodsForNode returns active (non-terminating) test pods on a node.
func (env *Environment) ExpectActivePodsForNode(nodeName string) []*corev1.Pod {
	GinkgoHelper()
	podList := &corev1.PodList{}
	Expect(env.Client.List(env.Context, podList, client.MatchingFields{"spec.nodeName": nodeName}, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
	return lo.Filter(lo.ToSlicePtr(podList.Items), func(p *corev1.Pod, _ int) bool {
		return p.DeletionTimestamp.IsZero()
	})
}

// EventuallyExpectBoundPodCount waits for a number of bound pods matching a selector.
func (env *Environment) EventuallyExpectBoundPodCount(selector labels.Selector, numPods int) []*corev1.Pod {
	GinkgoHelper()
	var res []*corev1.Pod
	Eventually(func(g Gomega) {
		res = []*corev1.Pod{}
		podList := &corev1.PodList{}
		g.Expect(env.Client.List(env.Context, podList, client.MatchingLabelsSelector{Selector: selector})).To(Succeed())
		for i := range podList.Items {
			if podList.Items[i].Spec.NodeName != "" {
				res = append(res, &podList.Items[i])
			}
		}
		g.Expect(res).To(HaveLen(numPods))
	}).WithTimeout(env.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
	return res
}

// NodeNames extracts names from a slice of nodes.
func NodeNames(nodes []*corev1.Node) []string {
	return lo.Map(nodes, func(n *corev1.Node, _ int) string { return n.Name })
}

// NodeClaimNames extracts names from a slice of NodeClaims.
func NodeClaimNames(nodeClaims []*karpv1.NodeClaim) []string {
	return lo.Map(nodeClaims, func(n *karpv1.NodeClaim, _ int) string { return n.Name })
}
