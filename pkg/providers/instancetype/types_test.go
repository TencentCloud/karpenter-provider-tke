package instancetype

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/samber/lo"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/operator/options"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
	tke2018 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tke/v20180525"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func testCtx() context.Context {
	return options.ToContext(context.Background(), &options.Options{
		VMMemoryOverheadPercent: 0.075,
		Region:                 "ap-guangzhou",
		ClusterID:              "cls-test",
		SecretID:               "test",
		SecretKey:              "test",
	})
}

func TestComputeEvictionSignal_Percentage(t *testing.T) {
	capacity := resource.MustParse("8Gi")
	result := computeEvictionSignal(capacity, "10%")
	// 8Gi = 8589934592 bytes, 10% = 858993459.2, ceil = 858993460
	expected := math.Ceil(capacity.AsApproximateFloat64() / 100 * 10)
	if result.AsApproximateFloat64() != expected {
		t.Errorf("expected %f, got %f", expected, result.AsApproximateFloat64())
	}
}

func TestComputeEvictionSignal_AbsoluteValue(t *testing.T) {
	capacity := resource.MustParse("8Gi")
	result := computeEvictionSignal(capacity, "500Mi")
	expected := resource.MustParse("500Mi")
	if result.Cmp(expected) != 0 {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestMustParsePercentage_Normal(t *testing.T) {
	got := mustParsePercentage("10%")
	if got != 10.0 {
		t.Errorf("expected 10.0, got %f", got)
	}
}

func TestMustParsePercentage_100Percent(t *testing.T) {
	got := mustParsePercentage("100%")
	if got != 0 {
		t.Errorf("expected 0 for 100%% (disabled), got %f", got)
	}
}

func TestMustParsePercentage_Float(t *testing.T) {
	got := mustParsePercentage("15.5%")
	if got != 15.5 {
		t.Errorf("expected 15.5, got %f", got)
	}
}

func TestMustParsePercentage_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid percentage")
		}
	}()
	mustParsePercentage("abc%")
}

func TestSystemReservedResources_Empty(t *testing.T) {
	result := systemReservedResources(map[string]string{})
	if len(result) != 0 {
		t.Errorf("expected empty resource list, got %d items", len(result))
	}
}

func TestSystemReservedResources_WithValues(t *testing.T) {
	result := systemReservedResources(map[string]string{
		"cpu":    "100m",
		"memory": "256Mi",
	})
	if len(result) != 2 {
		t.Errorf("expected 2 resources, got %d", len(result))
	}
	cpu := result[corev1.ResourceName("cpu")]
	if cpu.String() != "100m" {
		t.Errorf("expected 100m cpu, got %s", cpu.String())
	}
}

func TestKubeReservedResources_Post129(t *testing.T) {
	version := semver.MustParse("1.30.0")
	cpus := resource.MustParse("4")
	pods := resource.MustParse("110")
	result := kubeReservedResources(version, &cpus, &pods, 8, nil)
	if _, ok := result[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU reservation")
	}
	if _, ok := result[corev1.ResourceMemory]; !ok {
		t.Error("expected memory reservation")
	}
}

func TestKubeReservedResources_Pre129(t *testing.T) {
	version := semver.MustParse("1.28.0")
	cpus := resource.MustParse("4")
	pods := resource.MustParse("110")
	result := kubeReservedResources(version, &cpus, &pods, 8, nil)
	if _, ok := result[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU reservation")
	}
	if _, ok := result[corev1.ResourceMemory]; !ok {
		t.Error("expected memory reservation")
	}
}

func TestKubeReservedResources_WithOverride(t *testing.T) {
	version := semver.MustParse("1.30.0")
	cpus := resource.MustParse("4")
	pods := resource.MustParse("110")
	override := map[string]string{
		"cpu":    "500m",
		"memory": "1Gi",
	}
	result := kubeReservedResources(version, &cpus, &pods, 8, override)
	// Override should replace computed values
	cpu := result[corev1.ResourceCPU]
	if cpu.String() != "500m" {
		t.Errorf("expected override 500m cpu, got %s", cpu.String())
	}
}

func TestEvictionThreshold_Default(t *testing.T) {
	mem := resource.MustParse("8Gi")
	storage := resource.MustParse("50G")
	result := evictionThreshold(&mem, &storage, nil)
	// Default memory: 100Mi
	memOverhead := result[corev1.ResourceMemory]
	expected := resource.MustParse("100Mi")
	if memOverhead.Cmp(expected) != 0 {
		t.Errorf("expected 100Mi memory overhead, got %s", memOverhead.String())
	}
	// Default storage: ceil(50G / 100 * 10)
	if _, ok := result[corev1.ResourceEphemeralStorage]; !ok {
		t.Error("expected ephemeral storage eviction threshold")
	}
}

func TestEvictionThreshold_WithEvictionHard(t *testing.T) {
	mem := resource.MustParse("8Gi")
	storage := resource.MustParse("50G")
	evictionHard := map[string]string{
		MemoryAvailable: "500Mi",
		NodeFSAvailable: "15%",
	}
	result := evictionThreshold(&mem, &storage, evictionHard)
	memOverhead := result[corev1.ResourceMemory]
	expected := resource.MustParse("500Mi")
	if memOverhead.Cmp(expected) != 0 {
		t.Errorf("expected 500Mi memory eviction threshold, got %s", memOverhead.String())
	}
}

func TestPods_WithMaxPods(t *testing.T) {
	ctx := testCtx()
	instanceTypeInfo := cxm.InstanceTypeQuotaItem{CPU: 4}
	maxPods := lo.ToPtr(int32(50))
	result := pods(ctx, instanceTypeInfo, nil, maxPods, nil)
	if result.Value() != 50 {
		t.Errorf("expected 50 pods, got %d", result.Value())
	}
}

func TestPods_WithENI(t *testing.T) {
	ctx := testCtx()
	instanceTypeInfo := cxm.InstanceTypeQuotaItem{CPU: 4}
	eniCount := lo.ToPtr(int64(100))
	result := pods(ctx, instanceTypeInfo, eniCount, nil, nil)
	if result.Value() != 100 {
		t.Errorf("expected 100 pods (from ENI), got %d", result.Value())
	}
}

func TestPods_WithENI_MinimumEnforced(t *testing.T) {
	ctx := testCtx()
	instanceTypeInfo := cxm.InstanceTypeQuotaItem{CPU: 4}
	eniCount := lo.ToPtr(int64(30))
	result := pods(ctx, instanceTypeInfo, eniCount, nil, nil)
	if result.Value() != 61 {
		t.Errorf("expected 61 pods (minimum ENI), got %d", result.Value())
	}
}

func TestPods_Default(t *testing.T) {
	ctx := testCtx()
	instanceTypeInfo := cxm.InstanceTypeQuotaItem{CPU: 4}
	result := pods(ctx, instanceTypeInfo, nil, nil, nil)
	if result.Value() != 110 {
		t.Errorf("expected 110 pods (default), got %d", result.Value())
	}
}

func TestPods_WithPodsPerCore(t *testing.T) {
	ctx := testCtx()
	instanceTypeInfo := cxm.InstanceTypeQuotaItem{CPU: 4}
	podsPerCore := lo.ToPtr(int32(10))
	result := pods(ctx, instanceTypeInfo, nil, nil, podsPerCore)
	// min(4*10=40, 110) = 40
	if result.Value() != 40 {
		t.Errorf("expected 40 pods, got %d", result.Value())
	}
}

func TestEip_SmallCPU(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{CPU: 2}
	result := eip(ctx, inst)
	// CPU 1-5: 2-1 = 1
	if result.Value() != 1 {
		t.Errorf("expected 1 eip for 2 CPU, got %d", result.Value())
	}
}

func TestEip_MediumCPU(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{CPU: 8}
	result := eip(ctx, inst)
	// CPU 6-11: 3-1 = 2
	if result.Value() != 2 {
		t.Errorf("expected 2 eip for 8 CPU, got %d", result.Value())
	}
}

func TestEip_LargeCPU(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{CPU: 48}
	result := eip(ctx, inst)
	// CPU >= 48: 10-1 = 9
	if result.Value() != 9 {
		t.Errorf("expected 9 eip for 48 CPU, got %d", result.Value())
	}
}

func TestEni_MatchingInstance(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{InstanceType: "S5.LARGE8"}
	eniIP := lo.ToPtr(int64(10))
	directENI := lo.ToPtr(int64(5))
	subENI := lo.ToPtr(int64(3))
	limits := []*tke2018.PodLimitsInstance{
		{
			InstanceType: lo.ToPtr("S5.LARGE8"),
			PodLimits: &tke2018.PodLimitsByType{
				TKERouteENIStaticIP: eniIP,
				TKEDirectENI:        directENI,
				TKESubENI:           subENI,
			},
		},
	}
	gotENI, gotDirect, gotSub := eni(ctx, inst, limits)
	if lo.FromPtr(gotENI) != 10 {
		t.Errorf("expected eni 10, got %d", lo.FromPtr(gotENI))
	}
	if lo.FromPtr(gotDirect) != 5 {
		t.Errorf("expected direct eni 5, got %d", lo.FromPtr(gotDirect))
	}
	if lo.FromPtr(gotSub) != 3 {
		t.Errorf("expected sub eni 3, got %d", lo.FromPtr(gotSub))
	}
}

func TestEni_NoMatch(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{InstanceType: "S5.LARGE8"}
	limits := []*tke2018.PodLimitsInstance{
		{
			InstanceType: lo.ToPtr("S6.LARGE16"),
			PodLimits: &tke2018.PodLimitsByType{
				TKERouteENIStaticIP: lo.ToPtr(int64(10)),
			},
		},
	}
	gotENI, gotDirect, gotSub := eni(ctx, inst, limits)
	if gotENI != nil {
		t.Errorf("expected nil eni for non-matching instance")
	}
	if gotDirect != nil {
		t.Errorf("expected nil direct eni for non-matching instance")
	}
	if gotSub != nil {
		t.Errorf("expected nil sub eni for non-matching instance")
	}
}

func TestEni_EmptyLimits(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{InstanceType: "S5.LARGE8"}
	gotENI, gotDirect, gotSub := eni(ctx, inst, nil)
	if gotENI != nil || gotDirect != nil || gotSub != nil {
		t.Error("expected all nil for empty limits")
	}
}

func TestMemory(t *testing.T) {
	ctx := testCtx()
	// 4GB in bytes
	memBytes := 4 * 1024 * 1024 * 1024
	result := memory(ctx, memBytes)
	// memory = ceil((memBytes - kdump) * (1 - 0.075))
	kdump := KdumpLevels.eval(memBytes)
	expected := math.Ceil(float64((memBytes - kdump)) * (1 - 0.075))
	if result.AsApproximateFloat64() != expected {
		t.Errorf("expected %f, got %f", expected, result.AsApproximateFloat64())
	}
}

func TestComputeCapacity(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		CPU:          4,
		Memory:       8,
	}
	result := computeCapacity(ctx, 50, inst, nil, nil, nil)
	if _, ok := result[corev1.ResourceCPU]; !ok {
		t.Error("expected CPU in capacity")
	}
	if _, ok := result[corev1.ResourceMemory]; !ok {
		t.Error("expected memory in capacity")
	}
	if _, ok := result[corev1.ResourceEphemeralStorage]; !ok {
		t.Error("expected ephemeral storage in capacity")
	}
	if _, ok := result[corev1.ResourcePods]; !ok {
		t.Error("expected pods in capacity")
	}
}

func TestComputeCapacity_WithGPU(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType: "GN10X.LARGE40",
		CPU:          4,
		Memory:       40,
		Gpu:          1,
	}
	result := computeCapacity(ctx, 50, inst, nil, nil, nil)
	gpu, ok := result[corev1.ResourceName(api.ResourceNVIDIAGPU)]
	if !ok {
		t.Error("expected GPU in capacity for GPU instance")
	}
	if gpu.Value() != 1 {
		t.Errorf("expected 1 GPU, got %d", gpu.Value())
	}
}

func TestComputeRequirements(t *testing.T) {
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	reqs := computeRequirements(offerings, "ap-guangzhou", inst)
	if got := reqs.Get(corev1.LabelInstanceTypeStable); !got.Has("S5.LARGE8") {
		t.Errorf("expected instance type S5.LARGE8 in requirements")
	}
	if got := reqs.Get(corev1.LabelArchStable); !got.Has("amd64") {
		t.Errorf("expected arch amd64 in requirements")
	}
	if got := reqs.Get(api.LabelInstanceFamily); !got.Has("S5") {
		t.Errorf("expected instance family S5 in requirements")
	}
	if got := reqs.Get(api.LabelInstanceCPU); !got.Has("4") {
		t.Errorf("expected 4 CPU in requirements")
	}
	if got := reqs.Get(api.LabelInstanceMemoryGB); !got.Has("8") {
		t.Errorf("expected 8 GB memory in requirements")
	}
}

func TestComputeRequirements_DefaultArch(t *testing.T) {
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		// Arch is empty, should default to amd64
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	reqs := computeRequirements(offerings, "ap-guangzhou", inst)
	if got := reqs.Get(corev1.LabelArchStable); !got.Has("amd64") {
		t.Errorf("expected default arch amd64 when empty")
	}
}

func TestNewInstanceType(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")
	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, nil, nil)
	if it.Name != "S5.LARGE8" {
		t.Errorf("expected name S5.LARGE8, got %s", it.Name)
	}
	if it.Overhead == nil {
		t.Fatal("expected non-nil overhead")
	}
	if len(it.Capacity) == 0 {
		t.Error("expected non-empty capacity")
	}
}

func TestNewInstanceType_WithClusterInfo(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")

	// Test with cluster info that has non VPC-CNI network type and valid MaxNodePodNum
	property := `{"NetworkType":"GR"}`
	maxNodePodNum := uint64(64)
	clsInfo := &tke2018.Cluster{
		Property: &property,
		ClusterNetworkSettings: &tke2018.ClusterNetworkSettings{
			MaxNodePodNum: &maxNodePodNum,
		},
	}

	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, nil, clsInfo)
	if it == nil {
		t.Fatal("expected non-nil instance type")
	}
	// maxPods should be set to MaxNodePodNum - 3 = 61
	podsQty := it.Capacity[corev1.ResourcePods]
	if podsQty.Value() != 61 {
		t.Errorf("expected 61 pods from cluster info, got %d", podsQty.Value())
	}
}

func TestNewInstanceType_VPCCNICluster(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")

	// VPC-CNI should not override maxPods
	property := `{"NetworkType":"VPC-CNI"}`
	clsInfo := &tke2018.Cluster{
		Property: &property,
	}

	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, nil, clsInfo)
	if it == nil {
		t.Fatal("expected non-nil instance type")
	}
	// Default pods = 110
	podsQty := it.Capacity[corev1.ResourcePods]
	if podsQty.Value() != 110 {
		t.Errorf("expected 110 pods for VPC-CNI (no maxPods override), got %d", podsQty.Value())
	}
}

func TestComputeCapacity_WithENILimits(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		CPU:          4,
		Memory:       8,
	}
	eniLimits := []*tke2018.PodLimitsInstance{
		{
			InstanceType: lo.ToPtr("S5.LARGE8"),
			PodLimits: &tke2018.PodLimitsByType{
				TKERouteENIStaticIP: lo.ToPtr(int64(50)),
				TKEDirectENI:        lo.ToPtr(int64(10)),
				TKESubENI:           lo.ToPtr(int64(5)),
			},
		},
	}
	result := computeCapacity(ctx, 50, inst, nil, nil, eniLimits)
	// Should have EIP resource when eniLimits is non-empty
	if _, ok := result[corev1.ResourceName(api.TKELabelEIP)]; !ok {
		t.Error("expected EIP in capacity when eniLimits provided")
	}
	if _, ok := result[corev1.ResourceName(api.TKELabelENIIP)]; !ok {
		t.Error("expected ENIIP in capacity")
	}
	if _, ok := result[corev1.ResourceName(api.TKELabelDirectENI)]; !ok {
		t.Error("expected DirectENI in capacity")
	}
	if _, ok := result[corev1.ResourceName(api.TKELabelSubENI)]; !ok {
		t.Error("expected SubENI in capacity")
	}
}

func TestEip_AllCPURanges(t *testing.T) {
	ctx := testCtx()
	tests := []struct {
		cpu      int
		expected int64
	}{
		{1, 1},
		{5, 1},
		{6, 2},
		{11, 2},
		{12, 3},
		{17, 3},
		{18, 4},
		{23, 4},
		{24, 5},
		{29, 5},
		{30, 6},
		{35, 6},
		{36, 7},
		{41, 7},
		{42, 8},
		{47, 8},
		{48, 9},
		{96, 9},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("cpu_%d", tt.cpu), func(t *testing.T) {
			inst := cxm.InstanceTypeQuotaItem{CPU: tt.cpu}
			result := eip(ctx, inst)
			if result.Value() != tt.expected {
				t.Errorf("CPU=%d: expected %d eip, got %d", tt.cpu, tt.expected, result.Value())
			}
		})
	}
}

func TestEip_ZeroCPU(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{CPU: 0}
	result := eip(ctx, inst)
	// CPU=0 falls into default branch, returns 1
	if result.Value() != 1 {
		t.Errorf("expected 1 eip for 0 CPU (default), got %d", result.Value())
	}
}

func TestNewInstanceType_ClusterPropertyNilENILimits(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")

	// ClusterProperty: VpcCniType is empty, NetworkType is not "VPC-CNI", MetaFeatureParam is nil
	// This should cause eniLimits to be set to nil inside NewInstanceType.
	property := `{"NetworkType":"GR","VpcCniType":"","MetaFeatureParam":null}`
	clsInfo := &tke2018.Cluster{
		Property:               &property,
		ClusterNetworkSettings: nil, // nil so we don't enter the maxPods branch
	}

	// Pass eniLimits with a matching entry; the branch should clear them to nil.
	eniLimits := []*tke2018.PodLimitsInstance{
		{
			InstanceType: lo.ToPtr("S5.LARGE8"),
			PodLimits: &tke2018.PodLimitsByType{
				TKERouteENIStaticIP: lo.ToPtr(int64(50)),
				TKEDirectENI:        lo.ToPtr(int64(10)),
				TKESubENI:           lo.ToPtr(int64(5)),
			},
		},
	}

	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, eniLimits, clsInfo)
	if it == nil {
		t.Fatal("expected non-nil instance type")
	}

	// Because eniLimits was cleared to nil, ENI-related resources should NOT appear in capacity.
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelEIP)]; ok {
		t.Error("expected no EIP in capacity when eniLimits is cleared to nil")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelENIIP)]; ok {
		t.Error("expected no ENIIP in capacity when eniLimits is cleared to nil")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelDirectENI)]; ok {
		t.Error("expected no DirectENI in capacity when eniLimits is cleared to nil")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelSubENI)]; ok {
		t.Error("expected no SubENI in capacity when eniLimits is cleared to nil")
	}
}

func TestNewInstanceType_ClusterInfoNilProperty(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")

	// clsInfo with nil Property - should not panic, the body of the if is skipped.
	clsInfo := &tke2018.Cluster{
		Property: nil,
	}

	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, nil, clsInfo)
	if it == nil {
		t.Fatal("expected non-nil instance type when clsInfo.Property is nil")
	}
	if it.Name != "S5.LARGE8" {
		t.Errorf("expected name S5.LARGE8, got %s", it.Name)
	}
	if len(it.Capacity) == 0 {
		t.Error("expected non-empty capacity")
	}
}

func TestNewInstanceType_WithENILimitsAndVPCCNI(t *testing.T) {
	ctx := testCtx()
	inst := cxm.InstanceTypeQuotaItem{
		InstanceType:   "S5.LARGE8",
		CPU:            4,
		Memory:         8,
		InstanceFamily: "S5",
		Arch:           "amd64",
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "100003"),
				scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, "ap-guangzhou-3"),
			),
			Available: true,
		},
	}
	version := semver.MustParse("1.30.0")

	// NetworkType is "VPC-CNI", so eniLimits should NOT be cleared to nil.
	property := `{"NetworkType":"VPC-CNI"}`
	clsInfo := &tke2018.Cluster{
		Property: &property,
	}

	eniLimits := []*tke2018.PodLimitsInstance{
		{
			InstanceType: lo.ToPtr("S5.LARGE8"),
			PodLimits: &tke2018.PodLimitsByType{
				TKERouteENIStaticIP: lo.ToPtr(int64(50)),
				TKEDirectENI:        lo.ToPtr(int64(10)),
				TKESubENI:           lo.ToPtr(int64(5)),
			},
		},
	}

	it := NewInstanceType(ctx, "ap-guangzhou", 50, inst, version,
		nil, nil, nil, nil, nil, offerings, eniLimits, clsInfo)
	if it == nil {
		t.Fatal("expected non-nil instance type")
	}

	// ENI-related resources should appear in capacity because eniLimits is preserved.
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelEIP)]; !ok {
		t.Error("expected EIP in capacity for VPC-CNI cluster with eniLimits")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelENIIP)]; !ok {
		t.Error("expected ENIIP in capacity for VPC-CNI cluster with eniLimits")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelDirectENI)]; !ok {
		t.Error("expected DirectENI in capacity for VPC-CNI cluster with eniLimits")
	}
	if _, ok := it.Capacity[corev1.ResourceName(api.TKELabelSubENI)]; !ok {
		t.Error("expected SubENI in capacity for VPC-CNI cluster with eniLimits")
	}
}
