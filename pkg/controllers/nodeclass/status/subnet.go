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
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

type Subnet struct {
	zoneProvider zone.Provider
	vpcProvider  vpcprovider.Provider
}

func (s *Subnet) Reconcile(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (reconcile.Result, error) {
	subnets, err := s.vpcProvider.ListSubnets(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting subnets, %w", err)
	}
	if len(subnets) == 0 {
		nodeClass.Status.Subnets = nil
		return reconcile.Result{}, nil
	}
	sort.Slice(subnets, func(i, j int) bool {
		if int(lo.FromPtr(subnets[i].AvailableIpAddressCount)) != int(lo.FromPtr(subnets[j].AvailableIpAddressCount)) {
			return int(lo.FromPtr(subnets[i].AvailableIpAddressCount)) > int(lo.FromPtr(subnets[j].AvailableIpAddressCount))
		}
		return lo.FromPtr(subnets[i].SubnetId) < lo.FromPtr(subnets[j].SubnetId)
	})
	nodeClass.Status.Subnets = lo.Map(subnets, func(vpcsubnet *vpc.Subnet, _ int) api.Subnet {
		zoneID, _ := s.zoneProvider.IDFromZone(lo.FromPtr(vpcsubnet.Zone))
		return api.Subnet{
			ID:     lo.FromPtr(vpcsubnet.SubnetId),
			Zone:   lo.FromPtr(vpcsubnet.Zone),
			ZoneID: zoneID,
		}
	})

	return reconcile.Result{RequeueAfter: time.Minute}, nil
}
