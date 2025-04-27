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

package sshkey

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	cvm2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Provider interface {
	List(context.Context, *api.TKEMachineNodeClass) ([]*cvm2017.KeyPair, error)
}

type DefaultProvider struct {
	client *cvm2017.Client
}

func NewDefaultProvider(_ context.Context, client *cvm2017.Client) *DefaultProvider {
	return &DefaultProvider{
		client: client,
	}
}

func (p *DefaultProvider) List(ctx context.Context, nodeClass *api.TKEMachineNodeClass) ([]*cvm2017.KeyPair, error) {
	ids, filterSets := getFilterSets(nodeClass.Spec.SSHKeySelectorTerms)
	if len(filterSets) == 0 && len(ids) == 0 {
		return []*cvm2017.KeyPair{}, nil
	}

	keypairs := map[string]*cvm2017.KeyPair{}
	if len(ids) != 0 {
		req := cvm2017.NewDescribeKeyPairsRequest()
		req.KeyIds = ids
		resp, err := p.client.DescribeKeyPairs(req)
		if err != nil {
			return nil, fmt.Errorf("describe key pairs failed: %v", err)
		}
		log.FromContext(ctx).WithValues("process", "listsshkeyID").V(1).Info("tencent cloud request", "action", req.GetAction(), "requestID", resp.Response.RequestId)

		for _, keypair := range resp.Response.KeyPairSet {
			keypairs[lo.FromPtr(keypair.KeyId)] = keypair
		}
	}

	for _, filter := range filterSets {
		req := cvm2017.NewDescribeKeyPairsRequest()
		req.Filters = append(req.Filters, filter...)
		resp, err := p.client.DescribeKeyPairs(req)
		if err != nil {
			return nil, fmt.Errorf("describe key pairs failed: %v", err)
		}
		log.FromContext(ctx).WithValues("process", "listsshkeyTag").V(1).Info("tencent cloud request", "action", req.GetAction(), "requestID", resp.Response.RequestId)
		for _, keypair := range resp.Response.KeyPairSet {
			keypairs[lo.FromPtr(keypair.KeyId)] = keypair
		}
	}

	return lo.Values(keypairs), nil
}

func getFilterSets(terms []api.SSHKeySelectorTerm) (ids []*string, res [][]*cvm2017.Filter) {
	for _, term := range terms {
		switch {
		case term.ID != "":
			ids = append(ids, lo.ToPtr(term.ID))
		default:
			var filters []*cvm2017.Filter
			for k, v := range term.Tags {
				if v == "*" {
					filters = append(filters, &cvm2017.Filter{
						Name:   lo.ToPtr("tag-key"),
						Values: []*string{lo.ToPtr(k)},
					})
				} else {
					filters = append(filters, &cvm2017.Filter{
						Name:   lo.ToPtr(fmt.Sprintf("tag:%s", k)),
						Values: []*string{lo.ToPtr(v)},
					})
				}
			}
			res = append(res, filters)
		}
	}
	return ids, res
}
