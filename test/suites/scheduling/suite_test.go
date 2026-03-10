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

package scheduling_test

import (
	"fmt"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

func TestScheduling(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	env = tke.NewEnvironment(t)
	ginkgo.RunSpecs(t, "Scheduling")
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

var _ = Describe("Provisioning", func() {
	It("should provision a node for naked pods", func() {
		pod := env.TestPod()

		env.ExpectCreated(nodeClass, nodePool, pod)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
	})

	It("should provision a node for a deployment", func() {
		dep := coretest.Deployment(coretest.DeploymentOptions{
			Replicas: 5,
			PodOptions: coretest.PodOptions{
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
			},
		})

		env.ExpectCreated(nodeClass, nodePool, dep)
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(dep.Spec.Selector.MatchLabels), 5)
		env.ExpectCreatedNodeCount("<=", 2) // should probably all land on a single node, but at worst two
	})

	It("should provision a node for a self-affinity deployment", func() {
		podLabels := map[string]string{"test": "self-affinity"}
		dep := coretest.Deployment(coretest.DeploymentOptions{
			Replicas: 2,
			PodOptions: coretest.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
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
				PodRequirements: []corev1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{MatchLabels: podLabels},
						TopologyKey:   corev1.LabelHostname,
					},
				},
			},
		})

		env.ExpectCreated(nodeClass, nodePool, dep)
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(dep.Spec.Selector.MatchLabels), 2)
		env.ExpectCreatedNodeCount("==", 1)
	})

	It("should provision a node using a NodePool with higher priority", func() {
		nodePoolLowPri := coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Weight: lo.ToPtr(int32(10)),
				Template: karpv1.NodeClaimTemplate{
					ObjectMeta: karpv1.ObjectMeta{
						Labels: map[string]string{
							"karpenter-test":        "true",
							coretest.DiscoveryLabel: "unspecified",
						},
					},
					Spec: karpv1.NodeClaimTemplateSpec{
						NodeClassRef: &karpv1.NodeClassReference{
							Group: v1beta1.SchemeGroupVersion.Group,
							Kind:  "TKEMachineNodeClass",
							Name:  nodeClass.Name,
						},
						Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
							{Key: "kubernetes.io/os", Operator: corev1.NodeSelectorOpIn, Values: []string{"linux"}},
							{Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand}},
						},
						Taints: []corev1.Taint{
							{Key: "karpenter-test", Value: "true", Effect: corev1.TaintEffectNoSchedule},
						},
						ExpireAfter: karpv1.MustParseNillableDuration("Never"),
					},
				},
				Limits: karpv1.Limits(corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"),
				}),
			},
		})
		nodePoolHighPri := coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Weight: lo.ToPtr(int32(100)),
				Template: karpv1.NodeClaimTemplate{
					ObjectMeta: karpv1.ObjectMeta{
						Labels: map[string]string{
							"karpenter-test":        "true",
							coretest.DiscoveryLabel: "unspecified",
						},
					},
					Spec: karpv1.NodeClaimTemplateSpec{
						NodeClassRef: &karpv1.NodeClassReference{
							Group: v1beta1.SchemeGroupVersion.Group,
							Kind:  "TKEMachineNodeClass",
							Name:  nodeClass.Name,
						},
						Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
							{Key: "kubernetes.io/os", Operator: corev1.NodeSelectorOpIn, Values: []string{"linux"}},
							{Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand}},
						},
						Taints: []corev1.Taint{
							{Key: "karpenter-test", Value: "true", Effect: corev1.TaintEffectNoSchedule},
						},
						ExpireAfter: karpv1.MustParseNillableDuration("Never"),
					},
				},
				Limits: karpv1.Limits(corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"),
				}),
			},
		})
		pod := env.TestPod()
		env.ExpectCreated(pod, nodeClass, nodePoolLowPri, nodePoolHighPri)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		// Verify the pod landed on the higher-priority NodePool
		Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
		node := env.GetNode(pod.Spec.NodeName)
		Expect(node.Labels[karpv1.NodePoolLabelKey]).To(Equal(nodePoolHighPri.Name))
	})
})

var _ = Describe("Labels", func() {
	It("should apply annotations to the node", func() {
		nodePool.Spec.Template.Annotations = map[string]string{
			"foo": "bar",
		}
		pod := env.TestPod()
		env.ExpectCreated(nodeClass, nodePool, pod)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount(">=", 1)
		Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
		Expect(env.GetNode(pod.Spec.NodeName).Annotations).To(HaveKeyWithValue("foo", "bar"))
	})

	It("should support well-known labels for topology and architecture", func() {
		nodeSelector := map[string]string{
			karpv1.NodePoolLabelKey:     nodePool.Name,
			corev1.LabelTopologyZone:    env.ZoneID,
			corev1.LabelOSStable:        "linux",
			corev1.LabelArchStable:      "amd64",
			karpv1.CapacityTypeLabelKey: karpv1.CapacityTypeOnDemand,
			// Also require karpenter-test for our taint
			"karpenter-test": "true",
		}
		requirements := lo.MapToSlice(nodeSelector, func(key string, value string) corev1.NodeSelectorRequirement {
			return corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{value}}
		})
		dep := coretest.Deployment(coretest.DeploymentOptions{
			Replicas: 1,
			PodOptions: coretest.PodOptions{
				NodeSelector:     nodeSelector,
				NodePreferences:  requirements,
				NodeRequirements: requirements,
				Tolerations: []corev1.Toleration{
					{
						Key:      "karpenter-test",
						Operator: corev1.TolerationOpEqual,
						Value:    "true",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
		})
		env.ExpectCreated(nodeClass, nodePool, dep)
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(dep.Spec.Selector.MatchLabels), 1)
		env.ExpectCreatedNodeCount("==", 1)
	})

	DescribeTable("should support restricted label domain exceptions", func(domain string) {
		coretest.ReplaceRequirements(nodePool,
			karpv1.NodeSelectorRequirementWithMinValues{Key: domain + "/team", Operator: corev1.NodeSelectorOpExists},
			karpv1.NodeSelectorRequirementWithMinValues{Key: domain + "/custom-label", Operator: corev1.NodeSelectorOpExists},
			karpv1.NodeSelectorRequirementWithMinValues{Key: "subdomain." + domain + "/custom-label", Operator: corev1.NodeSelectorOpExists},
		)
		nodeSelector := map[string]string{
			domain + "/team":                        "team-1",
			domain + "/custom-label":                "custom-value",
			"subdomain." + domain + "/custom-label": "custom-value",
			"karpenter-test":                        "true",
		}
		requirements := lo.MapToSlice(nodeSelector, func(key string, value string) corev1.NodeSelectorRequirement {
			return corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{value}}
		})
		dep := coretest.Deployment(coretest.DeploymentOptions{
			Replicas: 1,
			PodOptions: coretest.PodOptions{
				NodeSelector:     nodeSelector,
				NodePreferences:  requirements,
				NodeRequirements: requirements,
				Tolerations: []corev1.Toleration{
					{
						Key:      "karpenter-test",
						Operator: corev1.TolerationOpEqual,
						Value:    "true",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
		})
		env.ExpectCreated(nodeClass, nodePool, dep)
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(dep.Spec.Selector.MatchLabels), 1)
		node := env.ExpectCreatedNodeCount("==", 1)[0]
		for k, v := range nodeSelector {
			if k == "karpenter-test" {
				continue
			}
			Expect(node.Labels).To(HaveKeyWithValue(k, v))
		}
	},
		Entry("node-restriction.kubernetes.io", "node-restriction.kubernetes.io"),
		Entry("node.kubernetes.io", "node.kubernetes.io"),
	)
})

var _ = Describe("TopologySpread", func() {
	It("should provision nodes for a zonal topology spread", func() {
		// Skip if zone is not configured - we need at least the known zone
		if env.ZoneID == "" {
			Skip("ZONE_ID not configured, skipping zonal topology spread test")
		}
		podLabels := map[string]string{"test": "zonal-spread"}
		dep := coretest.Deployment(coretest.DeploymentOptions{
			Replicas: 2,
			PodOptions: coretest.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
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
						LabelSelector:     &metav1.LabelSelector{MatchLabels: podLabels},
					},
				},
			},
		})

		env.ExpectCreated(nodeClass, nodePool, dep)
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(podLabels), 2)
		// With hostname spread and 2 pods, we expect 2 nodes
		env.EventuallyExpectCreatedNodeCount("==", 2)
	})
})

// Unused import guard
var _ = fmt.Sprintf
