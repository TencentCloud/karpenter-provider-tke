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

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	vpc2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

type Provider interface {
	ListSubnets(context.Context, *api.TKEMachineNodeClass) ([]*vpc2017.Subnet, error)
	ListSecurityGroups(ctx context.Context, nodeClass *api.TKEMachineNodeClass) ([]*vpc2017.SecurityGroup, error)
}

type DefaultProvider struct {
	client *vpc2017.Client
	vpcID  string
}

func NewDefaultProvider(_ context.Context, client *vpc2017.Client, vpcID string) *DefaultProvider {
	return &DefaultProvider{
		client: client,
		vpcID:  vpcID,
	}
}
