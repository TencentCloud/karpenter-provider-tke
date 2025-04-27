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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProviderSpec defines the configuration to use during node creation.
type ProviderSpec struct {
	// MachineType defines the type of the Machine. Different MachineSet use the different MachineType.
	// +optional
	Type MachineType `json:"type,omitempty"`

	// Value is an inlined, serialized representation of the resource
	// configuration. It is recommended that providers maintain their own
	// versioned API types that should be serialized/deserialized from this
	// field, akin to component config.
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	Value *runtime.RawExtension `json:"value,omitempty"`
}

// ObjectMeta is metadata that all persisted resources must have, which includes all objects
// users must create. This is a copy of customizable fields from metav1.ObjectMeta.
//
// ObjectMeta is embedded in `Machine.Spec`, `MachineDeployment.Template` and `MachineSet.Template`,
// which are not top-level Kubernetes objects. Given that metav1.ObjectMeta has lots of special cases
// and read-only fields which end up in the generated CRD validation, having it as a subset simplifies
// the API and some issues that can impact user experience.
//
// During the [upgrade to controller-tools@v2](https://github.com/kubernetes-sigs/cluster-api/pull/1054)
// for v1alpha2, we noticed a failure would occur running Cluster API test suite against the new CRDs,
// specifically `spec.metadata.creationTimestamp in body must be of type string: "null"`.
// The investigation showed that `controller-tools@v2` behaves differently than its previous version
// when handling types from [metav1](k8s.io/apimachinery/pkg/apis/meta/v1) package.
//
// In more details, we found that embedded (non-top level) types that embedded `metav1.ObjectMeta`
// had validation properties, including for `creationTimestamp` (metav1.Time).
// The `metav1.Time` type specifies a custom json marshaller that, when IsZero() is true, returns `null`
// which breaks validation because the field isn't marked as nullable.
//
// In future versions, controller-tools@v2 might allow overriding the type and validation for embedded
// types. When that happens, this hack should be revisited.
type ObjectMeta struct {
	// Name must be unique within a namespace. Is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Name string `json:"name,omitempty"`

	// GenerateName is an optional prefix, used by the server, to generate a unique
	// name ONLY IF the Name field has not been provided.
	// If this field is used, the name returned to the client will be different
	// than the name passed. This value will also be combined with a unique suffix.
	// The provided value has the same validation rules as the Name field,
	// and may be truncated by the length of the suffix required to make the value
	// unique on the server.
	//
	// If this field is specified and the generated name exists, the server will
	// NOT return a 409 - instead, it will either return 201 Created or 500 with Reason
	// ServerTimeout indicating a unique name could not be found in the time allotted, and the client
	// should retry (optionally after the time indicated in the Retry-After header).
	//
	// Applied only if Name is not specified.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#idempotency
	// +optional
	GenerateName string `json:"generateName,omitempty"`

	// Namespace defines the space within each name must be unique. An empty namespace is
	// equivalent to the "default" namespace, but "default" is the canonical representation.
	// Not all objects are required to be scoped to a namespace - the value of this field for
	// those objects will be empty.
	//
	// Must be a DNS_LABEL.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/namespaces
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// List of objects depended by this object. If ALL objects in the list have
	// been deleted, this object will be garbage collected. If this object is managed by a controller,
	// then an entry in this list will point to this controller, with the controller field set to true.
	// There cannot be more than one managing controller.
	// +optional
	// +patchMergeKey=uid
	// +patchStrategy=merge
	OwnerReferences []metav1.OwnerReference `json:"ownerReferences,omitempty" patchStrategy:"merge" patchMergeKey:"uid"`
}

// MachineType defines the type of the VM.
type MachineType string

const (
	// MachineTypeNative set the VM type to CXM, Notice that Native MachineSet use CXM VM type.
	MachineTypeNative MachineType = "Native"
	// MachineTypeNativeCVM set the VM type to CVM.
	MachineTypeNativeCVM MachineType = "NativeCVM"
	// MachineTypeCVM set the VM type to CVM. Notice that Regular MachineSet use CVM VM type.
	MachineTypeCVM MachineType = "CVM"

	// MachineTypeExternal set the host type to External. Notice that External MachineSet use host External type.
	MachineTypeExternal MachineType = "External"
)

// ConditionSeverity expresses the severity of a Condition Type failing.
type ConditionSeverity string

const (
	// ConditionSeverityError specifies that a condition with `Status=False` is an error.
	ConditionSeverityError ConditionSeverity = "Error"

	// ConditionSeverityWarning specifies that a condition with `Status=False` is a warning.
	ConditionSeverityWarning ConditionSeverity = "Warning"

	// ConditionSeverityInfo specifies that a condition with `Status=False` is informative.
	ConditionSeverityInfo ConditionSeverity = "Info"

	// ConditionSeverityNone should apply only to conditions with `Status=True`.
	ConditionSeverityNone ConditionSeverity = ""
)

// ConditionType is a valid value for Condition.Type.
type ConditionType string

// Valid conditions for a machine.
const (
	// SystemDiskCreated indicates whether the cbs system disk has been created or not. If not,
	// it should include a reason and message for the failure.
	SystemDiskCreated ConditionType = "SystemDiskCreated"
	// SystemDiskDeleted indicates whether the cbs system disk has been deleted or not. If not,
	// it should include a reason and message for the failure.
	SystemDiskDeleted ConditionType = "SystemDiskDeleted"
	// DataDisksCreated indicates whether the cbs data disks has been created or not. If not,
	// it should include a reason and message for the failure.
	DataDisksCreated ConditionType = "DataDisksCreated"
	// NormNodeAdded indicates whether the norm node has been added or not. If not,
	// it should include a reason and message for the failure.
	NormNodeAdded ConditionType = "NormNodeAdded"
	// NormNodeDeleted indicates whether the norm node has been deleted or not. If not,
	// it should include a reason and message for the failure.
	NormNodeDeleted ConditionType = "NormNodeDeleted"
	// TradeCreated indicates whether the trade has been created or not. If not,
	// it should include a reason and message for the failure.
	TradeCreated ConditionType = "TradeCreated"
	// AddMachineTags indicates whether the machine has been added tags or not.
	AddMachineTags ConditionType = "AddMachineTags"
	// InstanceCreated indicates whether the instance has been created or not. If not,
	// it should include a reason and message for the failure.
	InstanceCreated ConditionType = "InstanceCreated"
	// InstanceRecordCreated indicates whether the instance record has been created or not. If not,
	// it should include a reason and message for the failure.
	InstanceRecordCreated ConditionType = "InstanceRecordCreated"
	// AddressesAllocated indicates whether the EIP has been allocated or not. If not,
	// it should include a reason and message for the failure.
	AddressesAllocated ConditionType = "AddressesAllocated"
	// AddressAssociateRequested indicates whether the address associate request has been requested or not. If not,
	// it should include a reason and message for the failure.
	AddressAssociateRequested ConditionType = "AddressAssociateRequested"
	// AddressAssociated indicates whether the EIP has been associated or not. If not,
	// it should include a reason and message for the failure.
	AddressAssociated ConditionType = "AddressAssociated"
	// SetProviderStatus indicates whether the provider status has been setted or not. If not,
	// it should include a reason and message for the failure.
	SetProviderStatus ConditionType = "SetProviderStatus"
	// NodeDrained indicates whether the node has been drained or not. If not,
	// it should include a reason and message for the failure.
	NodeDrained ConditionType = "NodeDrained"
	// NodeDeleted indicates whether the node has been deleted or not. If not,
	// it should include a reason and message for the failure.
	NodeDeleted ConditionType = "NodeDeleted"
	// DataDisksDetached indicates whether the cbs disk has been detached or not. If not,
	// it should include a reason and message for the failure.
	DataDisksDetached ConditionType = "DataDisksDetached"
	// AddressDisassociated indicates whether the EIP has been disassociated or not. If not,
	// it should include a reason and message for the failure.
	AddressDisassociated ConditionType = "AddressDisassociated"
	// InstanceStopped indicates whether the instance has been stopped or not. If not,
	// it should include a reason and message for the failure.
	InstanceStopped ConditionType = "InstanceStopped"
	// AddressesReleased indicates whether the EIP has been released or not. If not,
	// it should include a reason and message for the failure.
	AddressesReleased ConditionType = "AddressesReleased"
	// DataDisksTerminated indicates whether the cbs disk has been terminated or not. If not,
	// it should include a reason and message for the failure.
	DataDisksTerminated ConditionType = "DataDisksTerminated"
	// DeleteMachineTags indicates whether the machine has been deleted tags or not.
	DeleteMachineTags ConditionType = "DeleteMachineTags"
	// ReleaseInternalIp indicates release the machine internal ip
	ReleaseInternalIp ConditionType = "ReleaseInternalIp"
	// InstanceTerminated indicates whether the instance has been terminated or not. If not,
	// it should include a reason and message for the failure.
	InstanceTerminated ConditionType = "InstanceTerminated"
	// InstanceExistsCondition is set on the Machine to show whether a virtual mahcine has been created by the cloud
	// provider.
	InstanceExistsCondition ConditionType = "InstanceExists"
	// InstanceRecordDeleted indicates whether the instance record has been deleted or not. If not,
	// it should include a reason and message for the failure.
	InstanceRecordDeleted ConditionType = "InstanceRecordDeleted"

	// NodeInitialized indicates whether the node has been initialized or not. If not,
	// it should include a reason and message for the failure.
	NodeInitialized ConditionType = "NodeInitialized"

	// DiskExpansion indicates whether the disk has been expanded or not. If not,
	// it should include a reason and message for the failure.
	DiskExpansion ConditionType = "DiskExpansion"

	// NodeUpgraded indicates whether the node has been upgraded or not. If not,
	// it should include a reason and message for the failure.
	NodeUpgraded ConditionType = "NodeUpgraded"

	// NodeReady means kubelet is healthy and ready to accept pods.
	NodeReady ConditionType = "NodeReady"

	// MaxCreationsNotExceeded indicates whether the instance creation times has been exceeded or not. If not,
	// it should include a reason and message for the failure.
	MaxCreationsNotExceeded ConditionType = "MaxCreationsNotExceeded"

	//InstanceOperation indicates reboot/start/stop instance operation
	InstanceOperation ConditionType = "InstanceOperation"

	//SetSecurityGroups indicates machine security groups has been set
	SetSecurityGroups ConditionType = "SetSecurityGroups"

	//Management indicates management has been set
	Management ConditionType = "Management"

	// UpdateChargeType indicates machine instance charge type has been update
	UpdateChargeType ConditionType = "UpdateChargeType"

	// UpdateSystemDiskSize indicates machine system disk size has been update
	UpdateSystemDiskSize ConditionType = "UpdateSystemDiskSize"

	//UpdateResourceAttributes indicates machine resource attributes has been update
	UpdateResourceAttributes ConditionType = "UpdateResourceAttributes"

	// UpdateInternetAccessible indicates machine internet accessible has been update
	UpdateInternetAccessible ConditionType = "UpdateInternetAccessible"
)

const (
	// NodeDrainedFailedConditionReason indicates node drain failure.
	NodeDrainedFailedConditionReason string = "NodeDrainedFailed"
	// NodeDrainedSucceededConditionReason indicates node drain success.
	NodeDrainedSucceededConditionReason string = "NodeDrainedSucceeded"
	// NodeDeletedFailedConditionReason indicates node deletion failure.
	NodeDeletedFailedConditionReason string = "NodeDeletedFailed"
	// NodeDeletedSucceededConditionReason indicates node deletion success.
	NodeDeletedSucceededConditionReason string = "NodeDeletedSucceeded"
	// NodeUpgradedFailedConditionReason indicates node upgrade failure.
	NodeUpgradedFailedConditionReason string = "NodeUpgradedFailed"
	// NodeUpgradedSucceededConditionReason indicates node upgrade success.
	NodeUpgradedSucceededConditionReason string = "NodeUpgradedSucceeded"
	// NodeInitialized indicates node initialize failure.
	NodeInitializedFailedConditionReason string = "NodeInitializedFailed"
	// NodeInitialized indicates node initialize success.
	NodeInitializedSucceededConditionReason string = "NodeInitializedSucceeded"
	// ErrorCheckingProviderReason is the reason used when the exist operation fails.
	// This would normally be because we cannot contact the provider.
	ErrorCheckingProviderReason = "ErrorCheckingProvider"
	// InstanceMissingReason is the reason used when the machine was provisioned, but the instance has gone missing.
	InstanceMissingReason = "InstanceMissing"
	// InstanceNotCreatedReason is the reason used when the machine has not yet been provisioned.
	InstanceNotCreatedReason = "InstanceNotCreated"
	// MaxCreationsNotExceededFailedConditionReason indicates instance create more than the maximum number of times.
	MaxCreationsNotExceededFailedConditionReason string = "ExceededMaxNumberOfCreations"
)

// Condition defines an observation of a Machine API resource operational state.
type Condition struct {
	// Type of condition in CamelCase or in foo.example.com/CamelCase.
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions
	// can be useful (see .node.status.conditions), the ability to deconflict is important.
	// +required
	Type ConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status"`

	// Severity provides an explicit classification of Reason code, so the users or machines can immediately
	// understand the current situation and act accordingly.
	// The Severity field MUST be set only when Status=False.
	// +optional
	Severity ConditionSeverity `json:"severity,omitempty"`

	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`

	// Last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed. If that is not known, then using the time when
	// the API field changed is acceptable.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition in CamelCase.
	// The specific API may choose whether or not this field is considered a guaranteed API.
	// This field may not be empty.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	// This field may be empty.
	// +optional
	Message string `json:"message,omitempty"`
}

// Conditions provide observations of the operational state of a Machine API resource.
type Conditions []Condition
