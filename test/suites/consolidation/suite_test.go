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

package consolidation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/test/pkg/environment/tke"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

var env *tke.Environment
var nodeClass *v1beta1.TKEMachineNodeClass
var nodePool *karpv1.NodePool

func TestConsolidation(t *testing.T) {
	RegisterFailHandler(Fail)
	env = tke.NewEnvironment(t)
	ginkgo.RunSpecs(t, "Consolidation")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultTKEMachineNodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})

var _ = AfterEach(func() {
	env.Cleanup()
})

var _ = AfterEach(func() {
	env.AfterEach()
})

var _ = Describe("Consolidation", func() {
	Context("Budgets", func() {
		var numPods int32
		var dep *appsv1.Deployment
		var selector labels.Selector

		BeforeEach(func() {
			numPods = 5
			dep = coretest.Deployment(coretest.DeploymentOptions{
				Replicas: numPods,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "consolidation-test"},
					},
					NodeSelector: map[string]string{
						"karpenter-test": "true",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "karpenter-test",
							Operator: corev1.TolerationOpEqual,
							Value:    "true",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					ResourceRequirements: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			})
			selector = labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
		})

		It("should not allow consolidation if the budget is fully blocking", func() {
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes: "0",
			}}

			// Hostname anti-affinity to require one pod on each node
			dep.Spec.Template.Spec.Affinity = &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: dep.Spec.Selector,
							TopologyKey:   corev1.LabelHostname,
						},
					},
				},
			}
			// Raise CPU limit to allow 5 nodes
			nodePool.Spec.Limits = karpv1.Limits(corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50"),
			})

			env.ExpectCreated(nodeClass, nodePool, dep)

			env.EventuallyExpectCreatedNodeClaimCount("==", int(numPods))
			env.EventuallyExpectCreatedNodeCount("==", int(numPods))
			env.EventuallyExpectHealthyPodCount(selector, int(numPods))

			dep.Spec.Replicas = lo.ToPtr[int32](1)
			By("making the nodes empty")
			env.ExpectUpdated(dep)

			// With budget of 0, no consolidation should happen
			env.ConsistentlyExpectNoDisruptions(int(numPods), time.Minute)
		})
	})

	Context("Delete", func() {
		It("should consolidate empty nodes when pods are removed", func() {
			// Create a deployment that spreads across multiple nodes
			numPods := int32(4)
			dep := coretest.Deployment(coretest.DeploymentOptions{
				Replicas: numPods,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "consolidation-delete-test"},
					},
					NodeSelector: map[string]string{
						"karpenter-test": "true",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "karpenter-test",
							Operator: corev1.TolerationOpEqual,
							Value:    "true",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       corev1.LabelHostname,
							WhenUnsatisfiable: corev1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "consolidation-delete-test"},
							},
						},
					},
					ResourceRequirements: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			})
			selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

			// Disable consolidation initially, raise limits
			nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("Never")
			nodePool.Spec.Limits = karpv1.Limits(corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50"),
			})

			env.ExpectCreated(nodeClass, nodePool, dep)

			env.EventuallyExpectHealthyPodCount(selector, int(numPods))
			initialNodes := env.EventuallyExpectCreatedNodeCount(">=", 2)
			initialCount := len(initialNodes)

			// Scale down the deployment to 1 replica
			dep.Spec.Replicas = lo.ToPtr[int32](1)
			env.ExpectUpdated(dep)
			env.EventuallyExpectHealthyPodCount(selector, 1)

			// Enable consolidation
			nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("0s")
			env.ExpectUpdated(nodePool)

			// Expect nodes to be consolidated (at least one removed)
			By(fmt.Sprintf("expecting consolidation to remove nodes from initial count of %d", initialCount))
			env.EventuallyExpectNodeCount("<=", initialCount-1)
		})
	})
})

// Import guard
var _ = fmt.Sprintf
var _ = time.Second
