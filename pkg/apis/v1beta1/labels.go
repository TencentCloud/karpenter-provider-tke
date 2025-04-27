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

package v1beta1

import (
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func init() {
	v1.RestrictedLabelDomains = v1.RestrictedLabelDomains.Insert(RestrictedLabelDomains...)
	v1.WellKnownLabels = v1.WellKnownLabels.Insert(
		// CapacityGroup+AnnotationCPU,
		// KubeReservedGroup+AnnotationCPU,
		// SystemReservedGroup+AnnotationCPU,
		// EvictionThresholdGroup+AnnotationCPU,

		// CapacityGroup+AnnotationMemory,
		// KubeReservedGroup+AnnotationMemory,
		// SystemReservedGroup+AnnotationMemory,
		// EvictionThresholdGroup+AnnotationMemory,

		// CapacityGroup+AnnotationPods,
		// KubeReservedGroup+AnnotationPods,
		// SystemReservedGroup+AnnotationPods,
		// EvictionThresholdGroup+AnnotationPods,

		// CapacityGroup+AnnotationEphemeralStorage,
		// KubeReservedGroup+AnnotationEphemeralStorage,
		// SystemReservedGroup+AnnotationEphemeralStorage,
		// EvictionThresholdGroup+AnnotationEphemeralStorage,

		// CapacityGroup+AnnotationENI,
		// KubeReservedGroup+AnnotationENI,
		// SystemReservedGroup+AnnotationENI,
		// EvictionThresholdGroup+AnnotationENI,

		// CapacityGroup+AnnotationDirectENI,
		// KubeReservedGroup+AnnotationDirectENI,
		// SystemReservedGroup+AnnotationDirectENI,
		// EvictionThresholdGroup+AnnotationDirectENI,

		// CapacityGroup+AnnotationEIP,
		// KubeReservedGroup+AnnotationEIP,
		// SystemReservedGroup+AnnotationEIP,
		// EvictionThresholdGroup+AnnotationEIP,

		// CapacityGroup+AnnotationGPUCount,
		// CapacityGroup+AnnotationGPUCount,
		// KubeReservedGroup+AnnotationGPUCount,
		// SystemReservedGroup+AnnotationGPUCount,
		// EvictionThresholdGroup+AnnotationGPUCount,

		// CapacityGroup+AnnotationGPUType,
		// KubeReservedGroup+AnnotationGPUType,
		// SystemReservedGroup+AnnotationGPUType,
		// EvictionThresholdGroup+AnnotationGPUType,

		LabelNodeClass,
		LabelNodeClaim,

		LabelInstanceFamily,
		LabelInstanceCPU,
		LabelInstanceMemoryGB,

		LabelCBSToplogy,

		TKELabelENIIP,
		TKELabelDirectENI,
		TKELabelENI,
		TKELabelSubENI,
		TKELabelEIP,
	)
}

var (
	CapacityGroup          = "capacity." + Group
	KubeReservedGroup      = "kube-reserved." + Group
	SystemReservedGroup    = "system-reserved." + Group
	EvictionThresholdGroup = "eviction-threshold." + Group

	LabelNodeClass = Group + "/tkemachinenodeclass"
	LabelNodeClaim = Group + "/nodeclaim"

	LabelInstanceFamily   = Group + "/instance-family"
	LabelInstanceCPU      = Group + "/instance-cpu"
	LabelInstanceMemoryGB = Group + "/instance-memory-gb"

	LabelCBSToplogy = "topology.com.tencent.cloud.csi.cbs/zone"

	TKELabelENIIP     = "tke.cloud.tencent.com/eni-ip"
	TKELabelDirectENI = "tke.cloud.tencent.com/direct-eni"
	TKELabelENI       = "tke.cloud.tencent.com/eni"
	TKELabelSubENI    = "tke.cloud.tencent.com/sub-eni"
	TKELabelEIP       = "tke.cloud.tencent.com/eip"

	// RestrictedLabelDomains are either prohibited by the kubelet or reserved by karpenter
	RestrictedLabelDomains = []string{
		Group,
	}
)

var (
	AnnotationMemory           = "/memory"
	AnnotationCPU              = "/cpu"
	AnnotationPods             = "/pods"
	AnnotationENIIP            = "/eni-ip"
	AnnotationDirectENI        = "/direct-eni"
	AnnotationENI              = "/eni"
	AnnotationSubENI           = "/sub-eni"
	AnnotationEIP              = "/eip"
	AnnotationGPUCount         = "/gpu-count"
	AnnotationGPUType          = "/gpu-type"
	AnnotationEphemeralStorage = "/ephemeral-storage"

	AnnotationOwnedMachine = Group + "/owned-machine"
	AnnotationManagedBy    = Group + "/managed-by"
	AnnotationUnitPrice    = Group + "/unit-price"

	AnnotationKubeletArgPrefix = "beta." + Group + ".kubelet.arg/"
)

var (
	TerminationFinalizer = Group + "/termination"
)
