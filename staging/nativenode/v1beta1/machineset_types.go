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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	AnnotationDeletionProtection               = "node.tke.cloud.tencent.com/deletion-protection"
	AnnotationSkipUnderwriteDeletionProtection = "node.tke.cloud.tencent.com/skip-underwrite-deletion-protection"
	AnnotationClusterCloudTag                  = "node.tke.cloud.tencent.com/cluster-cloud-tags"
	AnnotationMachineCloudTag                  = "node.tke.cloud.tencent.com/machine-cloud-tags"
	AnnotationDisableSyncMachineCloudTag       = "node.tke.cloud.tencent.com/disable-sync-machine-tags"
	AnnotationIgnoreValidateScaling            = "node.tke.cloud.tencent.com/ignore-validate-scaling"
	HouseKeeperFinalizer                       = "node.tke.cloud.tencent.com/finalizer"
)

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:skipVerbs=deleteCollection
// +genclient:method=ApplyScale,verb=apply,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineSet ensures that a specified number of machines replicas are running at any given time.
type MachineSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSetSpec   `json:"spec,omitempty"`
	Status MachineSetStatus `json:"status,omitempty"`
}

// MachineSetSpec defines the desired state of MachineSet
type MachineSetSpec struct {
	// MachineSetType is the type of this MachineSet.
	// Different types have different operation and maintenance policy.
	Type MachineSetType `json:"type,omitempty"`

	// The display name of the machineset.
	DisplayName string `json:"displayName,omitempty"`

	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// Selector is a label query over machines that should match the replica count.
	// Label keys and values that must match in order to be controlled by this MachineSet.
	// It must match the machine template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Template is the object that describes the machine that will be created if
	// insufficient replicas are detected.
	// +optional
	Template MachineTemplateSpec `json:"template,omitempty"`

	// HealthCheckPolicyName is the healthcheckpolicy name used by this machineset.
	// When created, the latest healthcheckpolicy resource version will be recorded
	// in `healthCheckPolicyResourceVersion` in the status field.
	// +optional
	HealthCheckPolicyName *string `json:"healthCheckPolicyName"`

	// MinReadySeconds is the minimum number of seconds for which a newly created machine should be ready.
	// Defaults to 0 (machine will be considered available as soon as it is ready)
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// DeletePolicy defines the policy used to identify nodes to delete when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	DeletePolicy MachineSetDeletePolicy `json:"deletePolicy,omitempty"`

	// SubnetIDs is an array of subnets used by the specified instances.
	// +patchStrategy=merge
	SubnetIDs []string `json:"subnetIDs,omitempty" patchStrategy:"merge"`

	// InstanceTypes specifies the tke instance types.
	// +patchStrategy=merge
	InstanceTypes []string `json:"instanceTypes,omitempty" patchStrategy:"merge"`

	// Scaling is the machine autoscaler configuration for this machineset.
	// +optional
	Scaling MachinSetScaling `json:"scaling,omitempty"`

	// UpgradeSettings: Specifies the upgrade settings for created node by this machineset.
	// +optional
	//UpgradeSettings MachineUpgradeSettings `json:"upgradeSettings,omitempty"`

	// AutoRepair specifies whether the node auto-repair is enabled for the node pool.
	// If enabled, the nodes in this node pool will be monitored and, if
	// they fail health checks too many times, an automatic repair action
	// will be triggered.
	// +optional
	AutoRepair *bool `json:"autoRepair,omitempty"`

	// GPUConfigs stores message of Gpu driver version installed in machine
	GPUConfigs []GPUConfig `json:"gpuConfigs,omitempty"`

	// KeyID stores machine set ssh public key id
	KeyID string `json:"keyID,omitempty"`

	// MachineUpdateStrategy store update specified machine params strategy
	// +optional
	MachineUpdateStrategy MachineUpdateStrategy `json:"machineUpdateStrategy,omitempty"`
}

// MachineSetStatus defines the observed state of MachineSet.
type MachineSetStatus struct {
	// HealthCheckPolicyResourceVersion record the healthcheckpolicy resource version used by this machineset.
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	HealthCheckPolicyRevision *string `json:"healthCheckPolicyRevision,omitempty"`

	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`

	// The number of replicas that have labels matching the labels of the machine template of the MachineSet.
	// +optional
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas,omitempty"`

	// The number of ready replicas for this MachineSet. A machine is considered ready when the node has been created
	// and is "Ready".
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// The number of available replicas (ready for at least minReadySeconds) for this MachineSet.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed MachineSet.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Kubelet version is the kubelet version of the machineset
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	KubeletVersion string `json:"kubeletVersion,omitempty"`

	// Runtime version is the runtime version of the machineset
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	RuntimeVersion string `json:"runtimeVersion,omitempty"`

	// Represents the latest available observations of a machine set's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []MachineSetCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// MachineSetDeletePolicy defines how priority is assigned to nodes to delete when
// downscaling a MachineSet. Defaults to "Random".
type MachineSetDeletePolicy string

const (
	// RandomMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.ErrorReason or Status.ErrorMessage are set to a non-empty value).
	// Finally, it picks Machines at random to delete.
	RandomMachineSetDeletePolicy MachineSetDeletePolicy = "Random"
	// NewestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.ErrorReason or Status.ErrorMessage are set to a non-empty value).
	// It then prioritizes the newest Machines for deletion based on the Machine's CreationTimestamp.
	NewestMachineSetDeletePolicy MachineSetDeletePolicy = "Newest"
	// OldestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.ErrorReason or Status.ErrorMessage are set to a non-empty value).
	// It then prioritizes the oldest Machines for deletion based on the Machine's CreationTimestamp.
	OldestMachineSetDeletePolicy MachineSetDeletePolicy = "Oldest"
)

// MachineSetType defines the type of the machineset.
type MachineSetType string

const (
	// NativeMachineSetType represents tke native node pool.
	NativeMachineSetType MachineSetType = "Native"

	// RegularMachineSetType represents regular node pool.
	RegularMachineSetType MachineSetType = "Regular"

	// ExternalMachineSetType represents regular node pool.
	ExternalMachineSetType MachineSetType = "External"
)

// MachineTemplateSpec describes the data needed to create a Machine from a template
type MachineTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta `            json:"metadata,omitempty"`
	// Specification of the desired behavior of the machine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec MachineSpec `json:"spec,omitempty"`
}

type MachineSetConditionType string

// These are valid conditions of a replica set.
const (
	// MachineSetReplicaFailure is added in a machine set when one of its machines fails to be created
	// due to configuration file not found, machine selectors, etc. or deleted
	// due to kubelet being down or finalizers are failing.
	MachineSetReplicaFailure MachineSetConditionType = "ReplicaFailure"

	MachineSetSetHealthCheckPolicyRevisionFailure MachineSetConditionType = "SetHealthCheckPolicyRevisionFailure"
)

// MachineSetCondition describes the state of a replica set at a certain point.
type MachineSetCondition struct {
	// Type of replica set condition.
	Type MachineSetConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineSetList contains a list of MachineSet
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is
// longer).
type MachineSetList struct {
	metav1.TypeMeta `             json:",inline"`
	metav1.ListMeta `             json:"metadata,omitempty"`
	Items           []MachineSet `json:"items"`
}

// MachinSetScaling  specifies scaling options.
type MachinSetScaling struct {
	// MinReplicas constrains the minimal number of replicas of a scalable resource
	// +optional
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas constrains the maximal number of replicas of a scalable resource
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// CreatePolicy defines the machine create policy when create machine, default ZoneEquality
	// +optional
	CreatePolicy *MachineCreatePolicy `json:"createPolicy,omitempty"`
}

// MachineCreatePolicy describes a policy where to create the machine.
// +enum
type MachineCreatePolicy string

type GPUConfig struct {
	InstanceType string `json:"instanceType,omitempty"`
	Driver       string `json:"driver,omitempty"`
	CUDA         string `json:"cuda,omitempty"`
	CUDNN        string `json:"cudnn,omitempty"`
	MIGEnable    bool   `json:"migEnable,omitempty"`
	Fabric       bool   `json:"fabric,omitempty"`
}

type GPUParams struct {
	Driver    string `json:"driver,omitempty"`
	CUDA      string `json:"cuda,omitempty"`
	CUDNN     string `json:"cudnn,omitempty"`
	MIGEnable bool   `json:"migEnable,omitempty"`
	Fabric    bool   `json:"fabric,omitempty"`
}

// MachineUpdateStrategyType defines the type of the machineset update strategy
type MachineUpdateStrategyType string

const (
	// RollingUpdateStrategyType represents rolling update strategy
	RollingUpdateStrategyType MachineUpdateStrategyType = "RollingUpdate"
)

// MachineUpdateStrategy defines update machine params strategy
type MachineUpdateStrategy struct {
	// currently only one type: RollingUpdate
	Type MachineUpdateStrategyType `json:"type,omitempty"`
	// When the type is RollingUpdate, the RollingUpdate field works
	RollingUpdate *MachineRollingUpdateStrategy `json:"rollingUpdate,omitempty"`
}

// MachineUpdateStrategy defines rolling update machine params strategy
type MachineRollingUpdateStrategy struct {
	// MaxConfigUnavailable indicates the maximum config unavailable number of updated nodes in the node pool, including update failed or updating
	MaxConfigUnavailable *intstr.IntOrString `json:"maxConfigUnavailable,omitempty"`
	// MaxSteps indicates the maximum number of nodes that can be updated simultaneously in a single update round
	MaxSteps *intstr.IntOrString `json:"maxSteps,omitempty"`
}

const (
	// ZoneEqualityPolicy means choose zone equality when create machine.
	ZoneEqualityPolicy MachineCreatePolicy = "ZoneEquality"
	// ZonePriorityPolicy means choose zone priority when create machine.
	ZonePriorityPolicy MachineCreatePolicy = "ZonePriority"
)

func (ms *MachineSet) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: ms.Namespace, Name: ms.Name}
}
