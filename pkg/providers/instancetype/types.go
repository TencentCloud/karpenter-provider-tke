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

package instancetype

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
	tke2018 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tke/v20180525"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/blang/semver/v4"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/operator/options"
	tchttp "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/http"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
)

const (
	MemoryAvailable = "memory.available"
	NodeFSAvailable = "nodefs.available"
)

type DescribeZoneInstanceConfigInfosRequest struct {
	Filters []*tke2018.Filter `json:"Filters,omitempty" name:"Filters"`
}

type DescribeZoneInstanceConfigInfosResponse struct {
	*tchttp.BaseResponse
	Response *DescribeZoneInstanceConfigInfosResponseParams `json:"Response"`
}

type DescribeZoneInstanceConfigInfosResponseParams struct {
	InstanceTypeQuotaSet *string `json:"InstanceTypeQuotaSet,omitempty" name:"InstanceTypeQuotaSet"`

	RequestId *string `json:"RequestId,omitempty" name:"RequestId"`
}

type MetaFeature struct {
	FeatureType string `json:"FeatureType,omitempty"`
	NeedVpcLb   bool   `json:"NeedVpcLb,omitempty"`
	StaticMode  bool   `json:"StaticMode"`
}

type ClusterProperty struct {
	NodeNameType              string       `json:"NodeNameType,omitempty"`
	NetworkType               string       `json:"NetworkType,omitempty"`
	IsNonStaticIpMode         bool         `json:"IsNonStaticIpMode,omitempty"`
	VpcCniType                string       `json:"VpcCniType,omitempty"`
	MetaFeatureParam          *MetaFeature `json:"MetaFeatureParam,omitempty"`
	EnableCustomizedPodCIDR   bool         `json:"EnableCustomizedPodCIDR,omitempty"`
	BasePodNumberPerNode      int          `json:"BasePodNumberPerNode,omitempty"`
	EnableMultiClusterCIDR    bool         `json:"EnableMultiClusterCIDR,omitempty"`
	MultiClusterCIDR          string       `json:"MultiClusterCIDR,omitempty"`
	IgnoreClusterCIDRConflict bool         `json:"IgnoreClusterCIDRConflict,omitempty"`
	IgnoreServiceCIDRConflict bool         `json:"IgnoreServiceCIDRConflict,omitempty"`
	IsSupportMultiENI         bool         `json:"IsSupportMultiENI,omitempty"`
	IsDualStack               bool         `json:"IsDualStack,omitempty"`
	IsNetworkWithApp          bool         `json:"IsNetworkWithApp,omitempty"`
}

func NewInstanceType(ctx context.Context, region string, storageInGB int32, instanceType cxm.InstanceTypeQuotaItem, k8sVersion semver.Version,
	maxPods *int32, podsPerCore *int32,
	kubeReserved map[string]string, systemReserved map[string]string, evictionHard map[string]string,
	offerings cloudprovider.Offerings, eniLimits []*tke2018.PodLimitsInstance, clsinfo *tke2018.Cluster) *cloudprovider.InstanceType {

	if clsinfo != nil && clsinfo.Property != nil {
		clsProperty := &ClusterProperty{}
		_ = json.Unmarshal([]byte(lo.FromPtr(clsinfo.Property)), clsProperty)
		if clsProperty.NetworkType != "VPC-CNI" && clsinfo.ClusterNetworkSettings != nil && lo.FromPtr(clsinfo.ClusterNetworkSettings.MaxNodePodNum) > 3 {
			maxPods = lo.ToPtr(int32(lo.FromPtr(clsinfo.ClusterNetworkSettings.MaxNodePodNum) - 3))
		}
		if len(clsProperty.VpcCniType) == 0 &&
			clsProperty.NetworkType != "VPC-CNI" &&
			clsProperty.MetaFeatureParam == nil {
			eniLimits = nil
		}
	}

	capacity := computeCapacity(ctx, storageInGB, instanceType, maxPods, podsPerCore, eniLimits)
	it := &cloudprovider.InstanceType{
		Name:         instanceType.InstanceType,
		Requirements: computeRequirements(offerings, region, instanceType),
		Offerings:    offerings,
		Capacity:     capacity,
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      kubeReservedResources(k8sVersion, capacity.Cpu(), capacity.Pods(), int64(instanceType.Memory), kubeReserved),
			SystemReserved:    systemReservedResources(systemReserved),
			EvictionThreshold: evictionThreshold(capacity.Memory(), capacity.StorageEphemeral(), evictionHard),
		},
	}
	return it
}

//nolint:gocyclo
func computeRequirements(offerings cloudprovider.Offerings, region string, instanceTypeInfo cxm.InstanceTypeQuotaItem) scheduling.Requirements {
	requirements := scheduling.NewRequirements(
		// Well Known Upstream
		scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, instanceTypeInfo.InstanceType),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
		scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, "linux"),
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, lo.Map(offerings.Available(), func(o *cloudprovider.Offering, _ int) string {
			return o.Requirements.Get(corev1.LabelTopologyZone).Any()
		})...),
		scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, lo.Map(offerings.Available(), func(o *cloudprovider.Offering, _ int) string {
			return o.Requirements.Get(api.LabelCBSToplogy).Any()
		})...),
		scheduling.NewRequirement(corev1.LabelWindowsBuild, corev1.NodeSelectorOpDoesNotExist),
		// Well Known to Karpenter
		scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, lo.Map(offerings.Available(), func(o *cloudprovider.Offering, _ int) string {
			return o.Requirements.Get(v1.CapacityTypeLabelKey).Any()
		})...),
		// Well Known to TKE
		scheduling.NewRequirement(api.LabelInstanceCPU, corev1.NodeSelectorOpIn, fmt.Sprint(instanceTypeInfo.CPU)),
		scheduling.NewRequirement(api.LabelInstanceMemoryGB, corev1.NodeSelectorOpIn, fmt.Sprintf("%d", instanceTypeInfo.Memory)),
		scheduling.NewRequirement(api.LabelInstanceFamily, corev1.NodeSelectorOpIn, instanceTypeInfo.InstanceFamily),
	)
	return requirements
}

func computeCapacity(ctx context.Context, storageInGB int32,
	instanceTypeInfo cxm.InstanceTypeQuotaItem,
	maxPods *int32, podsPerCore *int32, eniLimits []*tke2018.PodLimitsInstance) corev1.ResourceList {
	eniip, directeni, subeni := eni(ctx, instanceTypeInfo, eniLimits)
	resourceList := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse(strconv.Itoa(instanceTypeInfo.CPU)),
		corev1.ResourceMemory:           *memory(ctx, instanceTypeInfo.Memory*1024*1024*1024),
		corev1.ResourceEphemeralStorage: resource.MustParse(fmt.Sprintf("%dG", storageInGB)),
		corev1.ResourcePods:             *pods(ctx, instanceTypeInfo, eniip, maxPods, podsPerCore),
	}
	if len(eniLimits) != 0 {
		resourceList[corev1.ResourceName(api.TKELabelEIP)] = *eip(ctx, instanceTypeInfo)
	}
	if eniip != nil {
		resourceList[corev1.ResourceName(api.TKELabelENIIP)] = *resources.Quantity(fmt.Sprint(lo.FromPtr(eniip)))
	}
	if directeni != nil {
		resourceList[corev1.ResourceName(api.TKELabelDirectENI)] = *resources.Quantity(fmt.Sprint(lo.FromPtr(directeni)))
		resourceList[corev1.ResourceName(api.TKELabelENI)] = *resources.Quantity(fmt.Sprint(lo.FromPtr(directeni)))
	}
	if subeni != nil {
		resourceList[corev1.ResourceName(api.TKELabelSubENI)] = *resources.Quantity(fmt.Sprint(lo.FromPtr(subeni)))
	}
	return resourceList
}

func memory(ctx context.Context, m int) *resource.Quantity {
	mem := resources.Quantity(strconv.Itoa(
		int(
			math.Ceil(float64((m - KdumpLevels.eval(m))) * (1 - options.FromContext(ctx).VMMemoryOverheadPercent)),
		),
	))
	return mem
}

func pods(_ context.Context, instanceTypeInfo cxm.InstanceTypeQuotaItem, eni *int64, maxPods *int32, podsPerCore *int32) *resource.Quantity {
	var count int64
	switch {
	case lo.FromPtr(maxPods) > 0:
		count = int64(lo.FromPtr(maxPods))
	//TODO just support vpc-cni
	case lo.FromPtr(eni) > 0:
		count = lo.FromPtr(eni)
		if count < 61 {
			count = 61
		}
	default:
		count = 110
	}
	if lo.FromPtr(podsPerCore) > 0 {
		count = lo.Min([]int64{int64(lo.FromPtr(podsPerCore)) * int64(instanceTypeInfo.CPU), count})
	}
	return resources.Quantity(fmt.Sprint(count))
}

func eip(_ context.Context, instanceTypeInfo cxm.InstanceTypeQuotaItem) *resource.Quantity {
	switch {
	case instanceTypeInfo.CPU >= 1 && instanceTypeInfo.CPU <= 5:
		return resources.Quantity(fmt.Sprint(2 - 1))
	case instanceTypeInfo.CPU >= 6 && instanceTypeInfo.CPU <= 11:
		return resources.Quantity(fmt.Sprint(3 - 1))
	case instanceTypeInfo.CPU >= 12 && instanceTypeInfo.CPU <= 17:
		return resources.Quantity(fmt.Sprint(4 - 1))
	case instanceTypeInfo.CPU >= 18 && instanceTypeInfo.CPU <= 23:
		return resources.Quantity(fmt.Sprint(5 - 1))
	case instanceTypeInfo.CPU >= 24 && instanceTypeInfo.CPU <= 29:
		return resources.Quantity(fmt.Sprint(6 - 1))
	case instanceTypeInfo.CPU >= 30 && instanceTypeInfo.CPU <= 35:
		return resources.Quantity(fmt.Sprint(7 - 1))
	case instanceTypeInfo.CPU >= 36 && instanceTypeInfo.CPU <= 41:
		return resources.Quantity(fmt.Sprint(8 - 1))
	case instanceTypeInfo.CPU >= 42 && instanceTypeInfo.CPU <= 47:
		return resources.Quantity(fmt.Sprint(9 - 1))
	case instanceTypeInfo.CPU >= 48:
		return resources.Quantity(fmt.Sprint(10 - 1))
	}
	return resources.Quantity(fmt.Sprint(1))
}

func eni(_ context.Context, instanceTypeInfo cxm.InstanceTypeQuotaItem, eniLimits []*tke2018.PodLimitsInstance) (*int64, *int64, *int64) {
	for _, limit := range eniLimits {
		if instanceTypeInfo.InstanceType == *limit.InstanceType {
			return limit.PodLimits.TKERouteENIStaticIP, limit.PodLimits.TKEDirectENI, limit.PodLimits.TKESubENI
		}
	}
	return nil, nil, nil
}

func systemReservedResources(systemReserved map[string]string) corev1.ResourceList {
	return lo.MapEntries(systemReserved, func(k string, v string) (corev1.ResourceName, resource.Quantity) {
		return corev1.ResourceName(k), resource.MustParse(v)
	})
}

func kubeReservedResources(k8sVersion semver.Version, cpus, pods *resource.Quantity, memInGB int64, kubeReserved map[string]string) corev1.ResourceList {
	resources := corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", (20*pods.Value())+256)),
	}
	var cpuLevel Levels
	memLevel := Levels{
		{low: 0, high: 4, fact: 0.25 * 1024},
		{low: 4, high: 8, fact: 0.20 * 1024},
		{low: 8, high: 16, fact: 0.1 * 1024},
		{low: 16, high: 128, fact: 0.06 * 1024},
		{low: 128, high: 1 << 31, fact: 0.02 * 1024},
	}

	LegacyMemory := resource.MustParse(fmt.Sprintf("%dMi", memLevel.eval(memInGB)))

	if k8sVersion.GT(semver.MustParse("1.29.0-0")) {
		cpuLevel = Levels{
			{low: 0, high: 1000, base: 60},
			{low: 1000, high: 2000, fact: 0.01},
			{low: 2000, high: 4000, fact: 0.005},
			{low: 4000, high: 1 << 31, fact: 0.0025},
		}
		if LegacyMemory.Cmp(resources[corev1.ResourceMemory]) < 0 {
			resources[corev1.ResourceMemory] = LegacyMemory
		}
	} else {
		cpuLevel = Levels{
			{low: 0, high: 4000, base: 100},
			{low: 4000, high: 64000, fact: 0.025},
			{low: 64000, high: 128000, fact: 0.0125},
			{low: 128000, high: 1 << 31, fact: 0.005},
		}
		resources[corev1.ResourceMemory] = LegacyMemory
	}
	resources[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuLevel.eval(cpus.MilliValue()), resource.DecimalSI)

	return lo.Assign(resources, lo.MapEntries(kubeReserved, func(k string, v string) (corev1.ResourceName, resource.Quantity) {
		return corev1.ResourceName(k), resource.MustParse(v)
	}))
}

func evictionThreshold(memory *resource.Quantity, storage *resource.Quantity, evictionHard map[string]string) corev1.ResourceList {
	overhead := corev1.ResourceList{
		corev1.ResourceMemory:           resource.MustParse("100Mi"),
		corev1.ResourceEphemeralStorage: resource.MustParse(fmt.Sprint(math.Ceil(float64(storage.Value()) / 100 * 10))),
	}

	override := corev1.ResourceList{}
	var evictionSignals []map[string]string
	if evictionHard != nil {
		evictionSignals = append(evictionSignals, evictionHard)
	}
	for _, m := range evictionSignals {
		temp := corev1.ResourceList{}
		if v, ok := m[MemoryAvailable]; ok {
			temp[corev1.ResourceMemory] = computeEvictionSignal(*memory, v)
		}
		if v, ok := m[NodeFSAvailable]; ok {
			temp[corev1.ResourceEphemeralStorage] = computeEvictionSignal(*storage, v)
		}
		override = resources.MaxResources(override, temp)
	}
	// Assign merges maps from left to right so overrides will always be taken last
	return lo.Assign(overhead, override)
}
func computeEvictionSignal(capacity resource.Quantity, signalValue string) resource.Quantity {
	if strings.HasSuffix(signalValue, "%") {
		p := mustParsePercentage(signalValue)

		// Calculation is node.capacity * signalValue if percentage
		// From https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/#eviction-signals
		return resource.MustParse(fmt.Sprint(math.Ceil(capacity.AsApproximateFloat64() / 100 * p)))
	}
	return resource.MustParse(signalValue)
}
func mustParsePercentage(v string) float64 {
	p, err := strconv.ParseFloat(strings.Trim(v, "%"), 64)
	if err != nil {
		panic(fmt.Sprintf("expected percentage value to be a float but got %s, %v", v, err))
	}
	// Setting percentage value to 100% is considered disabling the threshold according to
	// https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/
	if p == 100 {
		p = 0
	}
	return p
}
