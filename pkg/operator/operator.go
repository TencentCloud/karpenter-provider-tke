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

package operator

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/operator/options"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/instancetype"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/machine"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/sshkey"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/vpc"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	tke2018 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tke/v20180525"
	vpc2017 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/operator"
)

func init() {
	v1.RestrictedLabelDomains = v1.RestrictedLabelDomains.Insert(api.Group)
	lo.Must0(apis.AddToScheme(scheme.Scheme))
}

type Operator struct {
	*operator.Operator

	MachineProvider      machine.Provider
	InstanceTypeProvider instancetype.Provider
	ZoneProvider         zone.Provider
	VPCProvider          vpc.Provider
	SSHKeyProvider       sshkey.Provider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	var credential common.CredentialIface
	credential = common.NewCredential(
		strings.ReplaceAll(
			options.FromContext(ctx).SecretID, "\n", ""),
		strings.ReplaceAll(
			options.FromContext(ctx).SecretKey, "\n", ""),
	)
	pf := profile.NewClientProfile()
	pf.Language = "en-US"
	pf.UnsafeRetryOnConnectionFailure = true
	pf.HttpProfile.RootDomain = "internal.tencentcloudapi.com"
	commonClient := common.NewCommonClient(credential, options.FromContext(ctx).Region, pf)
	client2018, _ := tke2018.NewClient(credential, options.FromContext(ctx).Region, pf)
	vpcClient, _ := vpc2017.NewClient(credential, options.FromContext(ctx).Region, pf)
	cvmClient, _ := cvm2017.NewClient(credential, options.FromContext(ctx).Region, pf)

	clsreq := tke2018.NewDescribeClustersRequest()
	clsreq.ClusterIds = []*string{lo.ToPtr(options.FromContext(ctx).ClusterID)}
	resp, err := client2018.DescribeClusters(clsreq)
	if err != nil {
		log.Panicf("DescribeClusters failed: %v", err)
	}
	if len(resp.Response.Clusters) == 0 {
		log.Panicf("DescribeClusters failed: no cluster found")
	}
	zoneProvider := zone.NewDefaultProvider(ctx)
	vpcProvider := vpc.NewDefaultProvider(ctx, vpcClient, lo.FromPtr(resp.Response.Clusters[0].ClusterNetworkSettings.VpcId))
	sshKeyProvider := sshkey.NewDefaultProvider(ctx, cvmClient)

	machineProvider := machine.NewDefaultProvider(ctx, operator.GetClient(), zoneProvider, options.FromContext(ctx).ClusterID)
	instanceTypeProvider := instancetype.NewDefaultProvider(ctx, options.FromContext(ctx).Region, operator.KubernetesInterface, operator.GetClient(), zoneProvider, commonClient, client2018, cache.New(10*time.Minute, time.Minute), cache.New(30*time.Minute, time.Minute))

	return ctx, &Operator{
		Operator:             operator,
		MachineProvider:      machineProvider,
		InstanceTypeProvider: instanceTypeProvider,
		ZoneProvider:         zoneProvider,
		VPCProvider:          vpcProvider,
		SSHKeyProvider:       sshKeyProvider,
	}
}
