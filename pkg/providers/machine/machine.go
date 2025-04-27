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

package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
	maxInstanceTypes = 60
)

type Tag struct {
	TagKey   string `json:"tagKey"`
	TagValue string `json:"tagValue"`
}

type Provider interface {
	Get(context.Context, string) (*capiv1beta1.Machine, error)
	List(context.Context) ([]*capiv1beta1.Machine, error)
	Create(context.Context, *api.TKEMachineNodeClass, *v1.NodeClaim, []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error)
	Delete(context.Context, *v1.NodeClaim) error
}

type DefaultProvider struct {
	kubeClient   client.Client
	zoneProvider zone.Provider
	clusterID    string
}

func NewDefaultProvider(_ context.Context, kubeClient client.Client, zoneProvider zone.Provider, clusterID string) *DefaultProvider {
	return &DefaultProvider{
		kubeClient:   kubeClient,
		zoneProvider: zoneProvider,
		clusterID:    clusterID,
	}
}

func (p *DefaultProvider) Get(ctx context.Context, providerID string) (*capiv1beta1.Machine, error) {
	machineList := &capiv1beta1.MachineList{}
	err := p.kubeClient.List(ctx, machineList)
	if err != nil {
		return nil, fmt.Errorf("unable to list machines during Machine Provider Get request: %w", err)
	}

	for _, m := range machineList.Items {
		if m.Spec.ProviderID != nil && *m.Spec.ProviderID == providerID {
			return &m, nil
		}
	}

	return nil, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("machine with providerID %s not found", providerID))
}

func (p *DefaultProvider) List(ctx context.Context) ([]*capiv1beta1.Machine, error) {
	machines := []*capiv1beta1.Machine{}

	machineList := &capiv1beta1.MachineList{}
	err := p.kubeClient.List(ctx, machineList)
	if err != nil {
		return nil, err
	}

	for _, m := range machineList.Items {
		for _, o := range m.OwnerReferences {
			if o.Kind == "NodeClaim" {
				machines = append(machines, &m)
			}
		}
	}

	return machines, nil
}

func (p *DefaultProvider) Create(ctx context.Context, nodeClass *api.TKEMachineNodeClass, nodeClaim *v1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error) {
	schedulingRequirements := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	// Only filter the instances if there are no minValues in the requirement.
	if !schedulingRequirements.HasMinValues() {
		instanceTypes = p.filterInstanceTypes(nodeClaim, instanceTypes)
	}
	instanceTypes, err := cloudprovider.InstanceTypes(instanceTypes).Truncate(schedulingRequirements, maxInstanceTypes)
	if err != nil {
		return nil, nil, fmt.Errorf("truncating instance types, %w", err)
	}

	instanceTypes = orderInstanceTypesByPrice(instanceTypes, schedulingRequirements)

	zone, err := p.zoneProvider.ZoneFromID(instanceTypes[0].Offerings.Available().Compatible(schedulingRequirements).Cheapest().Requirements.Get(corev1.LabelTopologyZone).Any())
	if err != nil {
		return nil, nil, fmt.Errorf("getting zone failed: %v", err)
	}
	var subnetID string

	for _, z := range nodeClass.Status.Subnets {
		if z.Zone == zone {
			subnetID = z.ID
			break
		}
	}
	if len(subnetID) == 0 {
		return nil, nil, fmt.Errorf("subnet for %s not found", zone)
	}

	machine := &capiv1beta1.Machine{}
	providerSpec := &capiv1beta1.CXMMachineProviderSpec{}
	machine.SetLabels(map[string]string{
		v1.NodePoolLabelKey:       nodeClaim.GetLabels()[v1.NodePoolLabelKey],
		api.LabelNodeClaim:        nodeClaim.Name,
		api.LabelNodeClass:        nodeClass.Name,
		api.LabelInstanceFamily:   instanceTypes[0].Requirements.Get(api.LabelInstanceFamily).Values()[0],
		api.LabelInstanceCPU:      instanceTypes[0].Requirements.Get(api.LabelInstanceCPU).Values()[0],
		api.LabelInstanceMemoryGB: instanceTypes[0].Requirements.Get(api.LabelInstanceMemoryGB).Values()[0],
		v1.CapacityTypeLabelKey:   instanceTypes[0].Offerings.Available().Compatible(schedulingRequirements).Cheapest().Requirements.Get(v1.CapacityTypeLabelKey).Any(),
	})
	//TODO may be conflict with existed machineset
	machine.GenerateName = fmt.Sprintf("np-%s-", utilrand.String(8))
	machine.Spec.DisplayName = nodeClaim.Name
	machine.Spec.SubnetID = subnetID
	machine.Spec.Zone = zone
	if machine.GetLabels()[v1.CapacityTypeLabelKey] == v1.CapacityTypeSpot {
		machine.Spec.ProviderSpec.Type = capiv1beta1.MachineTypeNativeCVM
		providerSpec.InstanceChargeType = capiv1beta1.SpotpaidChargeType
	} else {
		machine.Spec.ProviderSpec.Type = capiv1beta1.MachineTypeNative
		providerSpec.InstanceChargeType = capiv1beta1.PostpaidByHourChargeType
	}
	machine.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: nodeClaim.APIVersion,
		Kind:       nodeClaim.Kind,
		Name:       nodeClaim.Name,
		UID:        nodeClaim.UID,
	}}

	providerSpec.InstanceType = instanceTypes[0].Name
	providerSpec.KeyIDs = lo.Map(nodeClass.Status.SSHKeys, func(s api.SSHKey, _ int) string { return s.ID })
	providerSpec.SecurityGroupIDs = lo.Map(nodeClass.Status.SecurityGroups, func(s api.SecurityGroup, _ int) string { return s.ID })

	if len(providerSpec.SecurityGroupIDs) > 5 {
		providerSpec.SecurityGroupIDs = providerSpec.SecurityGroupIDs[:5]
	}

	if nodeClass.Spec.SystemDisk != nil {
		providerSpec.SystemDisk.DiskSize = nodeClass.Spec.SystemDisk.Size
		providerSpec.SystemDisk.DiskType = capiv1beta1.DiskType(nodeClass.Spec.SystemDisk.Type)
	} else {
		providerSpec.SystemDisk.DiskSize = 50
		providerSpec.SystemDisk.DiskType = capiv1beta1.CloudPremiumDiskType
	}
	for _, d := range nodeClass.Spec.DataDisks {
		cxmDisk := capiv1beta1.CXMDisk{
			DiskType:           capiv1beta1.DiskType(d.Type),
			DiskSize:           d.Size,
			DeleteWithInstance: lo.ToPtr(true),
			FileSystem:         string(api.FileSystemEXT4),
		}
		if d.FileSystem != nil {
			cxmDisk.FileSystem = string(lo.FromPtr(d.FileSystem))
		}
		if len(lo.FromPtr(d.MountTarget)) != 0 {
			cxmDisk.AutoFormatAndMount = true
			cxmDisk.MountTarget = path.Clean(lo.FromPtr(d.MountTarget))
		}
		providerSpec.DataDisks = append(providerSpec.DataDisks, cxmDisk)
	}

	if nodeClass.Spec.InternetAccessible != nil {
		providerSpec.InternetAccessible = &capiv1beta1.InternetAccessible{
			MaxBandwidthOut: 1,
			ChargeType:      capiv1beta1.TrafficPostpaidByHour,
		}
		if nodeClass.Spec.InternetAccessible.MaxBandwidthOut != nil {
			providerSpec.InternetAccessible.MaxBandwidthOut = lo.FromPtr(nodeClass.Spec.InternetAccessible.MaxBandwidthOut)
		}
		if nodeClass.Spec.InternetAccessible.ChargeType != nil {
			providerSpec.InternetAccessible.ChargeType = capiv1beta1.InternetChargeType(*nodeClass.Spec.InternetAccessible.ChargeType)
		}
		if nodeClass.Spec.InternetAccessible.BandwidthPackageID != nil {
			providerSpec.InternetAccessible.BandwidthPackageID = lo.FromPtr(nodeClass.Spec.InternetAccessible.BandwidthPackageID)
		}
		if machine.GetLabels()[v1.CapacityTypeLabelKey] == v1.CapacityTypeSpot {
			providerSpec.InternetAccessible.AddressType = capiv1beta1.PublicIpAddressType
			providerSpec.InternetAccessible.PublicIPAssigned = true
		}
	}

	for k, v := range nodeClaim.Annotations {
		result := strings.Split(k, api.AnnotationKubeletArgPrefix)
		if len(result) == 2 {
			providerSpec.Management.KubeletArgs = append(providerSpec.Management.KubeletArgs, fmt.Sprintf("%s=%s", result[1], v))
		}
	}
	providerSpec.Management.KubeletArgs = append(providerSpec.Management.KubeletArgs, fmt.Sprintf("register-with-taints=%s", v1.UnregisteredNoExecuteTaint.ToString()))
	if nodeClass.Spec.LifecycleScript != nil {
		if nodeClass.Spec.LifecycleScript.PreInitScript != nil {
			providerSpec.Lifecycle.PreInit = lo.FromPtr(nodeClass.Spec.LifecycleScript.PreInitScript)
		}
		if nodeClass.Spec.LifecycleScript.PostInitScript != nil {
			providerSpec.Lifecycle.PostInit = lo.FromPtr(nodeClass.Spec.LifecycleScript.PostInitScript)
		}
	}

	rawProviderSpec, err := capiv1beta1.RawExtensionFromProviderSpec(providerSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("converting provider spec to raw extension, %w", err)
	}
	machine.Spec.ProviderSpec.Value = rawProviderSpec
	machine.SetAnnotations(map[string]string{
		api.AnnotationManagedBy:                            p.clusterID,
		api.AnnotationUnitPrice:                            strconv.FormatFloat(instanceTypes[0].Offerings.Available().Compatible(schedulingRequirements).Cheapest().Price, 'f', 10, 64),
		api.CapacityGroup + api.AnnotationCPU:              instanceTypes[0].Capacity.Cpu().String(),
		api.CapacityGroup + api.AnnotationMemory:           instanceTypes[0].Capacity.Memory().String(),
		api.CapacityGroup + api.AnnotationPods:             instanceTypes[0].Capacity.Pods().String(),
		api.CapacityGroup + api.AnnotationEphemeralStorage: instanceTypes[0].Capacity.StorageEphemeral().String(),

		api.KubeReservedGroup + api.AnnotationCPU:    instanceTypes[0].Overhead.KubeReserved.Cpu().String(),
		api.KubeReservedGroup + api.AnnotationMemory: instanceTypes[0].Overhead.KubeReserved.Memory().String(),

		api.EvictionThresholdGroup + api.AnnotationMemory:           instanceTypes[0].Overhead.EvictionThreshold.Memory().String(),
		api.EvictionThresholdGroup + api.AnnotationEphemeralStorage: instanceTypes[0].Overhead.EvictionThreshold.StorageEphemeral().String(),
	})

	if !instanceTypes[0].Overhead.KubeReserved.StorageEphemeral().IsZero() {
		machine.Annotations[api.KubeReservedGroup+api.AnnotationMemory] = instanceTypes[0].Overhead.KubeReserved.StorageEphemeral().String()
	}

	if !instanceTypes[0].Overhead.SystemReserved.Cpu().IsZero() {
		machine.Annotations[api.SystemReservedGroup+api.AnnotationCPU] = instanceTypes[0].Overhead.SystemReserved.Cpu().String()
	}
	if !instanceTypes[0].Overhead.SystemReserved.Memory().IsZero() {
		machine.Annotations[api.SystemReservedGroup+api.AnnotationMemory] = instanceTypes[0].Overhead.SystemReserved.Memory().String()
	}
	if !instanceTypes[0].Overhead.SystemReserved.StorageEphemeral().IsZero() {
		machine.Annotations[api.SystemReservedGroup+api.AnnotationEphemeralStorage] = instanceTypes[0].Overhead.SystemReserved.StorageEphemeral().String()
	}

	if c, ok := instanceTypes[0].Capacity[corev1.ResourceName(api.TKELabelENIIP)]; ok {
		machine.Annotations[api.CapacityGroup+api.AnnotationENIIP] = c.String()
	}
	if c, ok := instanceTypes[0].Capacity[corev1.ResourceName(api.TKELabelDirectENI)]; ok {
		machine.Annotations[api.CapacityGroup+api.AnnotationDirectENI] = c.String()
	}
	if c, ok := instanceTypes[0].Capacity[corev1.ResourceName(api.TKELabelENI)]; ok {
		machine.Annotations[api.CapacityGroup+api.AnnotationENI] = c.String()
	}
	if c, ok := instanceTypes[0].Capacity[corev1.ResourceName(api.TKELabelSubENI)]; ok {
		machine.Annotations[api.CapacityGroup+api.AnnotationSubENI] = c.String()
	}
	if c, ok := instanceTypes[0].Capacity[corev1.ResourceName(api.TKELabelEIP)]; ok {
		machine.Annotations[api.CapacityGroup+api.AnnotationEIP] = c.String()
	}
	if len(nodeClass.Spec.Tags) != 0 {
		tags := lo.MapToSlice(nodeClass.Spec.Tags, func(k string, value string) Tag { return Tag{TagKey: k, TagValue: value} })
		tagsByte, err := json.Marshal(tags)
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling tags failed, %w", err)
		}
		machine.Annotations[capiv1beta1.AnnotationMachineCloudTag] = string(tagsByte)
	}

	err = p.kubeClient.Create(ctx, machine)
	return machine, providerSpec, err
}

func (p *DefaultProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	var machine *capiv1beta1.Machine
	var err error
	if len(nodeClaim.Status.ProviderID) != 0 {
		machine, err = p.Get(ctx, nodeClaim.Status.ProviderID)
		if err != nil {
			return err
		}
	} else {
		machine = nil
	}
	if machine == nil {
		// not found
		machineList := &capiv1beta1.MachineList{}
		err := p.kubeClient.List(ctx, machineList)
		if err != nil {
			return err
		}

		if len(machineList.Items) == 0 {
			return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("machine for nodeclaim %s not found", nodeClaim.Name))
		}
		for _, m := range machineList.Items {
			for _, o := range m.OwnerReferences {
				if o.Kind == "NodeClaim" && o.Name == nodeClaim.Name {
					return p.kubeClient.Delete(ctx, machine)
				}
			}
		}
	} else {
		return p.kubeClient.Delete(ctx, machine)
	}
	return nil
}

func (p *DefaultProvider) filterInstanceTypes(nodeClaim *v1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) []*cloudprovider.InstanceType {
	instanceTypes = filterExoticInstanceTypes(instanceTypes)
	if p.isMixedCapacityLaunch(nodeClaim, instanceTypes) {
		instanceTypes = filterUnwantedSpot(instanceTypes)
	}
	return instanceTypes
}

func (p *DefaultProvider) isMixedCapacityLaunch(nodeClaim *v1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) bool {
	requirements := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	if !requirements.Get(v1.CapacityTypeLabelKey).Has(v1.CapacityTypeSpot) ||
		!requirements.Get(v1.CapacityTypeLabelKey).Has(v1.CapacityTypeOnDemand) {
		return false
	}
	hasSpotOfferings := false
	hasODOffering := false
	if requirements.Get(v1.CapacityTypeLabelKey).Has(v1.CapacityTypeSpot) {
		for _, instanceType := range instanceTypes {
			for _, offering := range instanceType.Offerings.Available() {
				if requirements.Compatible(offering.Requirements, scheduling.AllowUndefinedWellKnownLabels) != nil {
					continue
				}
				if offering.Requirements.Get(v1.CapacityTypeLabelKey).Any() == v1.CapacityTypeSpot {
					hasSpotOfferings = true
				} else {
					hasODOffering = true
				}
			}
		}
	}
	return hasSpotOfferings && hasODOffering
}

func filterExoticInstanceTypes(instanceTypes []*cloudprovider.InstanceType) []*cloudprovider.InstanceType {
	var genericInstanceTypes []*cloudprovider.InstanceType
	for _, it := range instanceTypes {
		genericInstanceTypes = append(genericInstanceTypes, it)
	}
	if len(genericInstanceTypes) != 0 {
		return genericInstanceTypes
	}
	return instanceTypes
}

func filterUnwantedSpot(instanceTypes []*cloudprovider.InstanceType) []*cloudprovider.InstanceType {
	cheapestOnDemand := math.MaxFloat64
	for _, it := range instanceTypes {
		for _, o := range it.Offerings.Available() {
			if o.Requirements.Get(v1.CapacityTypeLabelKey).Any() == v1.CapacityTypeOnDemand && o.Price < cheapestOnDemand {
				cheapestOnDemand = o.Price
			}
		}
	}

	instanceTypes = lo.Filter(instanceTypes, func(item *cloudprovider.InstanceType, index int) bool {
		available := item.Offerings.Available()
		if len(available) == 0 {
			return false
		}
		return available.Cheapest().Price <= cheapestOnDemand
	})
	return instanceTypes
}

func orderInstanceTypesByPrice(instanceTypes []*cloudprovider.InstanceType, requirements scheduling.Requirements) []*cloudprovider.InstanceType {
	sort.Slice(instanceTypes, func(i, j int) bool {
		iPrice := math.MaxFloat64
		jPrice := math.MaxFloat64
		if len(instanceTypes[i].Offerings.Available().Compatible(requirements)) > 0 {
			iPrice = instanceTypes[i].Offerings.Available().Compatible(requirements).Cheapest().Price
		}
		if len(instanceTypes[j].Offerings.Available().Compatible(requirements)) > 0 {
			jPrice = instanceTypes[j].Offerings.Available().Compatible(requirements).Cheapest().Price
		}
		if iPrice == jPrice {
			return instanceTypes[i].Name < instanceTypes[j].Name
		}
		return iPrice < jPrice
	})
	return instanceTypes
}
