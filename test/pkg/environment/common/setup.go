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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/test/pkg/debug"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

// namespacedCleanableObjects are objects cleaned up via DeleteAllOf in each namespace.
var namespacedCleanableObjects = []client.Object{
	&corev1.Pod{},
	&appsv1.Deployment{},
	&appsv1.StatefulSet{},
	&policyv1.PodDisruptionBudget{},
	&corev1.PersistentVolumeClaim{},
	&storagev1.StorageClass{},
}

func (env *Environment) BeforeEach() {
	debug.BeforeEach(env.Context, env.Config, env.Client)
	env.Monitor.Reset()
	env.ExpectCleanCluster()
}

func (env *Environment) Cleanup() {
	// 1. Delete application resources (pods, deployments, etc.)
	env.cleanupNamespacedObjects()
	// 2. Delete NodePool - this triggers Karpenter to decommission nodes
	env.deleteTestNodePools()
	// 3. Wait for NodeClaims to be fully removed (indicates nodes are gone)
	env.eventuallyExpectNodeClaimsGone()
	// 4. Delete TKEMachineNodeClass
	env.deleteTestNodeClasses()
}

func (env *Environment) AfterEach() {
	debug.AfterEach(env.Context)
}

func (env *Environment) ExpectCleanCluster() {
	// Wait for any leftover test resources from a previous test to be cleaned up.
	// This can happen when running multiple tests sequentially and the previous
	// test's cleanup is still completing asynchronously.
	Eventually(func(g Gomega) {
		nodePoolList := &karpv1.NodePoolList{}
		g.Expect(env.Client.List(env.Context, nodePoolList)).To(Succeed())
		for _, np := range nodePoolList.Items {
			if _, ok := np.Labels[coretest.DiscoveryLabel]; ok {
				g.Expect(false).To(BeTrue(),
					fmt.Sprintf("found leftover NodePool %s with discovery label", np.Name))
			}
		}
		ncList := &v1beta1.TKEMachineNodeClassList{}
		g.Expect(env.Client.List(env.Context, ncList)).To(Succeed())
		for _, nc := range ncList.Items {
			if _, ok := nc.Labels[coretest.DiscoveryLabel]; ok {
				g.Expect(false).To(BeTrue(),
					fmt.Sprintf("found leftover TKEMachineNodeClass %s with discovery label", nc.Name))
			}
		}
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
}

func isIgnorableCleanupError(err error) bool {
	return err == nil || errors.IsNotFound(err) || errors.ReasonForError(err) == metav1.StatusReasonMethodNotAllowed
}

func (env *Environment) cleanupNamespacedObjects() {
	GinkgoHelper()
	namespaces := &corev1.NamespaceList{}
	Expect(env.Client.List(env.Context, namespaces)).To(Succeed())

	for _, obj := range namespacedCleanableObjects {
		for _, ns := range namespaces.Items {
			err := env.Client.DeleteAllOf(env.Context, obj,
				client.InNamespace(ns.Name),
				client.MatchingLabels{coretest.DiscoveryLabel: "unspecified"},
				client.PropagationPolicy(metav1.DeletePropagationForeground),
			)
			Expect(isIgnorableCleanupError(err)).To(BeTrue(),
				"unexpected error cleaning up %T in namespace %s: %v", obj, ns.Name, err)
		}
	}
}

func (env *Environment) deleteTestNodePools() {
	GinkgoHelper()
	nodePoolList := &karpv1.NodePoolList{}
	Expect(env.Client.List(env.Context, nodePoolList)).To(Succeed())
	for i := range nodePoolList.Items {
		if _, ok := nodePoolList.Items[i].Labels[coretest.DiscoveryLabel]; ok {
			Expect(client.IgnoreNotFound(env.Client.Delete(env.Context, &nodePoolList.Items[i],
				client.PropagationPolicy(metav1.DeletePropagationForeground)))).To(Succeed())
		}
	}
}

func (env *Environment) deleteTestNodeClasses() {
	GinkgoHelper()
	ncList := &v1beta1.TKEMachineNodeClassList{}
	Expect(env.Client.List(env.Context, ncList)).To(Succeed())
	for i := range ncList.Items {
		if _, ok := ncList.Items[i].Labels[coretest.DiscoveryLabel]; ok {
			// Remove finalizers if present so deletion can proceed
			if len(ncList.Items[i].Finalizers) > 0 {
				patched := ncList.Items[i].DeepCopy()
				patched.Finalizers = nil
				_ = env.Client.Patch(env.Context, patched, client.MergeFrom(&ncList.Items[i]))
			}
			Expect(client.IgnoreNotFound(env.Client.Delete(env.Context, &ncList.Items[i],
				client.PropagationPolicy(metav1.DeletePropagationForeground)))).To(Succeed())
		}
	}
}

func (env *Environment) eventuallyExpectNodeClaimsGone() {
	GinkgoHelper()
	// Wait for all test NodeClaims to be cleaned up.
	// Once NodeClaims are gone, the underlying nodes and cloud instances have been released.
	Eventually(func(g Gomega) {
		ncList := &karpv1.NodeClaimList{}
		g.Expect(env.Client.List(env.Context, ncList)).To(Succeed())
		testNCCount := 0
		for i := range ncList.Items {
			if _, ok := ncList.Items[i].Labels[coretest.DiscoveryLabel]; ok {
				testNCCount++
				// If the NodeClaim has been deleting for a while but is stuck on finalizers,
				// remove them so the cleanup can proceed.
				if ncList.Items[i].DeletionTimestamp != nil && len(ncList.Items[i].Finalizers) > 0 {
					nc := ncList.Items[i].DeepCopy()
					nc.Finalizers = nil
					_ = env.Client.Patch(env.Context, nc, client.MergeFrom(&ncList.Items[i]))
				}
			}
		}
		g.Expect(testNCCount).To(BeZero(), "expected all test NodeClaims to be removed")
	}).WithTimeout(env.DefaultTimeout).WithPolling(10 * time.Second).Should(Succeed())
}
