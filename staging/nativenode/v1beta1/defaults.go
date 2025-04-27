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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

var (
	DefaultNameservers       = []string{"183.60.83.19", "183.60.82.98"}
	DefaultContainerdRootDir = "/var/lib/containerd"
)

/*
//  Used by CA
const (
	nodeGroupMinSizeAnnotationKey   = "cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKey   = "cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size"
	nodeGroupMinSizeAnnotationKeyV2 = "node.tke.cloud.tencent.com/cluster-api-autoscaler-node-group-min-size"
	nodeGroupMaxSizeAnnotationKeyV2 = "node.tke.cloud.tencent.com/cluster-api-autoscaler-node-group-max-size"
)
*/

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_MachineSet sets defaults for MachineSet
func SetDefaults_MachineSet(obj *MachineSet) {
	// Set MachineSetSpec.Replicas to 1 if it is not set.
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = new(int32)
		*obj.Spec.Replicas = 1
	}

	if obj.Spec.Type == NativeMachineSetType && obj.Spec.Template.Spec.ProviderSpec.Type == "" {
		obj.Spec.Template.Spec.ProviderSpec.Type = MachineTypeNative
	}
	if obj.Spec.Type == RegularMachineSetType && obj.Spec.Template.Spec.ProviderSpec.Type == "" {
		obj.Spec.Template.Spec.ProviderSpec.Type = MachineTypeCVM
	}

	// Set default template
	SetDefaults_MachineSpec(&obj.Spec.Template.Spec)

	// Set default DeletePolicy as Newest.
	if obj.Spec.DeletePolicy == "" {
		obj.Spec.DeletePolicy = NewestMachineSetDeletePolicy
	}
	// Set default scaling options.
	scaling := &obj.Spec.Scaling

	if scaling.CreatePolicy == nil || string(*scaling.CreatePolicy) == "" {
		zoneEquality := ZoneEqualityPolicy
		scaling.CreatePolicy = &zoneEquality
	}

	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

	//obj.Annotations[nodeGroupMinSizeAnnotationKey] = cast.ToString(scaling.MinReplicas)
	//obj.Annotations[nodeGroupMaxSizeAnnotationKey] = cast.ToString(scaling.MaxReplicas)
	//obj.Annotations[nodeGroupMinSizeAnnotationKeyV2] = cast.ToString(scaling.MinReplicas)
	//obj.Annotations[nodeGroupMaxSizeAnnotationKeyV2] = cast.ToString(scaling.MaxReplicas)

	obj.Spec.SubnetIDs = removeDuplicate(obj.Spec.SubnetIDs)
	obj.Spec.InstanceTypes = removeDuplicate(obj.Spec.InstanceTypes)
}

// SetDefaults_Machine sets defaults for Machine
func SetDefaults_Machine(obj *Machine) {
	// Machine name prefix is fixed to `mc-`
	if obj.ObjectMeta.GenerateName == "" {
		obj.ObjectMeta.GenerateName = "mc-"
	}
	if obj.ObjectMeta.Annotations == nil {
		obj.ObjectMeta.Annotations = make(map[string]string, 0)
	}
	if obj.Spec.ProviderSpec.Type == MachineTypeNativeCVM {
		if _, ok := obj.ObjectMeta.Annotations["node.tke.cloud.tencent.com/force-delete"]; !ok {
			obj.ObjectMeta.Annotations["node.tke.cloud.tencent.com/force-delete"] = "true"
		}
	}
	SetDefaults_MachineSpec(&obj.Spec)
}

// SetDefaults_MachineSpec sets defaults for Machine spec
func SetDefaults_MachineSpec(obj *MachineSpec) {
	if obj.RuntimeRootDir == "" {
		obj.RuntimeRootDir = DefaultContainerdRootDir
	}

	val, err := ProviderSpecFromRawExtension(obj.ProviderSpec.Value)
	if err != nil {
		return
	}

	if val.InstanceChargeType == PrepaidChargeType || val.InstanceChargeType == UnderwriteChargeType {
		if obj.ObjectMeta.Annotations == nil {
			obj.ObjectMeta.Annotations = make(map[string]string, 0)
		}
		obj.ObjectMeta.Annotations[ScaleDownDisabledAnnotation] = "true"
	}

	SetDefaults_ProviderSpec(&obj.ProviderSpec)
}

// SetDefaults_ProviderSpec sets defaults for Provider spec
func SetDefaults_ProviderSpec(obj *ProviderSpec) {
	if obj.Type == "" {
		obj.Type = MachineTypeNative
	}

	switch obj.Type {
	case MachineTypeNative, MachineTypeNativeCVM:
		SetDefaults_CXMMachineProviderSpec(obj.Value)
	case MachineTypeCVM:
		SetDefaults_CVMMachineProviderSpec(obj.Value)
	default:
	}
}

// SetDefaults_CXMMachineProviderSpec sets defaults for CXMMachineProvider spec
func SetDefaults_CXMMachineProviderSpec(value *runtime.RawExtension) {
	obj, err := ProviderSpecFromRawExtension(value)
	if err != nil {
		return
	}

	// 如果客户没设置 Nameserver，我们会设置默认的server.
	if len(obj.Management.Nameservers) == 0 {
		obj.Management.Nameservers = append(obj.Management.Nameservers, DefaultNameservers...)
	}
	obj.Management.Nameservers = removeDuplicate(obj.Management.Nameservers)
	// 注释掉如下代码:
	// K8S只会用Nameserver中前三项，如果我们把默认值加在前面可能会导致客户自定义Nameserver不生效
	// 所以这里我们在控制台上做了强提醒，并且遵循客户的设置，如果客户一定要删掉腾讯云的Nameserver就需要将其设置为自定义DNS服务器的上游
	/*
		for _, ns := range DefaultNameservers {
			if !stringsutil.StringIn(ns, obj.Management.Nameservers) {
				obj.Management.Nameservers = removeDuplicate(append(DefaultNameservers, obj.Management.Nameservers...))
				break
			}
		}
	*/
	obj.Management.KernelArgs = RemoveDuplicateKVPairs(obj.Management.KernelArgs)
	obj.Management.KubeletArgs = RemoveDuplicateKVPairs(obj.Management.KubeletArgs)

	if string(obj.InstanceChargeType) == "" {
		obj.InstanceChargeType = PostpaidByHourChargeType
	}

	if obj.InstanceChargeType == PrepaidChargeType {
		if obj.InstanceChargePrepaid == nil {
			obj.InstanceChargePrepaid = &InstanceChargePrepaid{}
		}

		if obj.InstanceChargePrepaid != nil {
			if obj.InstanceChargePrepaid.RenewFlag == "" {
				obj.InstanceChargePrepaid.RenewFlag = NotifyAndManualRenew
			}

			if obj.InstanceChargePrepaid.Period == 0 {
				obj.InstanceChargePrepaid.Period = int32(1)
			}
		}
	}
	obj.KeyIDs = removeDuplicate(obj.KeyIDs)
	obj.SecurityGroupIDs = removeDuplicate(obj.SecurityGroupIDs)

	// TODO: Check the reason why the default value of this `ScaleDownDisabledAnnotation` setting does not take effect here
	/*
		// PrepaidChargeType must set ScaleDownDisabledAnnotation annotation
		if obj.IsPrepaid() {
			if obj.Annotations == nil {
				obj.ObjectMeta.Annotations = map[string]string{}
			}
			obj.ObjectMeta.Annotations[node.ScaleDownDisabledAnnotation] = "true"
		}
	*/

	newDataDisks := make([]CXMDisk, 0, len(obj.DataDisks))
	for _, disk := range obj.DataDisks {
		if disk.DeleteWithInstance == nil {
			disk.DeleteWithInstance = pointer.Bool(true)
		}
		if disk.KmsKeyId != "" {
			disk.Encrypt = "ENCRYPT"
		}
		/*
			if disk.AutoFormatAndMount && disk.MountTarget == "" {
				disk.MountTarget = "/var/lib/docker"
			}
			if disk.AutoFormatAndMount && disk.FileSystem == "" {
				disk.FileSystem = "ext4"
			}
		*/
		newDataDisks = append(newDataDisks, disk)
	}
	obj.DataDisks = newDataDisks

	if obj.InternetAccessible != nil {
		if obj.InternetAccessible.MaxBandwidthOut == 0 {
			obj.InternetAccessible.MaxBandwidthOut = 1
		}

		if obj.InternetAccessible.BandwidthPackageID != "" && obj.InternetAccessible.ChargeType == "" {
			obj.InternetAccessible.ChargeType = BandwidthPackage
		}

		if obj.InternetAccessible.ChargeType == "" {
			obj.InternetAccessible.ChargeType = TrafficPostpaidByHour
		}

		if obj.InternetAccessible.AddressType == "" {
			obj.InternetAccessible.AddressType = "EIP"
		}
	}

	newValue, err := RawExtensionFromProviderSpec(obj)
	if err != nil {
		return
	}

	*value = *newValue
}

// SetDefaults_CVMMachineProviderSpec sets defaults for CVMMachineProvider spec
func SetDefaults_CVMMachineProviderSpec(value *runtime.RawExtension) {
}

// SetDefaults_RollingUpgrade sets defaults for RollingUpgrade
func SetDefaults_RollingUpgrade(obj *RollingUpgrade) {
	// The RollingUpgrade name must be the same as the name of MachineSet,
	// this way can ensure MachineSet has only one RollingUpgrade bounded.
	obj.Name = obj.Spec.MachineSetName

	SetDefaults_RollingUpgradeSpec(&obj.Spec)
}

// SetDefaults_RollingUpgradeSpec sets defaults for RollingUpgrade spec
func SetDefaults_RollingUpgradeSpec(obj *RollingUpgradeSpec) {

	if obj.AutoUpgrade == nil {
		obj.AutoUpgrade = pointer.Bool(true)
	}

	// Set default upgrade options.
	if obj.UpgradeOptions.AutoUpgradeStartTime == "" {
		obj.UpgradeOptions.AutoUpgradeStartTime = "24:00"
	}
	if obj.UpgradeOptions.Duration.String() == "" {
		obj.UpgradeOptions.Duration = metav1.Duration{
			Duration: 2 * time.Hour,
		}
	}
	// TODO: Check if `weeklyPeriod` and `components` default value can be setted
	if len(obj.UpgradeOptions.WeeklyPeriod) == 0 {
		obj.UpgradeOptions.WeeklyPeriod = AvailableWeeklyPeriods
	}
	// Set default upgrade node components.
	if len(obj.Components) == 0 {
		obj.Components = []ComponentType{KubeletComponentType, RuntimeComponentType}
	}
	obj.Components = removeDuplicateComponent(obj.Components)
	obj.UpgradeOptions.WeeklyPeriod = removeDuplicate(obj.UpgradeOptions.WeeklyPeriod)

	if obj.Strategy.Type == "" {
		obj.Strategy.Type = RandomUpdateStrategy
	}

	if obj.Strategy.MaxUnavailable == nil {
		// Set default MaxUnavailable as 25% by default.
		maxUnavailable := intstr.FromInt(1)
		obj.Strategy.MaxUnavailable = &maxUnavailable
	}
	if obj.Strategy.UpgradeTimeout == nil {
		obj.Strategy.UpgradeTimeout = &metav1.Duration{
			Duration: 2 * time.Minute,
		}
	}

	if obj.IgnoreUpgradeFailures == nil {
		obj.IgnoreUpgradeFailures = pointer.Bool(false)
	}
}

// Remove duplicates or string
func removeDuplicate(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		if v != "" && !m[v] {
			s[len(m)] = v
			m[v] = true
		}
	}
	s = s[:len(m)]
	return s
}

// RemoveDuplicateKVPairs **NOTICE** will skip line without format key=value
func RemoveDuplicateKVPairs(lines []string) []string {
	reserve := func(lines []string) {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	reserve(lines)
	res := []string{}
	set := sets.NewString()
	for _, line := range lines {
		pair := strings.SplitN(line, "=", 2)
		if len(pair) < 2 {
			continue
		}
		if set.Has(pair[0]) {
			continue
		}
		set.Insert(pair[0])
		res = append(res, pair[0]+"="+pair[1])
	}
	reserve(res)
	return res
}

func removeDuplicateComponent(s []ComponentType) []ComponentType {
	m := map[ComponentType]bool{}
	for _, v := range s {
		if string(v) != "" && !m[v] {
			s[len(m)] = v
			m[v] = true
		}
	}
	s = s[:len(m)]
	return s
}
