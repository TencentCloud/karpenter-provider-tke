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
	core "k8s.io/kubernetes/pkg/apis/core"
)

// CXMHostMaintenanceType is a type representing acceptable values for OnHostMaintenance field in CXMMachineProviderSpec
type CXMHostMaintenanceType string

const (
	// MigrateHostMaintenanceType [default] - causes Compute Engine to live migrate an instance when there is a
	// maintenance event.
	MigrateHostMaintenanceType CXMHostMaintenanceType = "Migrate"
	// TerminateHostMaintenanceType - stops an instance instead of migrating it.
	TerminateHostMaintenanceType CXMHostMaintenanceType = "Terminate"
)

// CXMHostMaintenanceType is a type representing acceptable values for RestartPolicy field in CXMMachineProviderSpec
type CXMRestartPolicyType string

const (
	// Restart an instance if an instance crashes or the underlying infrastructure provider stops the instance as part
	// of a maintenance event.
	RestartPolicyAlways CXMRestartPolicyType = "Always"
	// Do not restart an instance if an instance crashes or the underlying infrastructure provider stops the instance as
	// part of a maintenance event.
	RestartPolicyNever CXMRestartPolicyType = "Never"
)

// DiskType specify system/data disk type used by instance.
type DiskType string

const (
	// High-performance cloud disk.
	CloudPremiumDiskType DiskType = "CloudPremium"
	// SSD cloud disk.
	CloudSSDDiskType  DiskType = "CloudSSD"
	CloudHSSDDiskType DiskType = "CloudHSSD"
	CloudTSSDDiskType DiskType = "CloudTSSD"
	CloudBSSDDiskType DiskType = "CloudBSSD"

	// remote ssd disk
	RemoteSSDDiskType DiskType = "REMOTE_SSD"

	// Local disk
	LOCALBasicDiskType DiskType = "LOCAL_BASIC"
	LOCALSSDDiskType   DiskType = "LOCAL_SSD"
	LOCALNVMEDiskType  DiskType = "LOCAL_NVME"
	LOCALPRODiskType   DiskType = "LOCAL_PRO"
)

// InternetChargeType specify the network billing mode.
type InternetChargeType string

const (
	// pay after use. You are billed for your traffic, by the hour.
	TrafficPostpaidByHour   InternetChargeType = "TrafficPostpaidByHour"
	BandwidthPackage        InternetChargeType = "BandwidthPackage"
	BandwidthPostpaidByHour InternetChargeType = "BandwidthPostpaidByHour"
	BandwidthPrepaid        InternetChargeType = "BandwidthPrepaid"
)

type InstanceChargeType string

const (
	PrepaidChargeType InstanceChargeType = "PrepaidCharge"
	// Pay after use, you are billed for your traffic by the hour.
	PostpaidByHourChargeType InstanceChargeType = "PostpaidByHour"
	// Pay before use, you are billed for your traffic by the year.
	UnderwriteChargeType InstanceChargeType = "Underwrite"
	// reference https://cloud.tencent.com/document/product/213/17816
	SpotpaidChargeType InstanceChargeType = "Spotpaid"
)

type RenewFlagType string

const (
	// notify upon expiration and renew automatically
	NotifyAndAutoRenew RenewFlagType = "NotifyAndAutoRenew"
	// notify upon expiration but do not renew automatically
	NotifyAndManualRenew RenewFlagType = "NotifyAndManualRenew"
	// neither notify upon expiration nor renew automatically
	DisableNotifyAndManualRenew RenewFlagType = "DisableNotifyAndManualRenew"
)

const (
	EIPAddressType            = "EIP"
	HighQualityEIPAddressType = "HighQualityEIP"
	PublicIpAddressType       = "PublicIP"
)

// CXMMachineProviderSpec is the type that will be embedded in a Machine.Spec.ProviderSpec field
// for an CXM virtual machine. It is used by the CXM machine actuator to create a single Machine.
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is
// longer).
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CXMMachineProviderSpec struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Lifecycle allow users to operations on the node before/after the node initialization.
	// +optional
	Lifecycle LifecycleConfig `json:"lifecycle,omitempty"`

	// Management contains list of items need to operation on the node.
	// Difference management type have different behavior.
	// +optional
	Management ManagementConfig `json:"management,omitempty"`

	// InstanceType is the vm instanceType selected to create the instance.
	// If the `instanceType` in `placement` also exists, this will be used first.
	// +optional
	InstanceType string `json:"instanceType,omitempty"`

	// Instance billing plan. Default: PostpaidByHourChargeType
	// optional
	InstanceChargeType InstanceChargeType `json:"instanceChargeType,omitempty"`

	// Configuration of prepaid instances. You can use the parameter to specify the attributes
	// of prepaid instances, such as the subscription period and the auto-renewalplan.
	// It is required if the `InstanceChargeType` is `PrepaidChargeType`.
	// +optional
	InstanceChargePrepaid *InstanceChargePrepaid `json:"instanceChargePrepaid,omitempty"`

	// SecurityGroupIDs is a list of Tencent Cloud Security Group ID.
	SecurityGroupIDs []string `json:"securityGroupIDs,omitempty"`

	// Configuration of the system disk of the instance.
	SystemDisk CXMDisk `json:"systemDisk,omitempty"`

	// SystemDisk is a list of data disk.
	// +optional
	DataDisks []CXMDisk `json:"dataDisks,omitempty"`

	// Tencent Cloud SSH IDs. After an instance is associated with a key, you can access the instance with
	// the private key in the key pair. You can call
	// [`DescribeKeyPairs`](https://intl. cloud.tencent.com/document/api/213/15699?from_cn_redirect=1) to obtain
	// `KeyId`. A key and password cannot be specified at the same time. Currently, you can only specify one key when
	// purchasing an instance.
	// +optional
	KeyIDs []string `json:"keyIDs,omitempty"`

	// InternetAccessible is the network configuration used to create network interface for the node.
	InternetAccessible *InternetAccessible `json:"internetAccessible,omitempty"`

	// If this value is an empty string，
	//   the displayName of the node is tke tke-${machineSetName}-work,
	//   os hostName is generated by cxm server,
	//   k8s nodeName is the internal IP address.
	// If this value is a non empty string,
	//   the machine's displayName, os hostName, and k8s nodeName are all generated based on
	//   the HostNamePattern annotate(node.tke.cloud.tencent.com/hostname-pattern) of machineSet.
	HostName string `json:"hostName,omitempty"`
	// HpcClusterId is the ID of the HPC cluster to which the instance belongs.
	HpcClusterId string `json:"hpcClusterId,omitempty"`
}

type InstanceChargePrepaid struct {
	// Subscription period; unit: month; valid values: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36.
	Period int32 `json:"period,omitempty"`

	// Auto renewal flag. Valid values: NotifyAndAutoRenew, NotifyAndManualRenew, DisableNotifyAndManualRenew.
	// Default: NotifyAndManualRenew.
	// +optional
	RenewFlag RenewFlagType `json:"renewFlag,omitempty"`
}

// EnhancedService contains Tencent Cloud services can be enabled.
type EnhancedService struct {
	// Enables cloud monitor service.
	// If this parameter is not specified, the cloud monitor service will be enabled by default.
	// +optional
	MonitorService *bool `json:"monitorService"`

	// Enables cloud security service.
	// If this parameter is not specified, the cloud security service will be enabled by default.
	// +optional
	SecurityService *bool `json:"securityService"`
}

// CXMDisk describes disks for CXM.
type CXMDisk struct {
	// DiskType is the type of the disk (eg: pd-standard). Default: CloudPremiumDiskType.
	// SystemDisk and DataDisk share this field.
	DiskType DiskType `json:"diskType,omitempty"`

	// DiskSize is the size of the disk (in GB).
	// SystemDisk and DataDisk share this field.
	DiskSize int32 `json:"diskSize,omitempty"`

	// DiskID is the id of the disk.
	// SystemDisk and DataDisk share this field
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	DiskID *string `json:"diskID,omitempty"`

	// DeleteWithInstance indicates if the disk will be auto-deleted when the instance is deleted (default false).
	// +optional
	DeleteWithInstance *bool `json:"deleteWithInstance,omitempty"`

	// AutoFormatAndMount specify whether automatic format and mount this disk by tke.
	// When AutoFormatAndMount is true, `FileSystem` and `MountTarget` must be specified.
	// +optional
	AutoFormatAndMount bool `json:"autoFormatAndMount,omitempty"`

	// FileSystem specify the filesystem used by this disk.
	// Different file systems will be selected according to different operating systems.
	// +optional
	FileSystem string `json:"fileSystem,omitempty"`

	// MountTarget specify where to mount this disk.
	// +optional
	MountTarget string `json:"mountTarget,omitempty"`

	// Encrypt specify whether to encrypt this disk.
	// +optional
	Encrypt string `json:"encrypt,omitempty"`
	// KmsKeyId specify the KMS key used to encrypt this disk.
	// +optional
	KmsKeyId string `json:"kmsKeyId,omitempty"`
	// SnapshotId specify the snapshot id used to create this disk.
	// +optional
	SnapshotId string `json:"snapshotId,omitempty"`
	// ThroughputPerformance specify the throughput performance of this disk.
	// +optional
	ThroughputPerformance int `json:"throughputPerformance,omitempty"`
	// DiskPartition specify use device name mount disk
	// +optional
	DiskPartition string `json:"DiskPartition"` // 设备名称，判断是否新的方式
	// ImageCacheId specify image cache id with snapshot
	// reference https://cloud.tencent.com/document/product/457/65908
	// +optional
	ImageCacheId string `json:"imageCacheId,omitempty"`
}

func (disk CXMDisk) IsSSD() bool {
	return disk.DiskType == CloudSSDDiskType
}

func (disk CXMDisk) IsLocalDisk() bool {
	return disk.DiskType == LOCALBasicDiskType || disk.DiskType == LOCALSSDDiskType || disk.DiskType == LOCALNVMEDiskType || disk.DiskType == LOCALPRODiskType
}

func (disk CXMDisk) IsRemoteDisk() bool {
	return disk.DiskType == RemoteSSDDiskType
}


// InternetAccessible describes network interfaces for CXM.
type InternetAccessible struct {
	// The maximum outbound bandwidth of the public network, in Mbps.
	// The default value is 1 Mbps.
	// For more information, see [Purchase Network
	// Bandwidth](https://intl.cloud.tencent.com/document/product/213/12523?from_cn_redirect=1).
	// +optional
	MaxBandwidthOut int32 `json:"maxBandwidthOut,omitempty"`

	// ChargeType specify the network connection billing plan.
	// Default `TrafficPostpaidByHour`
	// +optional
	ChargeType InternetChargeType `json:"chargeType,omitempty"`

	// The ID of the EIP instance, like `eip-hxlqja90`.
	// +optional
	AddressID *string `json:"addressID,omitempty"`

	// Bandwidth package ID. To obatin the IDs.
	// +optional
	BandwidthPackageID string `json:"bandwidthPackageID,omitempty"`

	// PublicIPAssigned indicates whether the public IP is assigned to the instance.
	// +optional
	PublicIPAssigned bool `json:"publicIPAssigned,omitempty"`

	// Resource type of the EIP, including `EIP` (elastic IP).
	// +optional
	AddressType string `json:"addressType,omitempty"`
}

// CXMMachineProviderStatus is the type that will be embedded in a Machine.Status.ProviderStatus field.
// It contains CXM-specific status information.
// Compatibility level 2: Stable within a major release for a minimum of 9 months or 3 minor releases (whichever is
// longer).
type CXMMachineProviderStatus struct {
	metav1.TypeMeta `                  json:",inline"`
	// +optional
	metav1.ObjectMeta `                  json:"metadata,omitempty"`
	// InstanceID is the ID of the instance in CXM
	// +optional
	InstanceID *string `json:"instanceID,omitempty"`
	// HostName is the hostName of the instance
	// +optional
	HostName string `json:"hostName,omitempty"`
	// DisplayName is the displayName of the instance
	// +optional
	DisplayName string `json:"displayName,omitempty"`
	// Globally unique ID of the instance.
	// +optional
	UUID *string `json:"uuid,omitempty"`
	// Conditions is a set of conditions associated with the Machine to indicate
	// errors or other status
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// TaskRef is a managed object reference to a Task related to the machine.
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	TaskRef map[string]string `json:"taskRef,omitempty"`

	// LastManagement record the last management configuration.
	// machine-lifecircle-controller use this field to obtain the change information
	// by comparing it with the current configuration.
	// +optional
	LastManagement *ManagementConfig `json:"lastManagement,omitempty"`

	// Node operation and maintenance status.
	// This field is maintained by machine-lifecircle-controller
	// Available values: Initializing, InitFailed, Initialized, Upgrading, Repairing, UpgradeFailed
	// +optional
	MaintenanceStatus *string `json:"maintenanceStatus,omitempty"`

	// SystemDisk is the system disk of the instance.
	// +optional
	SystemDisk *CXMDisk `json:"systemDisk,omitempty"`
}

type LifecycleConfig struct {
	// PreInit hooks will be executed before node initialization.
	// +optional
	PreInit string `json:"preInit,omitempty"`
	// PostInit hooks will be executed after node initialization.
	// +optional
	PostInit string `json:"postInit,omitempty"`
}

// ManagementConfig defines the content of automatic operation and maintenance.
type ManagementConfig struct {
	// DNS nameservers.
	// +optional
	Nameservers []string `json:"nameservers,omitempty"`
	// Hosts is an optional list of hosts and IPs that will be injected into the pod's hosts
	// file if specified. This is only valid for non-hostNetwork pods.
	// +optional
	// +patchMergeKey=ip
	// +patchStrategy=merge
	Hosts []core.HostAlias `json:"hosts,omitempty"       patchStrategy:"merge" patchMergeKey:"ip" protobuf:"bytes,23,rep,name=hosts"`
	// Custom linux kernel arguments.
	// +optional
	KernelArgs []string `json:"kernelArgs,omitempty"`
	// Custom kubernetes kubelet arguments.
	// +optional
	KubeletArgs []string `json:"kubeletArgs,omitempty"`
}

func (providerSpec *CXMMachineProviderSpec) IsPrepaid() bool {
	return providerSpec.InstanceChargeType == PrepaidChargeType
}

func (providerSpec *CXMMachineProviderSpec) IsUnderwrite() bool {
	return providerSpec.InstanceChargeType == UnderwriteChargeType
}
func (providerSpec *CXMMachineProviderSpec) IsPostpaidByHourChargeType() bool {
	return providerSpec.InstanceChargeType == PostpaidByHourChargeType
}

func (providerSpec *CXMMachineProviderSpec) IsSpotpaidChargeTypeChargeType() bool {
	return providerSpec.InstanceChargeType == SpotpaidChargeType
}
