package instancetype

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/cxm"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tke2018 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tke/v20180525"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// ---------------------------------------------------------------------------
// Helpers for ZoneNotSupported tests
// ---------------------------------------------------------------------------

// mockRoundTripper lets tests intercept every HTTP call made by the SDK clients
// and return a canned response or error.
type mockRoundTripper struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

// zoneNotSupportedBody returns a Tencent Cloud error JSON body whose Code
// contains "ZoneNotSupported".
func zoneNotSupportedBody() string {
	return `{"Response":{"Error":{"Code":"ZoneNotSupported","Message":"zone not supported"},"RequestId":"fake-request-id"}}`
}

// successInstanceTypesBody returns a minimal valid DescribeZoneInstanceConfigInfos
// response that contains one InstanceTypeQuotaItem.
func successInstanceTypesBody() string {
	return `{"Response":{"RequestId":"ok-request-id","InstanceTypeQuotaSet":"[{\"InstanceType\":\"S5.LARGE8\",\"Zone\":\"ap-guangzhou-3\",\"CPU\":4,\"Memory\":8,\"Status\":\"SELL\",\"Inventory\":100}]"}}`
}

// successVpcCniBody returns a minimal valid DescribeVpcCniPodLimits response.
func successVpcCniBody() string {
	return `{"Response":{"RequestId":"ok-request-id","PodLimitsInstanceSet":[]}}`
}

// makeHTTPResponse wraps a string body into an *http.Response with the given status code.
func makeHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// newCommonClientWithTransport creates a *common.Client that uses the provided
// http.RoundTripper instead of making real network calls.
func newCommonClientWithTransport(transport http.RoundTripper) *common.Client {
	cred := common.NewCredential("test-secret-id", "test-secret-key")
	pf := profile.NewClientProfile()
	pf.HttpProfile.Endpoint = "tke.tencentcloudapi.com"
	c := common.NewCommonClient(cred, "ap-guangzhou", pf)
	c.WithHttpTransport(transport)
	return c
}

// newTKE2018ClientWithTransport creates a *tke2018.Client that uses the provided
// http.RoundTripper instead of making real network calls.
func newTKE2018ClientWithTransport(transport http.RoundTripper) *tke2018.Client {
	cred := common.NewCredential("test-secret-id", "test-secret-key")
	pf := profile.NewClientProfile()
	pf.HttpProfile.Endpoint = "tke.tencentcloudapi.com"
	c, _ := tke2018.NewClient(cred, "ap-guangzhou", pf)
	c.WithHttpTransport(transport)
	return c
}

// itFakeClient is a minimal client.Client that returns an empty NodeClaimList
// for List calls, which is the only operation getInstanceTypes calls on rtclient.
type itFakeClient struct{}

func (f *itFakeClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}
func (f *itFakeClient) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if ncl, ok := list.(*v1.NodeClaimList); ok {
		ncl.Items = []v1.NodeClaim{}
	}
	return nil
}
func (f *itFakeClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return nil
}
func (f *itFakeClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return nil
}
func (f *itFakeClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return nil
}
func (f *itFakeClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return nil
}
func (f *itFakeClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}
func (f *itFakeClient) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
	return nil
}
func (f *itFakeClient) Status() client.SubResourceWriter        { return &itFakeSubResource{} }
func (f *itFakeClient) SubResource(_ string) client.SubResourceClient { return &itFakeSubResource{} }
func (f *itFakeClient) Scheme() *runtime.Scheme                { return runtime.NewScheme() }
func (f *itFakeClient) RESTMapper() meta.RESTMapper             { return nil }
func (f *itFakeClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (f *itFakeClient) IsObjectNamespaced(_ runtime.Object) (bool, error) { return false, nil }

type itFakeSubResource struct{}

func (s *itFakeSubResource) Get(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceGetOption) error {
	return nil
}
func (s *itFakeSubResource) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return nil
}
func (s *itFakeSubResource) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return nil
}
func (s *itFakeSubResource) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return nil
}

// minimalNodeClass builds a TKEMachineNodeClass with one subnet in the given zone.
func minimalNodeClass(zone string) *api.TKEMachineNodeClass {
	return &api.TKEMachineNodeClass{
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{{Zone: zone}},
		},
	}
}

// ---------------------------------------------------------------------------
// Tests for getInstanceTypes ZoneNotSupported handling
// ---------------------------------------------------------------------------

// TestGetInstanceTypes_ZoneNotSupported_HTTPError tests the case where the SDK's
// Send() returns an error containing "ZoneNotSupported" (e.g. from an HTTP-level
// error response).  The function should return an empty slice and nil error so
// that the caller keeps processing other zones.
func TestGetInstanceTypes_ZoneNotSupported_HTTPError(t *testing.T) {
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			// Return HTTP 200 but with a body whose Error.Code is ZoneNotSupported.
			// ParseErrorFromHTTPResponse (called inside Send via parseFromJson) will
			// translate this into a *TencentCloudSDKError whose .Error() string
			// contains "ZoneNotSupported".
			return makeHTTPResponse(200, zoneNotSupportedBody()), nil
		},
	}

	p := newTestProvider()
	p.client = newCommonClientWithTransport(transport)
	p.rtclient = &itFakeClient{}

	ctx := context.Background()
	nodeClass := minimalNodeClass("ap-guangzhou-3")

	result, err := p.getInstanceTypes(ctx, "amd64", false, false, nodeClass)
	if err != nil {
		t.Fatalf("expected nil error when ZoneNotSupported, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice when ZoneNotSupported, got %d items", len(result))
	}
}

// TestGetInstanceTypes_ZoneNotSupported_TransportError tests that when the
// http.RoundTripper itself returns an error whose message contains
// "ZoneNotSupported", getInstanceTypes returns an empty slice with nil error.
func TestGetInstanceTypes_ZoneNotSupported_TransportError(t *testing.T) {
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("request failed: ZoneNotSupported for this zone")
		},
	}

	p := newTestProvider()
	p.client = newCommonClientWithTransport(transport)
	p.rtclient = &itFakeClient{}

	ctx := context.Background()
	nodeClass := minimalNodeClass("ap-guangzhou-3")

	result, err := p.getInstanceTypes(ctx, "amd64", false, false, nodeClass)
	if err != nil {
		t.Fatalf("expected nil error when ZoneNotSupported, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice when ZoneNotSupported, got %d items", len(result))
	}
}

// TestGetInstanceTypes_OtherError tests that non-ZoneNotSupported errors are
// propagated as real errors.
func TestGetInstanceTypes_OtherError(t *testing.T) {
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	p := newTestProvider()
	p.client = newCommonClientWithTransport(transport)
	p.rtclient = &itFakeClient{}

	ctx := context.Background()
	nodeClass := minimalNodeClass("ap-guangzhou-3")

	_, err := p.getInstanceTypes(ctx, "amd64", false, false, nodeClass)
	if err == nil {
		t.Fatal("expected non-nil error for non-ZoneNotSupported failure")
	}
}

// ---------------------------------------------------------------------------
// Tests for getENILimits ZoneNotSupported handling
// ---------------------------------------------------------------------------

// TestGetENILimits_ZoneNotSupported_SkipsZone verifies that when a zone returns
// ZoneNotSupported, that zone is absent from the result map but the method still
// succeeds (nil error) so other zones can be processed.
func TestGetENILimits_ZoneNotSupported_SkipsZone(t *testing.T) {
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			// Always return ZoneNotSupported.
			return makeHTTPResponse(200, zoneNotSupportedBody()), nil
		},
	}

	p := newTestProvider()
	p.client2018 = newTKE2018ClientWithTransport(transport)

	ctx := context.Background()
	nodeClass := &api.TKEMachineNodeClass{
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{
				{Zone: "ap-guangzhou-3"},
				{Zone: "ap-guangzhou-4"},
			},
		},
	}

	limits, err := p.getENILimits(ctx, nodeClass)
	if err != nil {
		t.Fatalf("expected nil error when zones return ZoneNotSupported, got: %v", err)
	}
	if _, ok := limits["ap-guangzhou-3"]; ok {
		t.Error("ap-guangzhou-3 should be absent from limits map (ZoneNotSupported)")
	}
	if _, ok := limits["ap-guangzhou-4"]; ok {
		t.Error("ap-guangzhou-4 should be absent from limits map (ZoneNotSupported)")
	}
}

// TestGetENILimits_ZoneNotSupported_PartialSuccess verifies that when one zone
// succeeds and another returns ZoneNotSupported, only the successful zone appears
// in the result map.
func TestGetENILimits_ZoneNotSupported_PartialSuccess(t *testing.T) {
	callCount := 0
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount == 1 {
				// First call (ap-guangzhou-3): zone not supported.
				return makeHTTPResponse(200, zoneNotSupportedBody()), nil
			}
			// Second call (ap-guangzhou-4): success.
			return makeHTTPResponse(200, successVpcCniBody()), nil
		},
	}

	p := newTestProvider()
	p.client2018 = newTKE2018ClientWithTransport(transport)

	ctx := context.Background()
	nodeClass := &api.TKEMachineNodeClass{
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{
				{Zone: "ap-guangzhou-3"},
				{Zone: "ap-guangzhou-4"},
			},
		},
	}

	limits, err := p.getENILimits(ctx, nodeClass)
	if err != nil {
		t.Fatalf("expected nil error for partial success, got: %v", err)
	}
	if _, ok := limits["ap-guangzhou-3"]; ok {
		t.Error("ap-guangzhou-3 should be absent from limits map (ZoneNotSupported)")
	}
	if _, ok := limits["ap-guangzhou-4"]; !ok {
		t.Error("ap-guangzhou-4 should be present in limits map (success)")
	}
}

// TestGetENILimits_OtherError verifies that non-ZoneNotSupported errors from
// DescribeVpcCniPodLimits are propagated as real errors.
func TestGetENILimits_OtherError(t *testing.T) {
	transport := &mockRoundTripper{
		fn: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("internal server error")
		},
	}

	p := newTestProvider()
	p.client2018 = newTKE2018ClientWithTransport(transport)

	ctx := context.Background()
	nodeClass := minimalNodeClass("ap-guangzhou-3")

	_, err := p.getENILimits(ctx, nodeClass)
	if err == nil {
		t.Fatal("expected non-nil error for non-ZoneNotSupported failure in getENILimits")
	}
}
