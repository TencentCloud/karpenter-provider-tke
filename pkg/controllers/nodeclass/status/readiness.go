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

package status

import (
	"context"

	"github.com/awslabs/operatorpkg/status"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
)

type Readiness struct {
}

func (n Readiness) Reconcile(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (reconcile.Result, error) {
	if len(nodeClass.Status.Subnets) == 0 {
		nodeClass.StatusConditions().SetFalse(status.ConditionReady, "NodeClassNotReady", "Failed to resolve subnets")
		return reconcile.Result{}, nil
	}
	if len(nodeClass.Status.SecurityGroups) == 0 {
		nodeClass.StatusConditions().SetFalse(status.ConditionReady, "NodeClassNotReady", "Failed to resolve security groups")
		return reconcile.Result{}, nil
	}
	if len(nodeClass.Status.SSHKeys) == 0 {
		nodeClass.StatusConditions().SetFalse(status.ConditionReady, "NodeClassNotReady", "Failed to resolve ssh keys")
		return reconcile.Result{}, nil
	}
	// A NodeClass that uses AL2023 requires the cluster CIDR for launching nodes.
	// To allow Karpenter to be used for Non-EKS clusters, resolving the Cluster CIDR
	// will not be done at startup but instead in a reconcile loop.
	nodeClass.StatusConditions().SetTrue(status.ConditionReady)
	return reconcile.Result{}, nil
}
