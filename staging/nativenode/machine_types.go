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

package node

import (
	"k8s.io/kubernetes/pkg/apis/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Machine annotations
const (
	// When the billing type is `PrepaidChargeType`, this annotation will be added by default.
	ScaleDownDisabledAnnotation = "cluster-autoscaler.kubernetes.io/scale-down-disabled"
	// When BetaImageAnnotation value not empty, controller will query beta image from gpe when creating machine
	BetaImageAnnotation = "node.tke.cloud.tencent.com/beta-image"
	// When this annotation is added to machine, PrepareForUpdate() will change
	// machine's phase to rebooting/stoping/starting
	OperationMachineAnnotation = "node.tke.cloud.tencent.com/machine-operation"

	// When the machine enable multi cluster CIDR, this annotation will be added.
	DesiredPodNumAnnotation = "tke.cloud.tencent.com/desired-pod-num"
)

// Machine Phase status
const (
	// This is not a transient error, but
	// indicates a state that will likely need to be fixed before progress can be made
	// e.g Instance does NOT exist but Machine has providerID/address
	// e.g Cloud service returns a 4xx response
	PhaseFailed = "Failed"

	// Instance does NOT exist
	// Machine has NOT been given providerID/address
	PhaseProvisioning = "Provisioning"

	// Instance exists
	// Machine has been given providerID/address
	// Machine has NOT been given nodeRef
	PhaseProvisioned = "Provisioned"

	// Instance exists
	// Machine has been given providerID/address
	// Machine has been given a nodeRef
	// Node is not ready
	PhaseNotReady = "NotReady"

	// Instance exists
	// Machine has been given providerID/address
	// Machine has been given a nodeRef
	// Node is Ready
	PhaseRunning = "Running"

	// Machine has a deletion timestamp
	PhaseDeleting = "Deleting"

	PhaseRebooting = "Rebooting"
	PhaseStopping  = "Stopping"
	PhaseStopped   = "Stopped"
	PhaseStarting  = "Starting"
)

type StopType string
type OperationAction string

const (
	// 实例的关闭模式, soft_first, hard, soft
	StopTypeSoftFirst StopType = "soft_first" // 表示在正常关闭失败后进行强制关闭
	StopTypeHard      StopType = "hard"       // 表示直接强制关机
	StopTypeSoft      StopType = "soft"       // 仅软关机, 默认取值：soft

	OperationActionReboot OperationAction = "reboot" // 表示重启实例
	OperationActionStart  OperationAction = "start"  // 表示启动实例
	OperationActionStop   OperationAction = "stop"   // 表示停止实例
)

type MachineOperation struct {
	Action   OperationAction `json:"action,omitempty"`
	StopType StopType        `json:"stopType,omitempty"`
}

// Machine Maintenance Status
const (
	// Instance exist
	// Node is initializing
	MaintenanceStatusInitializing = "Initializing"

	// Instance exist
	// Node initialization failed
	MaintenanceStatusInitFailed = "InitFailed"

	// Instance exist
	// Node initialization succeed
	MaintenanceStatusInitialized = "Initialized"

	// Instance exist
	// Node has been initialized
	// Node is upgrading
	MaintenanceStatusUpgrading = "Upgrading"

	// Instance exist
	// Node has been initialized
	// Node has been upgraded
	MaintenanceStatusUpgraded = "Upgraded"

	// Instance exist
	// Node has been initialized
	// Node upgrade failed
	MaintenanceStatusUpgradedFailed = "UpgradeFailed"
)

type MachineStatusError string

const (
	// Represents that the combination of configuration in the MachineSpec
	// is not supported by this cluster. This is not a transient error, but
	// indicates a state that must be fixed before progress can be made.
	//
	// Example: the ProviderSpec specifies an instance type that doesn't exist,
	InvalidConfigurationMachineError MachineStatusError = "InvalidConfiguration"

	// This indicates that the MachineSpec has been updated in a way that
	// is not supported for reconciliation on this cluster. The spec may be
	// completely valid from a configuration standpoint, but the controller
	// does not support changing the real world state to match the new
	// spec.
	//
	// Example: the responsible controller is not capable of changing the
	// container runtime from docker to rkt.
	UnsupportedChangeMachineError MachineStatusError = "UnsupportedChange"

	// This generally refers to exceeding one's quota in a cloud provider,
	// or running out of physical machines in an on-premise environment.
	InsufficientResourcesMachineError MachineStatusError = "InsufficientResources"

	// There was an error while trying to create a Node to match this
	// Machine. This may indicate a transient problem that will be fixed
	// automatically with time, such as a service outage, or a terminal
	// error during creation that doesn't match a more specific
	// MachineStatusError value.
	//
	// Example: timeout trying to connect to GCE.
	CreateMachineError MachineStatusError = "CreateError"

	// There was an error while trying to update a Node that this
	// Machine represents. This may indicate a transient problem that will be
	// fixed automatically with time, such as a service outage,
	//
	// Example: error updating load balancers
	UpdateMachineError MachineStatusError = "UpdateError"

	// An error was encountered while trying to delete the Node that this
	// Machine represents. This could be a transient or terminal error, but
	// will only be observable if the provider's Machine controller has
	// added a finalizer to the object to more gracefully handle deletions.
	//
	// Example: cannot resolve EC2 IP address.
	DeleteMachineError MachineStatusError = "DeleteError"

	// This error indicates that the machine did not join the cluster
	// as a new node within the expected timeframe after instance
	// creation at the provider succeeded
	//
	// Example use case: A controller that deletes Machines which do
	// not result in a Node joining the cluster within a given timeout
	// and that are managed by a MachineSet
	JoinClusterTimeoutMachineError = "JoinClusterTimeoutError"

	// This error indicates that the instance not created or deleted
	InstanceNotFoundError MachineStatusError = "InstanceNotFound"
)

type ClusterStatusError string

const (
	// InvalidConfigurationClusterError indicates that the cluster
	// configuration is invalid.
	InvalidConfigurationClusterError ClusterStatusError = "InvalidConfiguration"

	// UnsupportedChangeClusterError indicates that the cluster
	// spec has been updated in an unsupported way. That cannot be
	// reconciled.
	UnsupportedChangeClusterError ClusterStatusError = "UnsupportedChange"

	// CreateClusterError indicates that an error was encountered
	// when trying to create the cluster.
	CreateClusterError ClusterStatusError = "CreateError"

	// UpdateClusterError indicates that an error was encountered
	// when trying to update the cluster.
	UpdateClusterError ClusterStatusError = "UpdateError"

	// DeleteClusterError indicates that an error was encountered
	// when trying to delete the cluster.
	DeleteClusterError ClusterStatusError = "DeleteError"
)

type MachineSetStatusError string

const (
	// Represents that the combination of configuration in the MachineTemplateSpec
	// is not supported by this cluster. This is not a transient error, but
	// indicates a state that must be fixed before progress can be made.
	//
	// Example: the ProviderSpec specifies an instance type that doesn't exist.
	InvalidConfigurationMachineSetError MachineSetStatusError = "InvalidConfiguration"
)

type MachineDeploymentStrategyType string

const (
	// Replace the old MachineSet by new one using rolling update
	// i.e. gradually scale down the old MachineSet and scale up the new one.
	RollingUpdateMachineDeploymentStrategyType MachineDeploymentStrategyType = "RollingUpdate"
)

// +genclient
// +genclient:skipVerbs=deleteCollection
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Machine is the Schema for the machines API.
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

// MachineSpec defines the desired state of Machine
type MachineSpec struct {
	// ObjectMeta will autopopulate the Node created. Use this to
	// indicate what labels, annotations, name prefix, etc., should be used
	// when creating the Node.
	// +optional
	ObjectMeta `json:"metadata,omitempty"`

	// The display name of the machine.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Zone is the availability zone used to create the instance.
	Zone string `json:"zone,omitempty"`

	// VPC subnet ID in the format `subnet-xxx`.
	// If the `subnetID` in `placement` also exists, this will be used first.
	SubnetID string `json:"subnetID,omitempty"`

	// The list of the taints to be applied to the corresponding Node in additive
	// manner. This list will not overwrite any other taints added to the Node on
	// an ongoing basis by other entities. These taints should be actively reconciled
	// e.g. if you ask the machine controller to apply a taint and then manually remove
	// the taint the machine controller will put it back) but not have the machine controller
	// remove any taints
	// +optional
	Taints []core.Taint `json:"taints,omitempty"`

	// Tags is the set of tags to add to apply to an instance, in addition to the ones
	// added by default by the actuator. These tags are additive. The actuator will ensure
	// these tags are present, but will not remove any other tags that may exist on the
	// instance.
	// +optional
	//Tags []TagSpecification `json:"tags,omitempty"`

	// Specified value of containerd --graph. Default value: /var/lib/docker
	// +optional
	RuntimeRootDir string `json:"runtimeRootDir,omitempty"`

	// Kubelet version is the kubelet version of the machine.
	// This field is maintained by the cloud vendor and will be populated with the latest version
	// corresponding to the cluster version. You should not set this field.
	// +optional
	KubeletVersion *string `json:"kubeletVersion,omitempty"`

	// Runtime version is the runtime version of the machine.
	// This field is maintained by the cloud vendor and will be populated with the latest version
	// corresponding to the cluster version. You should not set this field.
	// +optional
	RuntimeVersion *string `json:"runtimeVersion,omitempty"`

	// GPUConfig is the gpu driver version of the machine.
	// +optional
	GPUConfig GPUParams `json:"gpuConfig,omitempty"`

	// Sets whether the added node is schedulable. Default false.
	// After node initialization is completed, you can run kubectl uncordon $nodename to enable this node for
	// scheduling.
	// +optional
	Unschedulable bool `json:"unschedulable,omitempty"`

	// ProviderSpec details Provider-specific configuration to use during node creation.
	// +optional
	ProviderSpec ProviderSpec `json:"providerSpec"`

	// ProviderID is the identification ID of the machine provided by the provider.
	// This field must match the provider ID as seen on the node object corresponding to this machine.
	// This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler
	// with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out
	// machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a
	// generic out-of-tree provider for autoscaler, this field is required by autoscaler to be
	// able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver
	// and then a comparison is done to find out unregistered machines and are marked for delete.
	// This field will be set by the actuators and consumed by higher level entities like autoscaler that will
	// be interfacing with cluster-api as generic provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`
}

// LifecycleHook represents a single instance of a lifecycle hook
type LifecycleHook struct {
	// Name defines a unique name for the lifcycle hook.
	// The name should be unique and descriptive, ideally 1-3 words, in CamelCase or
	// it may be namespaced, eg. foo.example.com/CamelCase.
	// Names must be unique and should only be managed by a single entity.
	Name string `json:"name"`

	// Owner defines the owner of the lifecycle hook.
	// This should be descriptive enough so that users can identify
	// who/what is responsible for blocking the lifecycle.
	// This could be the name of a controller (e.g. clusteroperator/etcd)
	// or an administrator managing the hook.
	Owner string `json:"owner"`
}

// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// NodeRef will point to the corresponding Node if it exists.
	// +optional
	NodeRef *NodeReference `json:"nodeRef,omitempty"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// ProviderStatus details a Provider-specific status.
	// It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderStatus *runtime.RawExtension `json:"providerStatus,omitempty"`

	// Addresses is a list of addresses assigned to the machine. Queried from cloud provider, if available.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Addresses []core.NodeAddress `json:"addresses,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LastOperation describes the last-operation performed by the machine-controller.
	// This API should be useful as a history in terms of the latest operation performed on the
	// specific machine. It should also convey the state of the latest-operation for example if
	// it is still on-going, failed or completed successfully.
	// +optional
	LastOperation *LastOperation `json:"lastOperation,omitempty"`

	// Phase represents the current phase of machine actuation.
	// One of: Failed, Provisioning, Provisioned, Running, Deleting
	// This field is maintained by machine controller.
	// +optional
	Phase *string `json:"phase,omitempty"`

	// Conditions defines the current state of the Machine
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// NodeReference contains enough information to let you locate the
// referenced object inside the same namespace.
type NodeReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty"`

	// OS Image reported by the node from /etc/os-release (e.g. Debian GNU/Linux 7 (wheezy)).
	// +optional
	OSImage string `json:"osImage,omitempty"`

	// The Architecture reported by the node.
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// KubeletVersion reported by the node.
	// +optional
	KubeletVersion string `json:"kubeletVersion,omitempty"`

	// RuntimeVersion reported by the node.
	// +optional
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
}

// LastOperation represents the detail of the last performed operation on the MachineObject.
type LastOperation struct {
	// Description is the human-readable description of the last operation.
	Description *string `json:"description,omitempty"`

	// LastUpdated is the timestamp at which LastOperation API was last-updated.
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// State is the current status of the last performed operation.
	// E.g. Processing, Failed, Successful etc
	State *string `json:"state,omitempty"`

	// Type is the type of operation which was last performed.
	// E.g. Create, Delete, Update etc
	Type *string `json:"type,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineList contains a list of Machine
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is
// longer).
type MachineList struct {
	metav1.TypeMeta `          json:",inline"`
	metav1.ListMeta `          json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}

// TagSpecification is the name/value pair for a tag.
type TagSpecification struct {
	// Name of the tag
	Name string `json:"name"`

	// Value of the tag
	Value string `json:"value"`
}

func (m *Machine) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: m.Namespace, Name: m.Name}
}
