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
)

// TKEMachineNodeClassSpec is the top level specification for TKEMachineNodeClasses.
type TKEMachineNodeClassSpec struct {
	// SubnetSelectorTerms is a list of or subnet selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="subnetSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id']",rule="self.all(x, has(x.tags) || has(x.id))"
	// +kubebuilder:validation:XValidation:message="'id' is mutually exclusive, cannot be set with a combination of other fields in subnetSelectorTerms",rule="!self.all(x, has(x.id) && has(x.tags))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	SubnetSelectorTerms []SubnetSelectorTerm `json:"subnetSelectorTerms" hash:"ignore"`
	// SecurityGroupSelectorTerms is a list of or security group selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="securityGroupSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id']",rule="self.all(x, has(x.tags) || has(x.id))"
	// +kubebuilder:validation:XValidation:message="'id' is mutually exclusive, cannot be set with a combination of other fields in securityGroupSelectorTerms",rule="!self.all(x, has(x.id) && has(x.tags))"
	// +kubebuilder:validation:MaxItems:=5
	// +required
	SecurityGroupSelectorTerms []SecurityGroupSelectorTerm `json:"securityGroupSelectorTerms" hash:"ignore"`
	// SSHKeySelectorTerms is a list of or SSH key selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="sshKeySelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['tags', 'id']",rule="self.all(x, has(x.tags) || has(x.id))"
	// +kubebuilder:validation:XValidation:message="'id' is mutually exclusive, cannot be set with a combination of other fields in sshKeySelectorTerms",rule="!self.all(x, has(x.id) && has(x.tags))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	SSHKeySelectorTerms []SSHKeySelectorTerm `json:"sshKeySelectorTerms" hash:"ignore"`
	// SystemDisk defines the system disk of the instance.
	// if not specified, a default system disk (CloudPremium, 50GB) will be used.
	// +optional
	SystemDisk *SystemDisk `json:"systemDisk,omitempty"`
	// DataDisks defines the data disks of the instance.
	// +optional
	DataDisks []DataDisk `json:"dataDisks,omitempty"`
	// InternetAccessible is the network configuration used to create network interface for the node.
	// +optional
	InternetAccessible *InternetAccessible `json:"internetAccessible,omitempty"`
	// LifecycleScript allow users to operations on the node before/after the node initialization.
	// +optional
	LifecycleScript *LifecycleScript `json:"lifecycleScript,omitempty"`
	// Tags to be applied on tke machine resources like instances.
	// The tags must be already created in tencentcloud
	// (https://console.cloud.tencent.com/tag)
	// +kubebuilder:validation:XValidation:message="empty tag keys aren't supported",rule="self.all(k, k != '')"
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SubnetSelectorTerm defines selection logic for a subnet used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SubnetSelectorTerm struct {
	// Tags is a map of key/value tags used to select subnets
	// Specifying '*' for a value selects all values for a given tag key.
	// The tags must be already created in tencentcloud
	// (https://console.cloud.tencent.com/tag)
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
	// ID is the subnet id
	// +kubebuilder:validation:Pattern="subnet-[0-9a-z]+"
	// +optional
	ID string `json:"id,omitempty"`
}

// SecurityGroupSelectorTerm defines selection logic for a security group used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SecurityGroupSelectorTerm struct {
	// Tags is a map of key/value tags used to select security group
	// Specifying '*' for a value selects all values for a given tag key.
	// The tags should be already created in tencentcloud
	// (https://console.cloud.tencent.com/tag)
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
	// ID is the security group id
	// +kubebuilder:validation:Pattern:="sg-[0-9a-z]+"
	// +optional
	ID string `json:"id,omitempty"`
}

// SSHKeysSelectorTerm defines selection logic for a security group used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SSHKeySelectorTerm struct {
	// Tags is a map of key/value tags used to select sshkey
	// Specifying '*' for a value selects all values for a given tag key.
	// The tags must be already created in tencentcloud
	// (https://console.cloud.tencent.com/tag)
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
	// ID is the SSH key id
	// +kubebuilder:validation:Pattern:="skey-[0-9a-z]+"
	// +optional
	ID string `json:"id,omitempty"`
}

// +kubebuilder:validation:Enum:={CloudPremium,CloudSSD,CloudHSSD,CloudTSSD,CloudBSSD}
type DiskType string

// +kubebuilder:validation:Enum:={ext3,ext4,xfs}
type FileSystem string

// +kubebuilder:validation:Enum:={TrafficPostpaidByHour,BandwidthPackage,BandwidthPostpaidByHour}
type InternetChargeType string

const (
	DiskTypeCloudPremium DiskType = "CloudPremium"
	DiskTypeCloudSSD     DiskType = "CloudSSD"
	DiskTypeCloudHSSD    DiskType = "CloudHSSD"
	DiskTypeTSSD         DiskType = "CloudTSSD"
	DiskTypeBSSD         DiskType = "CloudBSSD"

	FileSystemEXT3 FileSystem = "ext3"
	FileSystemEXT4 FileSystem = "ext4"
	FileSystemXFS  FileSystem = "xfs"

	TrafficPostpaidByHour   InternetChargeType = "TrafficPostpaidByHour"
	BandwidthPackage        InternetChargeType = "BandwidthPackage"
	BandwidthPostpaidByHour InternetChargeType = "BandwidthPostpaidByHour"
)

type SystemDisk struct {
	// Size of disk in GB.
	// Supported size: 20-2048, step size is 1.
	// +kubebuilder:validation:Minimum=20
	// +kubebuilder:validation:Maximum=2048
	// +required
	Size int32 `json:"size,omitempty"`
	// Type of disk, supported type: {CloudPremium, CloudSSD, CloudHSSD, CloudTSSD, CloudBSSD}.
	// +optional
	Type DiskType `json:"type,omitempty"`
}

type DataDisk struct {
	// Size of disk in GB.
	// Supported size: 20-32000, step size is 10.
	// +kubebuilder:validation:Minimum=20
	// +kubebuilder:validation:Maximum=32000
	// +kubebuilder:validation:XValidation:message="step size should be 10",rule="self%10 == 0"
	// +required
	Size int32 `json:"size,omitempty"`
	// Type of disk, supported type: {CloudPremium, CloudSSD, CloudHSSD, CloudTSSD, CloudBSSD}.
	// +optional
	Type DiskType `json:"type,omitempty"`
	// MountTarget is the path that disk wil mount during intalization.
	// +optional
	MountTarget *string `json:"mountTarget,omitempty"`
	// FileSystem specify the filesystem used by this disk.
	// Supported filesystem: {ext3, ext4, xfs}.
	// If not specified, default etx4 will be used.
	// +optional
	FileSystem *FileSystem `json:"fileSystem,omitempty"`
}

// +kubebuilder:validation:XValidation:message="bandwidthPackageID should be specified when chargeType is BandwidthPostpaidByHour",rule="has(self.chargeType) && self.chargeType == 'BandwidthPackage' ? has(self.bandwidthPackageID) : true"
type InternetAccessible struct {
	// The maximum outbound bandwidth of the public network, in Mbps.
	// Valid Range: Minimum value of 1. Maximum value of 100.
	// If not specified, default 1 Mbps will be used.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	MaxBandwidthOut *int32 `json:"maxBandwidthOut,omitempty"`
	// ChargeType specify the network connection billing plan.
	// If not specified, default TrafficPostpaidByHour will be used.
	// Supported type: {TrafficPostpaidByHour, BandwidthPackage, BandwidthPostpaidByHour}.
	// +optional
	ChargeType *InternetChargeType `json:"chargeType,omitempty"`
	// Bandwidth package ID.
	// The ID should be already created in tencentcloud
	// (https://console.cloud.tencent.com/vpc/package)
	// +kubebuilder:validation:Pattern="bwp-[0-9a-z]+"
	// +optional
	BandwidthPackageID *string `json:"bandwidthPackageID,omitempty"`
}

type LifecycleScript struct {
	// PreInitScript will be executed before node initialization..
	// +optional
	PreInitScript *string `json:"preInitScript,omitempty"`
	// PostInitScript will be executed after node initialization.
	// +optional
	PostInitScript *string `json:"postInitScript,omitempty"`
}

// TKEMachineNodeClass is the Schema for the TKEMachineNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""
// +kubebuilder:resource:path=tkemachinenodeclasses,scope=Cluster,categories=karpenter,shortName={tmnc,tmncs}
// +kubebuilder:subresource:status
type TKEMachineNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TKEMachineNodeClassSpec   `json:"spec,omitempty"`
	Status TKEMachineNodeClassStatus `json:"status,omitempty"`
}

// TKEMachineNodeClassList contains a list of TKEMachineNodeClasses
// +kubebuilder:object:root=true
type TKEMachineNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TKEMachineNodeClass `json:"items"`
}
