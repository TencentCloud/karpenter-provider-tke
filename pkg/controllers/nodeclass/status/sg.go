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
	"fmt"
	"sort"

	"time"

	vpcprovider "github.com/tencentcloud/karpenter-provider-tke/pkg/providers/vpc"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

type SecurityGroup struct {
	vpcProvider vpcprovider.Provider
}

func (s *SecurityGroup) Reconcile(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (reconcile.Result, error) {
	securityGroups, err := s.vpcProvider.ListSecurityGroups(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting securitygroups, %w", err)
	}
	if len(securityGroups) == 0 {
		nodeClass.Status.SecurityGroups = nil
		return reconcile.Result{}, nil
	}
	sort.Slice(securityGroups, func(i, j int) bool {
		return *securityGroups[i].SecurityGroupId < *securityGroups[j].SecurityGroupId
	})
	nodeClass.Status.SecurityGroups = lo.Map(securityGroups, func(sg *vpc.SecurityGroup, _ int) api.SecurityGroup {
		return api.SecurityGroup{
			ID: lo.FromPtr(sg.SecurityGroupId),
		}
	})

	return reconcile.Result{RequeueAfter: time.Minute}, nil
}
