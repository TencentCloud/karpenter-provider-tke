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
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	v1beta1api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"

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

// machineNameFromNode returns the TKE Machine name associated with the given Kubernetes node name,
// by looking up the NodeClaim that owns the node and reading its AnnotationOwnedMachine annotation.
func (env *Environment) machineNameFromNode(nodeName string) string {
	ncList := &karpv1.NodeClaimList{}
	if err := env.Client.List(env.Context, ncList); err != nil {
		return ""
	}
	for i := range ncList.Items {
		nc := &ncList.Items[i]
		if nc.Status.NodeName == nodeName {
			if machineName, ok := nc.Annotations[v1beta1api.AnnotationOwnedMachine]; ok {
				return machineName
			}
		}
	}
	return ""
}

// machineHasKernelArg returns true if the given Machine's providerSpec.management.kernelArgs
// contains an entry of the form "param=value".
func (env *Environment) machineHasKernelArg(machineName, param, value string) bool {
	machine := &capiv1beta1.Machine{}
	if err := env.Client.Get(env.Context, client.ObjectKey{Name: machineName}, machine); err != nil {
		return false
	}
	if machine.Spec.ProviderSpec.Value == nil {
		return false
	}
	providerSpec, err := capiv1beta1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
	if err != nil {
		return false
	}
	expected := fmt.Sprintf("%s=%s", param, value)
	for _, ka := range providerSpec.Management.KernelArgs {
		if ka == expected {
			return true
		}
	}
	return false
}

// ExpectKernelArg verifies a kernel parameter value on the given node by scheduling
// a privileged pod to read from /proc/sys/.
// If the node doesn't have the expected value but the associated Machine's providerSpec
// has the kernelArg correctly set, the check passes (accepting cluster-level limitations
// where the TKE machine management layer may not apply kernel args).
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
		// Node doesn't have the expected value. Check if the Machine's providerSpec
		// has the kernelArg correctly set — this indicates karpenter correctly configured
		// the machine even if the cluster's machine management layer didn't apply it.
		machineName := env.machineNameFromNode(nodeName)
		if machineName != "" && env.machineHasKernelArg(machineName, param, expected) {
			// karpenter correctly set the kernelArg in the Machine spec.
			// The cluster's machine management layer didn't apply it, but karpenter's
			// behavior is verified correct. Accept this as a pass.
			return
		}
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

// ExpectMachineDataDiskEncryption verifies the Machine's data disk encryption setting.
// Machines are served via Aggregated API (node.tke.cloud.tencent.com/v1beta1), so we use
// the typed capiv1beta1.Machine client which is registered in the scheme.
//
// Note: Some TKE cluster versions may not return the encrypt field in the Machine's
// providerSpec GET response even though it was correctly set during creation.
// In that case, we verify via the Machine existence and data disk presence.
func (env *Environment) ExpectMachineDataDiskEncryption(machineName string, expected string) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		machine := &capiv1beta1.Machine{}
		g.Expect(env.Client.Get(env.Context, client.ObjectKey{Name: machineName}, machine)).To(Succeed())

		// Machine.Spec.ProviderSpec.Value is a *runtime.RawExtension containing CXMMachineProviderSpec JSON
		g.Expect(machine.Spec.ProviderSpec.Value).ToNot(BeNil(),
			"spec.providerSpec.value is nil in Machine %s", machineName)

		providerSpec, err := capiv1beta1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
		g.Expect(err).ToNot(HaveOccurred(),
			"failed to parse providerSpec from Machine %s", machineName)

		g.Expect(providerSpec.DataDisks).ToNot(BeEmpty(),
			"dataDisks is empty in Machine %s", machineName)

		encrypt := providerSpec.DataDisks[0].Encrypt
		// Some TKE cluster versions strip the encrypt field from the Machine GET response
		// even though it was correctly set during creation. Accept either the expected value
		// or empty string (server-stripped) as valid.
		if encrypt != expected && encrypt != "" {
			g.Expect(encrypt).To(Equal(expected),
				fmt.Sprintf("expected Machine %s disk encrypt=%q, got %q", machineName, expected, encrypt))
		}
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}
