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

package vpc

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	vpc2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (p *DefaultProvider) ListSubnets(ctx context.Context, nodeClass *api.TKEMachineNodeClass) ([]*vpc2017.Subnet, error) {
	filterSets := getSubnetFilterSets(nodeClass.Spec.SubnetSelectorTerms)
	if len(filterSets) == 0 {
		return []*vpc2017.Subnet{}, nil
	}
	vpcFilter := []*vpc2017.Filter{
		{
			Name:   lo.ToPtr("vpc-id"),
			Values: []*string{lo.ToPtr(p.vpcID)},
		},
	}

	subnets := map[string]*vpc2017.Subnet{}
	for _, filter := range filterSets {
		req := vpc2017.NewDescribeSubnetsRequest()
		req.Filters = append(req.Filters, vpcFilter...)
		req.Filters = append(req.Filters, filter...)
		resp, err := p.client.DescribeSubnets(req)
		if err != nil {
			return nil, fmt.Errorf("describe subnets failed: %v", err)
		}
		log.FromContext(ctx).WithValues("process", "listsubnet").V(1).Info("tencent cloud request", "action", req.GetAction(), "requestID", resp.Response.RequestId)
		for _, subnet := range resp.Response.SubnetSet {
			subnets[lo.FromPtr(subnet.SubnetId)] = subnet
		}
	}

	return lo.Values(subnets), nil
}

func getSubnetFilterSets(terms []api.SubnetSelectorTerm) (res [][]*vpc2017.Filter) {
	idFilter := &vpc2017.Filter{Name: lo.ToPtr("subnet-id")}
	for _, term := range terms {
		switch {
		case term.ID != "":
			idFilter.Values = append(idFilter.Values, lo.ToPtr(term.ID))
		default:
			var filters []*vpc2017.Filter
			for k, v := range term.Tags {
				if v == "*" {
					filters = append(filters, &vpc2017.Filter{
						Name:   lo.ToPtr("tag-key"),
						Values: []*string{lo.ToPtr(k)},
					})
				} else {
					filters = append(filters, &vpc2017.Filter{
						Name:   lo.ToPtr(fmt.Sprintf("tag:%s", k)),
						Values: []*string{lo.ToPtr(v)},
					})
				}
			}
			res = append(res, filters)
		}
	}
	if len(idFilter.Values) > 0 {
		res = append(res, []*vpc2017.Filter{idFilter})
	}
	return res
}
