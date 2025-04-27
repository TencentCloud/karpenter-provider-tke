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
	op "github.com/awslabs/operatorpkg/status"
)

// Subnet contains resolved Subnet selector values utilized for node launch
type Subnet struct {
	// ID of the subnet
	// +required
	ID string `json:"id"`
	// The associated availability zone
	// +required
	Zone string `json:"zone"`
	// The associated availability zone ID
	// +optional
	ZoneID string `json:"zoneID,omitempty"`
}

// SecurityGroup contains resolved SecurityGroup selector values utilized for node launch
type SecurityGroup struct {
	// ID of the security group
	// +required
	ID string `json:"id"`
}

// SSHKey contains resolved SSHKey selector values utilized for node launch
type SSHKey struct {
	// ID of the ssh key paire
	// +required
	ID string `json:"id"`
}

// TKEMachineNodeClassStatus contains the resolved state of the TKEMachineNodeClass
type TKEMachineNodeClassStatus struct {
	// Subnets contains the current Subnet values that are available to the
	// cluster under the subnet selectors.
	// +optional
	Subnets []Subnet `json:"subnets,omitempty"`
	// SecurityGroups contains the current Security Groups values that are available to the
	// cluster under the SecurityGroups selectors.
	// +optional
	SecurityGroups []SecurityGroup `json:"securityGroups,omitempty"`
	// SSHKeys contains the current SSH Key values that are available to the
	// cluster under the SSH Keys selectors.
	// +optional
	SSHKeys []SSHKey `json:"sshKeys,omitempty"`
	// Conditions contains signals for health and readiness
	// +optional
	Conditions []op.Condition `json:"conditions,omitempty"`
}

const (
	// 	ConditionTypeNodeClassReady = "Ready" condition indicates that subnets, security groups, AMIs and instance profile for nodeClass were resolved
	ConditionTypeNodeClassReady = "Ready"
)

func (in *TKEMachineNodeClass) StatusConditions() op.ConditionSet {
	return op.NewReadyConditions(ConditionTypeNodeClassReady).For(in)
}

func (in *TKEMachineNodeClass) GetConditions() []op.Condition {
	return in.Status.Conditions
}

func (in *TKEMachineNodeClass) SetConditions(conditions []op.Condition) {
	in.Status.Conditions = conditions
}
