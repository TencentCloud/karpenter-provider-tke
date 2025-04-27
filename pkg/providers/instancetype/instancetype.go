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

package instancetype

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/operator/options"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tchttp "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/http"
	tke2018 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tke/v20180525"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

type Provider interface {
	List(context.Context,
		// *v1beta1.KubeletConfiguration,
		*api.TKEMachineNodeClass, bool) ([]*cloudprovider.InstanceType, error)
	BlockInstanceType(ctx context.Context, instName, capacityType, zone, message string)
	GetInsufficientFailureCount(ctx context.Context, instName, capacityType, zone string) int
	AddInsufficientFailure(ctx context.Context, instName, capacityType, zone string)
}

type DefaultProvider struct {
	region         string
	zoneProvider   zone.Provider
	client         *common.Client
	client2018     *tke2018.Client
	k8sclient      kubernetes.Interface
	rtclient       client.Client
	providerCache  *cache.Cache
	blacklistCache *cache.Cache
}

func NewDefaultProvider(_ context.Context, region string, kc kubernetes.Interface, rtc client.Client, zoneProvider zone.Provider, client *common.Client, client2018 *tke2018.Client, cache, blacklistCache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		region:         region,
		zoneProvider:   zoneProvider,
		client:         client,
		client2018:     client2018,
		k8sclient:      kc,
		rtclient:       rtc,
		providerCache:  cache,
		blacklistCache: blacklistCache,
	}
}

func (p *DefaultProvider) List(ctx context.Context,
	nodeClass *api.TKEMachineNodeClass, refresh bool) ([]*cloudprovider.InstanceType, error) {
	if refresh {
		p.providerCache.Flush()
	}
	v, err := p.k8sclient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get server version failed: %v", err)
	}
	currentVersion, err := semver.Make(normalizeVersion(v.GitVersion))
	if err != nil {
		return nil, fmt.Errorf("parse server version failed: %v", err)
	}

	if len(nodeClass.Status.Subnets) == 0 {
		return nil, fmt.Errorf("no subnets found")
	}

	subnetZones := sets.New(lo.Map(nodeClass.Status.Subnets, func(s api.Subnet, _ int) string {
		return s.Zone
	})...)
	subnetZonesHash, _ := hashstructure.Hash(subnetZones, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})

	odkey := fmt.Sprintf("instance-types-od-%016x", subnetZonesHash)
	spotkey := fmt.Sprintf("instance-types-spot-%016x", subnetZonesHash)
	enikey := fmt.Sprintf("eni-limits-spot-%016x", subnetZonesHash)
	clsinfokey := "cluster-info"

	var odTypes, spotTypes []cxm.InstanceTypeQuotaItem
	var eniLimits map[string][]*tke2018.PodLimitsInstance
	var clsInfo tke2018.Cluster

	if item, ok := p.providerCache.Get(odkey); ok {
		odTypes = item.([]cxm.InstanceTypeQuotaItem)
	} else {
		odTypes, err = p.getInstanceTypes(ctx, false, refresh, nodeClass)
		if err != nil {
			return nil, fmt.Errorf("get on-demand instance types failed: %v", err)
		}
		p.providerCache.SetDefault(odkey, odTypes)
	}

	if item, ok := p.providerCache.Get(spotkey); ok {
		spotTypes = item.([]cxm.InstanceTypeQuotaItem)
	} else {
		spotTypes, err = p.getInstanceTypes(ctx, true, refresh, nodeClass)
		if err != nil {
			return nil, fmt.Errorf("get spot instance types failed: %v", err)
		}
		p.providerCache.SetDefault(spotkey, spotTypes)
	}

	if item, ok := p.providerCache.Get(enikey); ok {
		eniLimits = item.(map[string][]*tke2018.PodLimitsInstance)
	} else {
		eniLimits, err = p.getENILimits(ctx, nodeClass)
		if err != nil {
			return nil, fmt.Errorf("get pod eni limits failed: %v", err)
		}
		p.providerCache.SetDefault(enikey, eniLimits)
	}

	if item, ok := p.providerCache.Get(clsinfokey); ok {
		clsInfo = item.(tke2018.Cluster)
	} else {
		clsreq := tke2018.NewDescribeClustersRequest()
		clsreq.ClusterIds = []*string{lo.ToPtr(options.FromContext(ctx).ClusterID)}
		resp, err := p.client2018.DescribeClusters(clsreq)

		if err != nil {
			return nil, fmt.Errorf("get cluster info failed: %v", err)
		}
		if len(resp.Response.Clusters) == 0 || resp.Response.Clusters[0] == nil {
			return nil, fmt.Errorf("no cluster found for %s", options.FromContext(ctx).ClusterID)
		}
		p.providerCache.SetDefault(clsinfokey, *resp.Response.Clusters[0])
	}

	var storageInGB int32

	if nodeClass.Spec.SystemDisk != nil {
		storageInGB = nodeClass.Spec.SystemDisk.Size - 15
	} else {
		storageInGB = 50 - 15
	}

	for _, d := range nodeClass.Spec.DataDisks {
		mountTarget := path.Clean(lo.FromPtr(d.MountTarget))
		if mountTarget == "/var/lib/container" ||
			mountTarget == "/var/lib/kubelet" ||
			mountTarget == "/var/lib" ||
			mountTarget == "/var" {
			storageInGB = d.Size - 5
		}
	}
	if storageInGB < 0 {
		storageInGB = 0
	}

	offeringsMap := map[string]cloudprovider.Offerings{}

	instanceTypeMap := lo.SliceToMap(odTypes, func(i cxm.InstanceTypeQuotaItem) (string, cxm.InstanceTypeQuotaItem) {
		if !p.isBlocked(i.InstanceType, v1.CapacityTypeOnDemand, i.Zone) {
			offeringsMap[i.InstanceType] = append(offeringsMap[i.InstanceType], p.createOfferings(ctx, v1.CapacityTypeOnDemand, i)...)
		}
		return i.InstanceType, i
	})

	for _, i := range spotTypes {
		if p.isBlocked(i.InstanceType, v1.CapacityTypeSpot, i.Zone) {
			continue
		}
		offeringsMap[i.InstanceType] = append(offeringsMap[i.InstanceType], p.createOfferings(ctx, v1.CapacityTypeSpot, i)...)
		if _, ok := instanceTypeMap[i.InstanceType]; !ok {
			instanceTypeMap[i.InstanceType] = i
		}
	}

	if len(p.blacklistCache.Items()) == 0 {
		blockedInstanceType.Reset()
	}

	return lo.MapToSlice(instanceTypeMap, func(k string, i cxm.InstanceTypeQuotaItem) *cloudprovider.InstanceType {
		return NewInstanceType(ctx, p.region, storageInGB, i, currentVersion,
			nil, nil, nil, nil, nil,
			offeringsMap[k], eniLimits[i.Zone], &clsInfo)
	}), nil

}

func (p *DefaultProvider) BlockInstanceType(ctx context.Context, instName, capacityType, zone, message string) {
	p.blacklistCache.SetDefault(fmt.Sprintf("blocked-ins-%s-%s-%s", instName, capacityType, zone), true)
	blockedInstanceType.With(prometheus.Labels{
		instanceTypeLabel: instName,
		capacityTypeLabel: capacityType,
		zoneLabel:         zone,
	}).Set(1)
	log.FromContext(ctx).WithValues("process", "blockinstancetype").Info(fmt.Sprintf("Instance type is blocked: %s", message), "instance-type", instName, "capacity-type", capacityType, "zone", zone)
}

func (p *DefaultProvider) GetInsufficientFailureCount(ctx context.Context, instName, capacityType, zone string) int {
	result, ok := p.blacklistCache.Get(fmt.Sprintf("failure-ins-%s-%s-%s", instName, capacityType, zone))
	if ok {
		return result.(int)
	}
	return 0
}

func (p *DefaultProvider) AddInsufficientFailure(ctx context.Context, instName, capacityType, zone string) {
	setCount := 1
	result, ok := p.blacklistCache.Get(fmt.Sprintf("failure-ins-%s-%s-%s", instName, capacityType, zone))
	if ok {
		setCount = setCount + result.(int)
	}
	p.blacklistCache.Set(fmt.Sprintf("failure-ins-%s-%s-%s", instName, capacityType, zone), setCount, time.Hour)
	return
}

func (p *DefaultProvider) isBlocked(instName, capacityType, zone string) bool {
	_, ok := p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", instName, capacityType, zone))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", "*", "*", "*"))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", instName, capacityType, "*"))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", instName, "*", zone))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", instName, "*", "*"))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", "*", capacityType, zone))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", "*", capacityType, "*"))
	if ok {
		return true
	}
	_, ok = p.blacklistCache.Get(fmt.Sprintf("blocked-ins-%s-%s-%s", "*", "*", zone))
	if ok {
		return true
	}
	return false
}

func (p *DefaultProvider) getInstanceTypes(ctx context.Context, isSpot, refresh bool, nodeClass *api.TKEMachineNodeClass) ([]cxm.InstanceTypeQuotaItem, error) {
	filters := []*tke2018.Filter{}
	nodeClaimList := &v1.NodeClaimList{}
	if err := p.rtclient.List(ctx, nodeClaimList); err != nil {
		return nil, fmt.Errorf("get nodeclaim failed: %v", err)
	}

	existedZones := lo.SliceToMap(nodeClaimList.Items, func(nc v1.NodeClaim) (string, bool) {
		if nc.Spec.NodeClassRef.Name == nodeClass.Name {
			return nc.GetLabels()[api.LabelCBSToplogy], true
		}
		return "", false
	})
	delete(existedZones, "")

	zones := lo.Map(nodeClass.Status.Subnets, func(s api.Subnet, _ int) *string {
		delete(existedZones, s.Zone)
		return &s.Zone
	})

	zones = append(zones, lo.MapToSlice(existedZones, func(k string, _ bool) *string {
		return lo.ToPtr(k)
	})...)

	zoneFilter := tke2018.Filter{
		Name:   lo.ToPtr("zone"),
		Values: zones,
	}

	chargeTypeFilter := tke2018.Filter{
		Name:   lo.ToPtr("instance-charge-type"),
		Values: nil,
	}
	if isSpot {
		chargeTypeFilter.Values = []*string{lo.ToPtr("SPOTPAID")}
	} else {
		chargeTypeFilter.Values = []*string{lo.ToPtr("POSTPAID_BY_HOUR")}
	}

	filters = append(filters, &zoneFilter, &chargeTypeFilter)

	if refresh {
		refreshFilter := tke2018.Filter{
			Name:   lo.ToPtr("refresh-cache"),
			Values: []*string{lo.ToPtr("true")},
		}
		filters = append(filters, &refreshFilter)
	}

	commonRequest := tchttp.NewCommonRequest("tke", "2022-05-01", "DescribeZoneInstanceConfigInfos")

	request := DescribeZoneInstanceConfigInfosRequest{Filters: filters}
	response := DescribeZoneInstanceConfigInfosResponse{}

	params, _ := json.Marshal(request)

	err := commonRequest.SetActionParameters(string(params))
	if err != nil {
		return nil, fmt.Errorf("set parameters failed: %v", err)
	}
	commonResponse := tchttp.NewCommonResponse()
	err = p.client.Send(commonRequest, commonResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance config infos, requestID: %v", err)
	}
	if err := commonResponse.ParseErrorFromHTTPResponse(commonResponse.GetBody()); err != nil {
		return nil, fmt.Errorf("failed to describe instance config infos: %v", err)
	}

	if err := json.Unmarshal(commonResponse.GetBody(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal instanceInfos failed: %v", err)
	}
	if response.Response == nil || response.Response.RequestId == nil || response.Response.InstanceTypeQuotaSet == nil {
		return nil, fmt.Errorf("invaild response: %v", commonResponse.GetBody())
	}

	log.FromContext(ctx).WithValues("process", "getinstancetypes").Info("tencent cloud request", "action", "DescribeZoneInstanceConfigInfos", "requestID", response.Response.RequestId)
	var instanceTypes = &[]cxm.InstanceTypeQuotaItem{}

	err = json.Unmarshal([]byte(lo.FromPtr(response.Response.InstanceTypeQuotaSet)), instanceTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal instanceTypeQuotaSet: %v", err)
	}
	return *instanceTypes, nil
}

func (p *DefaultProvider) createOfferings(ctx context.Context, capacityType string, insType cxm.InstanceTypeQuotaItem) []*cloudprovider.Offering {
	var offerings []*cloudprovider.Offering
	available := insType.Status == "SELL" && insType.Inventory > 0
	zoneID, _ := p.zoneProvider.IDFromZone(insType.Zone)
	offering := &cloudprovider.Offering{
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zoneID),
			scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, insType.Zone),
		),
		Price:     insType.Price.UnitPrice,
		Available: available,
	}
	offerings = append(offerings, offering)
	return offerings
}

func (p *DefaultProvider) getENILimits(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (map[string][]*tke2018.PodLimitsInstance, error) {

	limits := map[string][]*tke2018.PodLimitsInstance{}

	for _, s := range nodeClass.Status.Subnets {
		if _, ok := limits[s.Zone]; ok {
			continue
		}
		req := tke2018.NewDescribeVpcCniPodLimitsRequest()
		req.Zone = lo.ToPtr(s.Zone)
		resp, err := p.client2018.DescribeVpcCniPodLimitsWithContext(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to get vpc cni pod limits: %v", err)
		}
		log.FromContext(ctx).V(8).Info("tencent cloud request", "action", req.GetAction(), "requestID", resp.Response.RequestId)
		limits[s.Zone] = resp.Response.PodLimitsInstanceSet
	}

	return limits, nil
}

func normalizeVersion(v string) string {
	if strings.HasPrefix(v, "v") {
		// No need to account for unicode widths.
		return v[1:]
	}
	return v
}
