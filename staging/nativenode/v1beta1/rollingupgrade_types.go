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
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RollingUpgrade is the Schema for the rollingupgrades API.
type RollingUpgrade struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior for this RollingUpgrade.
	// +optional
	Spec   RollingUpgradeSpec   `json:"spec,omitempty"`
	Status RollingUpgradeStatus `json:"status,omitempty"`
}

// RollingUpgradeSpec defines the desired state of RollingUpgrade.
type RollingUpgradeSpec struct {
	// MachineSetName specify the name of the machineset for this rollingupgrade.
	// +optional
	MachineSetName string `json:"machineSetName,omitempty"`

	// AutoUpgrade specifies whether node auto-upgrade is enabled for the node
	// pool. If enabled, node auto-upgrade helps keep the nodes in your node pool
	// up to date with the latest release version of Kubernetes.
	// +optional
	AutoUpgrade *bool `json:"autoUpgrade"`

	// UpgradeOptions Specifies the Auto Upgrade knobs for the node pool.
	UpgradeOptions AutoUpgradeOptions `json:"upgradeOptions,omitempty"`

	// Components defines which component to upgrade when available
	// +optional
	// +patchStrategy=merge
	Components []ComponentType `json:"components,omitempty" patchStrategy:"merge"`

	Strategy UpdateStrategy `json:"strategy,omitempty"`

	// +optional
	DesiredKubeletVersion string `json:"desiredKubeletVersion,omitempty"`

	// +optional
	DesiredRuntimeVersion string `json:"desiredRuntimeVersion,omitempty"`

	// +optional
	IgnoreUpgradeFailures *bool `json:"ignoreUpgradeFailures,omitempty"`
}

// RollingUpgradeStatus defines the observed state of RollingUpgrade.
type RollingUpgradeStatus struct {
	Phase               string `json:"phase,omitempty"`
	CompletePercentage  string `json:"completePercentage,omitempty"`
	StartTime           string `json:"startTime,omitempty"`
	EndTime             string `json:"endTime,omitempty"`
	MachinesProcessed   int32  `json:"machinesProcessed,omitempty"`
	TotalMachines       int32  `json:"totalMachines,omitempty"`
	TotalProcessingTime string `json:"totalProcessingTime,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []RollingUpgradeCondition `json:"conditions,omitempty"          patchStrategy:"merge" patchMergeKey:"type"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RollingUpgradeList contains a list of RollingUpgrade.
type RollingUpgradeList struct {
	metav1.TypeMeta `                 json:",inline"`
	metav1.ListMeta `                 json:"metadata,omitempty"`
	Items           []RollingUpgrade `json:"items"`
}

// AutoUpgradeOptions defines the set of options for
// the user to control how the Auto Upgrades will proceed.
type AutoUpgradeOptions struct {
	// AutoUpgradeStartTime is set when upgrades
	// are about to commence with the approximate start time for the
	// upgrades, in RFC3339 (https://www.ietf.org/rfc/rfc3339.txt) text
	// format.
	AutoUpgradeStartTime string `json:"autoUpgradeStartTime,omitempty"`
	// Maintenance duration time, default 2h
	// Example: 3h
	// +optional
	Duration metav1.Duration `json:"duration,omitempty"`
	// WeeklyPeriod is the days to do the maintenance job.
	// Example: Wednesday,Thursday
	// +optional
	// +patchStrategy=merge
	WeeklyPeriod []string `json:"weeklyPeriod,omitempty"         patchStrategy:"merge"`
}

var (
	// AvailableWeeklyPeriods defines all available auto-upgrade weekly period.
	AvailableWeeklyPeriods = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
)

// ComponentType defines the type of node component.
type ComponentType string

// These are valid node component to upgrade.
const (
	// kubelet
	KubeletComponentType ComponentType = "Kubelet"
	// containerd
	RuntimeComponentType ComponentType = "Rumtime"
	// Linux OS
	KernelComponentType ComponentType = "OS"
	// Contains security vulnerabilities and configuration item convergence
	// in three dimensions: Kubenetes, Runtime, and OS
	NodeSecurityComponentType ComponentType = "NodeSecurity"
)

type RollingUpgradeStep string

const (
	// Status
	StatusUpgradable = "Upgradable"
	StatusUpgrading  = "Upgrading"
	StatusComplete   = "Completed"
	StatusFailed     = "Failed"

	// Conditions
	UpgradeComplete UpgradeConditionType = "Complete"

	MachineRotationKickoff   RollingUpgradeStep = "Kickoff"
	MachineRotationUpgrading RollingUpgradeStep = "Upgrading"
	//MachineRotationWaitUpgrade RollingUpgradeStep = "WaitUpgrade"
	MachineRotationCompleted RollingUpgradeStep = "Completed"
	MachineRotationUpgraded  RollingUpgradeStep = "Upgraded"
	MachineRotationTotal     RollingUpgradeStep = "Total"

	// Steps for future use
	MachineRotationDesiredMachineReady RollingUpgradeStep = "DesiredNodeReady"
	MachineRotationPredrainScript      RollingUpgradeStep = "PreDrainScript"
	MachineRotationDrain               RollingUpgradeStep = "Drain"
	MachineRotationPostdrainScript     RollingUpgradeStep = "PostDrainScript"
	MachineRotationPostWait            RollingUpgradeStep = "PostWait"
	MachineRotationTerminate           RollingUpgradeStep = "Terminate"
	MachineRotationPostTerminate       RollingUpgradeStep = "PostTerminate"
	MachineRotationTerminated          RollingUpgradeStep = "Terminated"
)

var MachineRotationStepOrders = map[RollingUpgradeStep]int{
	MachineRotationKickoff:   10,
	MachineRotationUpgrading: 20,
	//MachineRotationWaitUpgrade: 30,
	MachineRotationUpgraded: 50,
	MachineRotationTotal:    60,

	// Steps for future use
	MachineRotationDesiredMachineReady: 70,
	MachineRotationPredrainScript:      80,
	MachineRotationDrain:               90,
	MachineRotationPostdrainScript:     100,
	MachineRotationPostWait:            110,
	MachineRotationTerminate:           120,
	MachineRotationPostTerminate:       130,
	MachineRotationTerminated:          140,
	MachineRotationCompleted:           1000,
}

var (
	FiniteStates        = []string{StatusComplete, StatusFailed}
	AllowedStrategyType = []string{string(RandomUpdateStrategy), string(UniformAcrossAzUpdateStrategy)}
	DefaultRequeueTime  = time.Second * 30
)

// RollingUpgradeCondition describes the state of the RollingUpgrade
type RollingUpgradeCondition struct {
	Type   UpgradeConditionType   `json:"type,omitempty"`
	Status corev1.ConditionStatus `json:"status,omitempty"`
}

type UpdateStrategyType string
type UpgradeConditionType string

const (
	RandomUpdateStrategy          UpdateStrategyType = "randomUpdate"
	UniformAcrossAzUpdateStrategy UpdateStrategyType = "uniformAcrossAzUpdate"
)

// UpdateStrategy holds the information needed to perform update based on different update strategies
type UpdateStrategy struct {
	Type UpdateStrategyType `json:"type,omitempty"`
	// The maximum number of machines that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired machines (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0.
	// Defaults to 25%.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	UpgradeTimeout *metav1.Duration    `json:"upgradeTimeout,omitempty"`
}

// NamespacedName returns namespaced name of the object.
func (r *RollingUpgrade) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: r.Namespace, Name: r.Name}
}

func (r *RollingUpgrade) MachineSetName() string {
	return r.Spec.MachineSetName
}

func (r *RollingUpgrade) UpgradeTimeout() *metav1.Duration {
	return r.Spec.Strategy.UpgradeTimeout
}

func (r *RollingUpgrade) Phase() string {
	return r.Status.Phase
}

func (r *RollingUpgrade) UpdateStrategyType() UpdateStrategyType {
	return r.Spec.Strategy.Type
}

func (r *RollingUpgrade) MaxUnavailable() intstr.IntOrString {
	return *r.Spec.Strategy.MaxUnavailable
}

func (r *RollingUpgrade) SetPhase(status string) {
	r.Status.Phase = status
}

func (r *RollingUpgrade) SetStartTime(t string) {
	r.Status.StartTime = t
}

func (r *RollingUpgrade) StartTime() string {
	return r.Status.StartTime
}

func (r *RollingUpgrade) SetEndTime(t string) {
	r.Status.EndTime = t
}

func (r *RollingUpgrade) EndTime() string {
	return r.Status.EndTime
}

func (r *RollingUpgrade) SetTotalProcessingTime(t string) {
	r.Status.TotalProcessingTime = t
}

func (r *RollingUpgrade) SetTotalMachines(n int) {
	r.Status.TotalMachines = int32(n)
}

func (r *RollingUpgrade) SetMachinesProcessed(n int) {
	r.Status.MachinesProcessed = int32(n)
}

func (r *RollingUpgrade) SetCompletePercentage(n int) {
	r.Status.CompletePercentage = fmt.Sprintf("%s%%", strconv.Itoa(n))
}
func (r *RollingUpgrade) GetStatus() RollingUpgradeStatus {
	return r.Status
}

func (r *RollingUpgrade) IsIgnoreUpgradeFailures() *bool {
	return r.Spec.IgnoreUpgradeFailures
}

func (s *RollingUpgradeStatus) SetCondition(cond RollingUpgradeCondition) {
	// if condition exists, overwrite, otherwise append
	for ix, c := range s.Conditions {
		if c.Type == cond.Type {
			s.Conditions[ix] = cond
			return
		}
	}
	s.Conditions = append(s.Conditions, cond)
}
