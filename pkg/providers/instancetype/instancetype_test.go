package instancetype

import (
	"context"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
	corev1 "k8s.io/api/core/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestNormalizeVersion_WithPrefix(t *testing.T) {
	got := normalizeVersion("v1.30.0")
	if got != "1.30.0" {
		t.Errorf("expected 1.30.0, got %s", got)
	}
}

func TestNormalizeVersion_WithoutPrefix(t *testing.T) {
	got := normalizeVersion("1.30.0")
	if got != "1.30.0" {
		t.Errorf("expected 1.30.0, got %s", got)
	}
}

func TestNormalizeVersion_Empty(t *testing.T) {
	got := normalizeVersion("")
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func newTestProvider() *DefaultProvider {
	return &DefaultProvider{
		region:         "ap-guangzhou",
		blacklistCache: cache.New(5*time.Minute, 10*time.Minute),
		providerCache:  cache.New(5*time.Minute, 10*time.Minute),
	}
}

func TestIsBlocked_NotBlocked(t *testing.T) {
	p := newTestProvider()
	if p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected not blocked")
	}
}

func TestIsBlocked_ExactMatch(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-S5.LARGE8-on-demand-ap-guangzhou-3", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for exact match")
	}
}

func TestIsBlocked_AllWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-*-*-*", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for all-wildcard")
	}
}

func TestIsBlocked_InstanceCapacityWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-S5.LARGE8-on-demand-*", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for instance+capacity with zone wildcard")
	}
}

func TestIsBlocked_InstanceZoneWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-S5.LARGE8-*-ap-guangzhou-3", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for instance+zone with capacity wildcard")
	}
}

func TestIsBlocked_InstanceAllWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-S5.LARGE8-*-*", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for instance with both wildcards")
	}
}

func TestIsBlocked_CapacityZoneWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-*-on-demand-ap-guangzhou-3", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for capacity+zone with instance wildcard")
	}
}

func TestIsBlocked_CapacityAllWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-*-on-demand-*", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for capacity with instance+zone wildcards")
	}
}

func TestIsBlocked_ZoneAllWildcard(t *testing.T) {
	p := newTestProvider()
	p.blacklistCache.Set("blocked-ins-*-*-ap-guangzhou-3", true, cache.DefaultExpiration)
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected blocked for zone with instance+capacity wildcards")
	}
}

func TestBlockInstanceType(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()
	p.BlockInstanceType(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3", "test block")
	if !p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected instance to be blocked after BlockInstanceType")
	}
}

func TestGetInsufficientFailureCount_NotExists(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()
	count := p.GetInsufficientFailureCount(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	if count != 0 {
		t.Errorf("expected 0 for non-existent failure count, got %d", count)
	}
}

func TestGetInsufficientFailureCount_Exists(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()
	p.blacklistCache.Set("failure-ins-S5.LARGE8-on-demand-ap-guangzhou-3", 5, cache.DefaultExpiration)
	count := p.GetInsufficientFailureCount(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestAddInsufficientFailure_FirstTime(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()
	p.AddInsufficientFailure(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	count := p.GetInsufficientFailureCount(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	if count != 1 {
		t.Errorf("expected 1 after first failure, got %d", count)
	}
}

func TestAddInsufficientFailure_Accumulate(t *testing.T) {
	p := newTestProvider()
	ctx := context.Background()
	p.AddInsufficientFailure(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	p.AddInsufficientFailure(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	p.AddInsufficientFailure(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	count := p.GetInsufficientFailureCount(ctx, "S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3")
	if count != 3 {
		t.Errorf("expected 3 after three failures, got %d", count)
	}
}

type mockZoneProviderIT struct{}

func (m *mockZoneProviderIT) ZoneFromID(id string) (string, error) { return "ap-guangzhou-3", nil }
func (m *mockZoneProviderIT) IDFromZone(zone string) (string, error) {
	return "100003", nil
}

func TestCreateOfferings_SELL(t *testing.T) {
	p := newTestProvider()
	p.zoneProvider = &mockZoneProviderIT{}
	ctx := context.Background()
	insType := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		Zone:         "ap-guangzhou-3",
		Status:       "SELL",
		Inventory:    100,
		Price:        cxm.ItemPrice{UnitPrice: 1.5},
	}
	offerings := p.createOfferings(ctx, v1.CapacityTypeOnDemand, insType)
	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	if !offerings[0].Available {
		t.Error("expected offering to be available for SELL status with inventory > 0")
	}
	if offerings[0].Price != 1.5 {
		t.Errorf("expected price 1.5, got %f", offerings[0].Price)
	}
	// Check requirements
	capReq := offerings[0].Requirements.Get(v1.CapacityTypeLabelKey)
	if !capReq.Has(v1.CapacityTypeOnDemand) {
		t.Error("expected on-demand capacity type in offering")
	}
	zoneReq := offerings[0].Requirements.Get(corev1.LabelTopologyZone)
	if !zoneReq.Has("100003") {
		t.Error("expected zone ID 100003 in offering")
	}
	cbsReq := offerings[0].Requirements.Get(api.LabelCBSToplogy)
	if !cbsReq.Has("ap-guangzhou-3") {
		t.Error("expected CBS topology ap-guangzhou-3 in offering")
	}
}

func TestCreateOfferings_NotSELL(t *testing.T) {
	p := newTestProvider()
	p.zoneProvider = &mockZoneProviderIT{}
	ctx := context.Background()
	insType := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		Zone:         "ap-guangzhou-3",
		Status:       "SOLD_OUT",
		Inventory:    0,
		Price:        cxm.ItemPrice{UnitPrice: 1.5},
	}
	offerings := p.createOfferings(ctx, v1.CapacityTypeOnDemand, insType)
	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	if offerings[0].Available {
		t.Error("expected offering to be unavailable for SOLD_OUT status")
	}
}

func TestCreateOfferings_SELLButZeroInventory(t *testing.T) {
	p := newTestProvider()
	p.zoneProvider = &mockZoneProviderIT{}
	ctx := context.Background()
	insType := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		Zone:         "ap-guangzhou-3",
		Status:       "SELL",
		Inventory:    0,
	}
	offerings := p.createOfferings(ctx, v1.CapacityTypeSpot, insType)
	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	if offerings[0].Available {
		t.Error("expected offering to be unavailable when SELL but inventory = 0")
	}
	capReq := offerings[0].Requirements.Get(v1.CapacityTypeLabelKey)
	if !capReq.Has(v1.CapacityTypeSpot) {
		t.Error("expected spot capacity type")
	}
}

func TestNewDefaultProvider(t *testing.T) {
	c := cache.New(5*time.Minute, 10*time.Minute)
	bc := cache.New(5*time.Minute, 10*time.Minute)
	p := NewDefaultProvider(context.Background(), "ap-guangzhou", nil, nil, &mockZoneProviderIT{}, nil, nil, c, bc)
	if p.region != "ap-guangzhou" {
		t.Errorf("expected region ap-guangzhou, got %s", p.region)
	}
}

func TestIsBlocked_NonMatchingWildcard(t *testing.T) {
	p := newTestProvider()
	// Block a different instance type
	p.blacklistCache.Set("blocked-ins-S6.LARGE16-on-demand-ap-guangzhou-3", true, cache.DefaultExpiration)
	if p.isBlocked("S5.LARGE8", v1.CapacityTypeOnDemand, "ap-guangzhou-3") {
		t.Error("expected not blocked for different instance type")
	}
}

func TestCreateOfferings_SpotCapacityType(t *testing.T) {
	p := newTestProvider()
	p.zoneProvider = &mockZoneProviderIT{}
	ctx := context.Background()
	insType := cxm.InstanceTypeQuotaItem{
		InstanceType: "S5.LARGE8",
		Zone:         "ap-guangzhou-3",
		Status:       "SELL",
		Inventory:    50,
	}
	offerings := p.createOfferings(ctx, v1.CapacityTypeSpot, insType)
	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	capReq := offerings[0].Requirements.Get(v1.CapacityTypeLabelKey)
	if !capReq.Has(v1.CapacityTypeSpot) {
		t.Error("expected spot capacity type in offering")
	}
}
