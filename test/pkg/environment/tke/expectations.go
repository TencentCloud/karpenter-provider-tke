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

package tke

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2" //nolint:stylecheck
	. "github.com/onsi/gomega"    //nolint:stylecheck
)

// TestPod creates a test pod with karpenter-test toleration and nodeSelector.
func (env *Environment) TestPod(opts ...coretest.PodOptions) *corev1.Pod {
	options := coretest.PodOptions{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				coretest.DiscoveryLabel: "unspecified",
			},
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
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
			},
		},
	}
	if len(opts) > 0 {
		return coretest.Pod(options, opts[0])
	}
	return coretest.Pod(options)
}

// ExpectKernelArg verifies a kernel parameter value on the given node by scheduling
// a privileged pod to read from /proc/sys/.
func (env *Environment) ExpectKernelArg(nodeName, param, expected string) {
	GinkgoHelper()
	// Convert kernel param format (e.g. "vm.max_map_count") to /proc/sys/ path (e.g. "vm/max_map_count")
	procPath := "/proc/sys/" + strings.ReplaceAll(param, ".", "/")
	// Sanitize param for use in pod names: replace dots and underscores with dashes
	sanitizedParam := strings.NewReplacer(".", "-", "_", "-").Replace(param)

	privileged := true
	// Validate by running a pod that exits 0 if the value matches, 1 otherwise
	validatePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kernel-validate-%s", sanitizedParam),
			Namespace: "default",
			Labels: map[string]string{
				coretest.DiscoveryLabel: "unspecified",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations: []corev1.Toleration{
				{
					Key:      "karpenter-test",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "validate",
					Image: "busybox:latest",
					Command: []string{"sh", "-c",
						fmt.Sprintf(`val=$(cat %s | tr -d '[:space:]'); if [ "$val" = "%s" ]; then exit 0; else echo "expected %s got $val" > /dev/termination-log; exit 1; fi`,
							procPath, expected, expected),
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	Expect(env.Client.Create(env.Context, validatePod)).To(Succeed())
	defer func() {
		_ = client.IgnoreNotFound(env.Client.Delete(env.Context, validatePod, client.PropagationPolicy(metav1.DeletePropagationForeground)))
	}()

	Eventually(func(g Gomega) {
		p := &corev1.Pod{}
		g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(validatePod), p)).To(Succeed())
		g.Expect(p.Status.Phase).To(BeElementOf(corev1.PodSucceeded, corev1.PodFailed))
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	p := &corev1.Pod{}
	Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(validatePod), p)).To(Succeed())
	if p.Status.Phase != corev1.PodSucceeded {
		msg := ""
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				msg = cs.State.Terminated.Message
			}
		}
		Fail(fmt.Sprintf("kernel param %s validation failed: %s", param, msg))
	}
}

// ExpectKubeletArg verifies a kubelet flag is present in the kubelet process arguments on the given node.
// It schedules a privileged pod to read /proc and checks for the flag.
// For "max-pods", it directly checks node.status.capacity.pods via the Kubernetes API.
func (env *Environment) ExpectKubeletArg(nodeName, flagName, expectedValue string) {
	GinkgoHelper()

	// Special case: max-pods is reflected directly in node capacity, so we verify via K8s API
	if flagName == "max-pods" {
		Eventually(func(g Gomega) {
			node := env.GetNode(nodeName)
			pods := node.Status.Capacity[corev1.ResourcePods]
			g.Expect(pods.String()).To(Equal(expectedValue),
				fmt.Sprintf("expected node %s to have max-pods=%s but got %s", nodeName, expectedValue, pods.String()))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		return
	}

	// For other kubelet flags, verify via a privileged pod checking /proc
	privileged := true
	podName := fmt.Sprintf("kubelet-check-%s", strings.ReplaceAll(flagName, "-", ""))
	// Truncate pod name to 63 chars to satisfy DNS label constraints
	if len(podName) > 63 {
		podName = podName[:63]
	}
	expected := fmt.Sprintf("--%s=%s", flagName, expectedValue)

	validatePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
			Labels: map[string]string{
				coretest.DiscoveryLabel: "unspecified",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			HostPID:       true,
			Tolerations: []corev1.Toleration{
				{
					Key:      "karpenter-test",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "validate",
					Image: "busybox:latest",
					Command: []string{"sh", "-c",
						fmt.Sprintf(`found=0; for f in /proc/*/cmdline; do [ -r "$f" ] || continue; val=$(cat "$f" 2>/dev/null | tr '\0' '\n' | grep -cF '%s' 2>/dev/null || true); [ "$val" -gt 0 ] 2>/dev/null && found=1 && break; done; if [ "$found" = "1" ]; then exit 0; else echo "kubelet arg %s not found" > /dev/termination-log; exit 1; fi`,
							expected, expected),
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	Expect(env.Client.Create(env.Context, validatePod)).To(Succeed())
	defer func() {
		_ = client.IgnoreNotFound(env.Client.Delete(env.Context, validatePod, client.PropagationPolicy(metav1.DeletePropagationForeground)))
	}()

	Eventually(func(g Gomega) {
		p := &corev1.Pod{}
		g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(validatePod), p)).To(Succeed())
		g.Expect(p.Status.Phase).To(BeElementOf(corev1.PodSucceeded, corev1.PodFailed))
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	p := &corev1.Pod{}
	Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(validatePod), p)).To(Succeed())
	if p.Status.Phase != corev1.PodSucceeded {
		msg := ""
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				msg = cs.State.Terminated.Message
			}
		}
		Fail(fmt.Sprintf("kubelet arg --%s=%s validation failed on node %s: %s", flagName, expectedValue, nodeName, msg))
	}
}

// ExpectMachineDataDiskEncryption verifies the Machine CRD's data disk encryption setting
// using an unstructured client to avoid importing staging types.
func (env *Environment) ExpectMachineDataDiskEncryption(machineName string, expected string) {
	GinkgoHelper()

	machineGVR := schema.GroupVersionResource{
		Group:    "machine.cluster.k8s.io",
		Version:  "v1alpha1",
		Resource: "machines",
	}

	Eventually(func(g Gomega) {
		machine := &unstructured.Unstructured{}
		machine.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   machineGVR.Group,
			Version: machineGVR.Version,
			Kind:    "Machine",
		})
		g.Expect(env.Client.Get(env.Context, client.ObjectKey{Name: machineName}, machine)).To(Succeed())

		// Navigate: spec.providerSpec.value.dataDisks[0].encrypt
		spec, found, err := unstructured.NestedMap(machine.Object, "spec", "providerSpec", "value")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue(), "spec.providerSpec.value not found in Machine %s", machineName)

		dataDisks, found, err := unstructured.NestedSlice(spec, "dataDisks")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue(), "dataDisks not found in Machine %s", machineName)
		g.Expect(dataDisks).ToNot(BeEmpty(), "dataDisks is empty in Machine %s", machineName)

		disk0, ok := dataDisks[0].(map[string]interface{})
		g.Expect(ok).To(BeTrue())

		encrypt, _, _ := unstructured.NestedString(disk0, "encrypt")
		g.Expect(encrypt).To(Equal(expected),
			fmt.Sprintf("expected Machine %s disk encrypt=%s, got %s", machineName, expected, encrypt))
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}
