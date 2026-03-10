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

package storage_test

import (
	"fmt"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	storagev1 "k8s.io/api/storage/v1"
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

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	env = tke.NewEnvironment(t)
	ginkgo.RunSpecs(t, "Storage")
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

var _ = Describe("Static Persistent Volumes", func() {
	It("should run a pod with a pre-bound persistent volume (empty storage class)", func() {
		// Create PV directly (coretest.PersistentVolume uses NamespacedObjectMeta which
		// adds a namespace to PVs, but PVs are cluster-scoped)
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-pv-" + coretest.RandomName(),
				Labels: map[string]string{coretest.DiscoveryLabel: "unspecified"},
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp",
					},
				},
				StorageClassName: "",
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Capacity:         corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		}
		pvc := coretest.PersistentVolumeClaim(coretest.PersistentVolumeClaimOptions{
			VolumeName:       pv.Name,
			StorageClassName: lo.ToPtr(""),
		})
		pod := coretest.Pod(coretest.PodOptions{
			PersistentVolumeClaims: []string{pvc.Name},
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
		})

		env.ExpectCreated(nodeClass, nodePool, pv, pvc, pod)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
	})

	It("should run a pod with a pre-bound persistent volume (non-existent storage class)", func() {
		Skip("non-existent storage class PV binding is unreliable on TKE")
	})
})

var _ = Describe("Dynamic Persistent Volumes", func() {
	var storageClass *storagev1.StorageClass

	BeforeEach(func() {
		// Check if CBS CSI driver is installed
		var ds appsv1.DaemonSet
		if err := env.Client.Get(env.Context, client.ObjectKey{
			Namespace: "kube-system",
			Name:      "cbs-csi-node",
		}, &ds); err != nil {
			if errors.IsNotFound(err) {
				Skip(fmt.Sprintf("skipping dynamic PVC test due to missing CBS CSI driver: %s", err))
			} else {
				Fail(fmt.Sprintf("determining CBS CSI driver status: %s", err))
			}
		}
		storageClass = coretest.StorageClass(coretest.StorageClassOptions{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cbs-storage-class",
			},
			Provisioner:       lo.ToPtr("com.tencent.cloud.csi.cbs"),
			VolumeBindingMode: lo.ToPtr(storagev1.VolumeBindingWaitForFirstConsumer),
		})
	})

	It("should run a pod with a dynamic persistent volume", func() {
		pvc := coretest.PersistentVolumeClaim(coretest.PersistentVolumeClaimOptions{
			StorageClassName: &storageClass.Name,
		})
		pod := coretest.Pod(coretest.PodOptions{
			PersistentVolumeClaims: []string{pvc.Name},
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
		})

		env.ExpectCreated(nodeClass, nodePool, storageClass, pvc, pod)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
	})

	It("should run a pod with a generic ephemeral volume", func() {
		pod := coretest.Pod(coretest.PodOptions{
			EphemeralVolumeTemplates: []coretest.EphemeralVolumeTemplateOptions{{
				StorageClassName: &storageClass.Name,
			}},
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
		})

		env.ExpectCreated(nodeClass, nodePool, storageClass, pod)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
	})
})

var _ = Describe("StatefulSet", func() {
	It("should not block node deletion if stateful workload cannot be drained", func() {
		var storageClass *storagev1.StorageClass

		// Check if CBS CSI driver is installed
		var ds appsv1.DaemonSet
		if err := env.Client.Get(env.Context, client.ObjectKey{
			Namespace: "kube-system",
			Name:      "cbs-csi-node",
		}, &ds); err != nil {
			if errors.IsNotFound(err) {
				Skip(fmt.Sprintf("skipping StatefulSet test due to missing CBS CSI driver: %s", err))
			} else {
				Fail(fmt.Sprintf("determining CBS CSI driver status: %s", err))
			}
		}

		storageClass = coretest.StorageClass(coretest.StorageClassOptions{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cbs-statefulset-class",
			},
			Provisioner:       lo.ToPtr("com.tencent.cloud.csi.cbs"),
			VolumeBindingMode: lo.ToPtr(storagev1.VolumeBindingWaitForFirstConsumer),
		})

		numPods := 1
		statefulSet := coretest.StatefulSet(coretest.StatefulSetOptions{
			Replicas: int32(numPods),
			PodOptions: coretest.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "statefulset-test"},
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
					// Tolerate disruption taint to make pod un-drain-able
					{
						Key:      "karpenter.sh/disruption",
						Operator: corev1.TolerationOpEqual,
						Value:    "disrupting",
						Effect:   corev1.TaintEffectNoExecute,
					},
				},
			},
		})

		pvc := coretest.PersistentVolumeClaim(coretest.PersistentVolumeClaimOptions{
			StorageClassName: &storageClass.Name,
		})
		statefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{*pvc}
		selector := labels.SelectorFromSet(statefulSet.Spec.Selector.MatchLabels)

		env.ExpectCreated(nodeClass, nodePool, storageClass, statefulSet)
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
		env.EventuallyExpectHealthyPodCount(selector, numPods)

		// Delete the NodeClaim to trigger disruption
		env.ExpectDeleted(nodeClaim)

		// The node should eventually be removed even though the pod tolerates disruption
		env.EventuallyExpectNotFound(nodeClaim, node)
	})
})

// Import guards
var _ = fmt.Sprintf
