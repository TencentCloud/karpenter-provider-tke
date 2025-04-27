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

package cloudprovider

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"github.com/samber/lo"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/blang/semver/v4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"

	"github.com/awslabs/operatorpkg/status"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/instancetype"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/machine"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
)

func init() {
	scheduling.KnownEphemeralTaints = append(scheduling.KnownEphemeralTaints, corev1.Taint{
		Key:    "tke.cloud.tencent.com/eni-ip-unavailable",
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	}, corev1.Taint{
		Key:    "tke.cloud.tencent.com/direct-eni-ip-unavailable",
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	}, corev1.Taint{
		Key:    "tke.cloud.tencent.com/uninitialized",
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	},
		corev1.Taint{
			Key:    corev1.TaintNodeNetworkUnavailable,
			Effect: corev1.TaintEffectNoSchedule,
		},
	)
}

func NewCloudProvider(ctx context.Context,
	kubeClient client.Client, machineProvider machine.Provider,
	instanceTypeProvider instancetype.Provider, zoneProvider zone.Provider) *CloudProvider {
	return &CloudProvider{
		kubeClient:           kubeClient,
		machineProvider:      machineProvider,
		instancetypeProvider: instanceTypeProvider,
		zoneProvider:         zoneProvider,
	}
}

type CloudProvider struct {
	kubeClient           client.Client
	machineProvider      machine.Provider
	instancetypeProvider instancetype.Provider
	zoneProvider         zone.Provider
}

func (c CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	nodeClass, err := c.resolveNodeClassFromNodeClaim(ctx, nodeClaim)
	if err != nil {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("resolving node class, %w", err))
	}
	nodeClassReady := nodeClass.StatusConditions().Get("Ready")
	if !nodeClassReady.IsTrue() {
		return nil, fmt.Errorf("resolving tkemachinenodeclass, %s", nodeClassReady.Message)
	}
	instanceTypes, err := c.resolveInstanceTypes(ctx, nodeClaim, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("resolving instance types, %w", err)
	}
	if len(instanceTypes) == 0 {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("all requested instance types were unavailable during launch"))
	}
	mc, providerSpec, err := c.machineProvider.Create(ctx, nodeClass, nodeClaim, instanceTypes)
	if err != nil {
		c.instancetypeProvider.BlockInstanceType(ctx, providerSpec.InstanceType, mc.GetLabels()[v1.CapacityTypeLabelKey], mc.Spec.Zone, fmt.Sprintf("create machine block: %s", err.Error()))
		return nil, err
	}
	return c.machineToNodeClaim(ctx, mc)
}

func (c CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	return c.machineProvider.Delete(ctx, nodeClaim)
}

// Get returns a NodeClaim for the Machine object with the supplied provider ID, or nil if not found.
func (c CloudProvider) Get(ctx context.Context, providerID string) (*v1.NodeClaim, error) {
	if len(providerID) == 0 {
		return nil, fmt.Errorf("no providerID supplied to Get, cannot continue")
	}

	machine, err := c.machineProvider.Get(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("unable to get Machine from machine provider: %w", err)
	}
	if machine == nil {
		return nil, nil
	}

	nodeClaim, err := c.machineToNodeClaim(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert Machine to NodeClaim in CloudProvider.Get: %w", err)
	}

	return nodeClaim, nil
}

func (c CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	instanceTypes := []*cloudprovider.InstanceType{}

	if nodePool == nil {
		return instanceTypes, fmt.Errorf("node pool reference is nil, no way to proceed")
	}

	nodeClass, err := c.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		return instanceTypes, err
	}

	instanceTypes, err = c.instancetypeProvider.List(ctx,
		nodeClass, false)
	return instanceTypes, err

}

func (c CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{&api.TKEMachineNodeClass{}}
}

func (c *CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	return []cloudprovider.RepairPolicy{
		// Supported Kubelet Node Conditions
		{
			ConditionType:      corev1.NodeReady,
			ConditionStatus:    corev1.ConditionFalse,
			TolerationDuration: 30 * time.Minute,
		},
		{
			ConditionType:      corev1.NodeReady,
			ConditionStatus:    corev1.ConditionUnknown,
			TolerationDuration: 30 * time.Minute,
		},
	}
}

func (c CloudProvider) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
	return "", nil
}

func (c CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	machines, err := c.machineProvider.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing machines, %w", err)
	}

	var nodeClaims []*v1.NodeClaim
	for _, machine := range machines {
		nodeClaim, err := c.machineToNodeClaim(ctx, machine)
		if err != nil {
			return []*v1.NodeClaim{}, err
		}
		nodeClaims = append(nodeClaims, nodeClaim)
	}

	return nodeClaims, nil
}

func (c CloudProvider) Name() string {
	return "tke"
}

func (c CloudProvider) machineToNodeClaim(ctx context.Context, machine *capiv1beta1.Machine) (*v1.NodeClaim, error) {
	instanceType, err := c.resolveMachineToInstanceType(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("resolving machine to instancetype failed, %v", err)
	}
	nodeClaim := &v1.NodeClaim{}
	labels := map[string]string{}
	annotations := map[string]string{}

	resourceFilter := func(n corev1.ResourceName, v resource.Quantity) bool {
		if resources.IsZero(v) {
			return false
		}
		return true
	}
	nodeClaim.Status.Capacity = lo.PickBy(instanceType.Capacity, resourceFilter)
	nodeClaim.Status.Allocatable = lo.PickBy(instanceType.Allocatable(), resourceFilter)

	for key, req := range instanceType.Requirements {
		if req.Len() == 1 {
			labels[key] = req.Values()[0]
		}
	}

	if v, ok := machine.Labels[v1.NodePoolLabelKey]; ok {
		labels[v1.NodePoolLabelKey] = v
	}

	if v, ok := machine.Annotations[api.AnnotationManagedBy]; ok {
		annotations[api.AnnotationManagedBy] = v
	}
	annotations[api.AnnotationOwnedMachine] = machine.Name

	nodeClaim.Status.ProviderID = lo.FromPtr(machine.Spec.ProviderID)
	nodeClaim.Labels = labels
	nodeClaim.Annotations = annotations
	nodeClaim.CreationTimestamp = machine.GetCreationTimestamp()

	if lo.FromPtr(machine.Status.Phase) == capiv1beta1.PhaseDeleting {
		nodeClaim.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	}

	return nodeClaim, nil
}
func (c CloudProvider) resolveMachineToInstanceType(ctx context.Context, machine *capiv1beta1.Machine) (*cloudprovider.InstanceType, error) {
	spec, err := capiv1beta1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
	if err != nil {
		return nil, fmt.Errorf("unable to get ProviderSpec from Machine %q, %w", machine.GetName(), err)
	}
	capacity := resourceListFromAnnotations(api.CapacityGroup, machine.GetAnnotations())
	kubereserved := resourceListFromAnnotations(api.KubeReservedGroup, machine.GetAnnotations())
	systemreserved := resourceListFromAnnotations(api.SystemReservedGroup, machine.GetAnnotations())
	evictionthreshold := resourceListFromAnnotations(api.EvictionThresholdGroup, machine.GetAnnotations())

	kubletVersion, err := semver.Make(lo.FromPtr(machine.Spec.KubeletVersion))
	cxmInstanceType := cxm.InstanceTypeQuotaItem{}
	cxmInstanceType.Zone = machine.Spec.Zone
	cxmInstanceType.InstanceType = spec.InstanceType
	cxmInstanceType.InstanceFamily = machine.GetLabels()[api.LabelInstanceFamily]
	cxmInstanceType.CPU, err = strconv.Atoi(machine.GetLabels()[api.LabelInstanceCPU])
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine %s cpu %v", machine.GetName(), err)
	}
	cxmInstanceType.Memory, err = strconv.Atoi(machine.GetLabels()[api.LabelInstanceMemoryGB])
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine %s memory %v", machine.GetName(), err)
	}
	cxmInstanceType.Price.UnitPrice, err = strconv.ParseFloat(machine.Annotations[api.AnnotationUnitPrice], 64)
	if err != nil {
		return nil, fmt.Errorf("unable to convert machine %s price %v", machine.GetName(), err)
	}
	// TODO gpu resource

	var offerings []*cloudprovider.Offering
	zoneID, _ := c.zoneProvider.IDFromZone(cxmInstanceType.Zone)
	offering := &cloudprovider.Offering{
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, machine.GetLabels()[v1.CapacityTypeLabelKey]),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zoneID),
			scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, cxmInstanceType.Zone),
		),
		Price:     cxmInstanceType.Price.UnitPrice,
		Available: true,
	}
	offerings = append(offerings, offering)
	instanceType := instancetype.NewInstanceType(ctx, "", 50, cxmInstanceType, kubletVersion,
		nil, nil, nil, nil, nil,
		offerings,
		nil, nil)
	_, found := capacity[corev1.ResourceCPU]
	if !found {
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no cpu capacity found", machine.GetName())
	}

	_, found = capacity[corev1.ResourceMemory]
	if !found {
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no memory capacity found", machine.GetName())
	}
	instanceType.Name = spec.InstanceType
	instanceType.Capacity = capacity
	instanceType.Overhead = &cloudprovider.InstanceTypeOverhead{
		KubeReserved:      kubereserved,
		SystemReserved:    systemreserved,
		EvictionThreshold: evictionthreshold,
	}
	return instanceType, nil
}

func (c CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *v1.NodePool) (*api.TKEMachineNodeClass, error) {
	nodeClass := &api.TKEMachineNodeClass{}

	if nodePool.Spec.Template.Spec.NodeClassRef == nil {
		return nil, fmt.Errorf("node class reference is nil, no way to proceed")
	}

	name := nodePool.Spec.Template.Spec.NodeClassRef.Name
	if name == "" {
		return nil, fmt.Errorf("node class reference name is empty, no way to proceed")
	}

	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nodePool.Namespace}, nodeClass); err != nil {
		return nil, err
	}

	return nodeClass, nil
}

func (c *CloudProvider) resolveNodeClassFromNodeClaim(ctx context.Context, nodeClaim *v1.NodeClaim) (*api.TKEMachineNodeClass, error) {
	nodeClass := &api.TKEMachineNodeClass{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodeClaim.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
	return nodeClass, nil
}

func (c *CloudProvider) resolveInstanceTypes(ctx context.Context, nodeClaim *v1.NodeClaim, nodeClass *api.TKEMachineNodeClass) ([]*cloudprovider.InstanceType, error) {
	instanceTypes, err := c.instancetypeProvider.List(ctx,
		nodeClass, false)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	return lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool {
		return reqs.Compatible(i.Requirements, scheduling.AllowUndefinedWellKnownLabels) == nil &&
			len(i.Offerings.Compatible(reqs).Available()) > 0 &&
			resources.Fits(nodeClaim.Spec.Resources.Requests, i.Allocatable())
	}), nil
}

func resourceListFromAnnotations(group string, annotations map[string]string) corev1.ResourceList {
	r := corev1.ResourceList{}

	if annotations == nil {
		return r
	}

	cpu, found := annotations[group+api.AnnotationCPU]
	if found {
		r[corev1.ResourceCPU] = resource.MustParse(cpu)
	}

	memory, found := annotations[group+api.AnnotationMemory]
	if found {
		r[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	pods, found := annotations[group+api.AnnotationPods]
	if found {
		r[corev1.ResourcePods] = resource.MustParse(pods)
	}

	estore, found := annotations[group+api.AnnotationEphemeralStorage]
	if found {
		r[corev1.ResourceEphemeralStorage] = resource.MustParse(estore)
	}

	eniip, found := annotations[group+api.AnnotationENIIP]
	if found {
		r[corev1.ResourceName(api.TKELabelENIIP)] = resource.MustParse(eniip)
	}
	directeni, found := annotations[group+api.AnnotationDirectENI]
	if found {
		r[corev1.ResourceName(api.TKELabelDirectENI)] = resource.MustParse(directeni)
	}
	eni, found := annotations[group+api.AnnotationENI]
	if found {
		r[corev1.ResourceName(api.TKELabelENI)] = resource.MustParse(eni)
	}
	subeni, found := annotations[group+api.AnnotationSubENI]
	if found {
		r[corev1.ResourceName(api.TKELabelSubENI)] = resource.MustParse(subeni)
	}
	eip, found := annotations[group+api.AnnotationEIP]
	if found {
		r[corev1.ResourceName(api.TKELabelEIP)] = resource.MustParse(eip)
	}

	return r
}

// newTerminatingNodeClassError returns a NotFound error for handling by
func newTerminatingNodeClassError(name string) *errors.StatusError {
	qualifiedResource := schema.GroupResource{Group: api.Group, Resource: "tkemachinenodeclasses"}
	err := errors.NewNotFound(qualifiedResource, name)
	err.ErrStatus.Message = fmt.Sprintf("%s %q is terminating, treating as not found", qualifiedResource.String(), name)
	return err
}
