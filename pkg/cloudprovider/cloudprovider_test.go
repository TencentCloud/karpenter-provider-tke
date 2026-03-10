package cloudprovider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/awslabs/operatorpkg/status"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/operator/options"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

func TestName(t *testing.T) {
	cp := &CloudProvider{}
	if cp.Name() != "tke" {
		t.Errorf("expected 'tke', got %q", cp.Name())
	}
}

func TestGetSupportedNodeClasses(t *testing.T) {
	cp := &CloudProvider{}
	classes := cp.GetSupportedNodeClasses()
	if len(classes) != 1 {
		t.Fatalf("expected 1 supported node class, got %d", len(classes))
	}
	_, ok := classes[0].(*api.TKEMachineNodeClass)
	if !ok {
		t.Error("expected TKEMachineNodeClass type")
	}
}

func TestRepairPolicies(t *testing.T) {
	cp := &CloudProvider{}
	policies := cp.RepairPolicies()
	if len(policies) != 2 {
		t.Fatalf("expected 2 repair policies, got %d", len(policies))
	}
	// First policy: NodeReady = False
	if policies[0].ConditionType != corev1.NodeReady {
		t.Errorf("expected NodeReady condition, got %s", policies[0].ConditionType)
	}
	if policies[0].ConditionStatus != corev1.ConditionFalse {
		t.Errorf("expected ConditionFalse, got %s", policies[0].ConditionStatus)
	}
	if policies[0].TolerationDuration != 30*time.Minute {
		t.Errorf("expected 30m toleration, got %v", policies[0].TolerationDuration)
	}
	// Second policy: NodeReady = Unknown
	if policies[1].ConditionType != corev1.NodeReady {
		t.Errorf("expected NodeReady condition, got %s", policies[1].ConditionType)
	}
	if policies[1].ConditionStatus != corev1.ConditionUnknown {
		t.Errorf("expected ConditionUnknown, got %s", policies[1].ConditionStatus)
	}
	if policies[1].TolerationDuration != 30*time.Minute {
		t.Errorf("expected 30m toleration, got %v", policies[1].TolerationDuration)
	}
}

func TestIsDrifted(t *testing.T) {
	cp := &CloudProvider{}
	reason, err := cp.IsDrifted(context.Background(), nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if reason != "" {
		t.Errorf("expected empty drift reason, got %q", string(reason))
	}
}

func TestResourceListFromAnnotations_Nil(t *testing.T) {
	result := resourceListFromAnnotations("capacity.", nil)
	if len(result) != 0 {
		t.Errorf("expected empty resource list for nil annotations, got %d", len(result))
	}
}

func TestResourceListFromAnnotations_Empty(t *testing.T) {
	result := resourceListFromAnnotations("capacity.", map[string]string{})
	if len(result) != 0 {
		t.Errorf("expected empty resource list for empty annotations, got %d", len(result))
	}
}

func TestResourceListFromAnnotations_CPU(t *testing.T) {
	annotations := map[string]string{
		api.CapacityGroup + api.AnnotationCPU: "4",
	}
	result := resourceListFromAnnotations(api.CapacityGroup, annotations)
	cpu, ok := result[corev1.ResourceCPU]
	if !ok {
		t.Fatal("expected CPU in resource list")
	}
	if cpu.Value() != 4 {
		t.Errorf("expected 4 CPU, got %d", cpu.Value())
	}
}

func TestResourceListFromAnnotations_Memory(t *testing.T) {
	annotations := map[string]string{
		api.CapacityGroup + api.AnnotationMemory: "8Gi",
	}
	result := resourceListFromAnnotations(api.CapacityGroup, annotations)
	mem, ok := result[corev1.ResourceMemory]
	if !ok {
		t.Fatal("expected Memory in resource list")
	}
	expected := "8Gi"
	if mem.String() != expected {
		t.Errorf("expected %s, got %s", expected, mem.String())
	}
}

func TestResourceListFromAnnotations_AllResources(t *testing.T) {
	annotations := map[string]string{
		api.CapacityGroup + api.AnnotationCPU:              "4",
		api.CapacityGroup + api.AnnotationMemory:           "8Gi",
		api.CapacityGroup + api.AnnotationPods:             "110",
		api.CapacityGroup + api.AnnotationEphemeralStorage: "50G",
		api.CapacityGroup + api.AnnotationENIIP:            "10",
		api.CapacityGroup + api.AnnotationDirectENI:        "5",
		api.CapacityGroup + api.AnnotationENI:              "3",
		api.CapacityGroup + api.AnnotationSubENI:           "2",
		api.CapacityGroup + api.AnnotationEIP:              "1",
		api.CapacityGroup + api.AnnotationGPUCount:         "2",
	}
	result := resourceListFromAnnotations(api.CapacityGroup, annotations)
	if len(result) != 10 {
		t.Errorf("expected 10 resources, got %d", len(result))
	}
}

func TestResourceListFromAnnotations_DifferentGroups(t *testing.T) {
	annotations := map[string]string{
		api.KubeReservedGroup + api.AnnotationCPU:    "100m",
		api.SystemReservedGroup + api.AnnotationCPU:  "50m",
		api.CapacityGroup + api.AnnotationCPU:        "4",
	}
	// Only extract for KubeReservedGroup
	result := resourceListFromAnnotations(api.KubeReservedGroup, annotations)
	cpu, ok := result[corev1.ResourceCPU]
	if !ok {
		t.Fatal("expected CPU in resource list for KubeReservedGroup")
	}
	if cpu.String() != "100m" {
		t.Errorf("expected 100m, got %s", cpu.String())
	}
	// Should only have 1 entry
	if len(result) != 1 {
		t.Errorf("expected 1 resource, got %d", len(result))
	}
}

func TestNewTerminatingNodeClassError(t *testing.T) {
	err := newTerminatingNodeClassError("test-class")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.IsNotFound(err) {
		t.Error("expected NotFound error")
	}
	if err.ErrStatus.Message == "" {
		t.Error("expected non-empty error message")
	}
	expectedContains := "is terminating"
	msg := err.ErrStatus.Message
	if !containsStr(msg, expectedContains) {
		t.Errorf("expected message to contain %q, got %q", expectedContains, msg)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

// mockMachineProvider implements machine.Provider with configurable func fields.
type mockMachineProvider struct {
	GetFn    func(context.Context, string) (*capiv1beta1.Machine, error)
	ListFn   func(context.Context) ([]*capiv1beta1.Machine, error)
	CreateFn func(context.Context, *api.TKEMachineNodeClass, *v1.NodeClaim, []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error)
	DeleteFn func(context.Context, *v1.NodeClaim) error
}

func (m *mockMachineProvider) Get(ctx context.Context, id string) (*capiv1beta1.Machine, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, fmt.Errorf("Get not implemented")
}

func (m *mockMachineProvider) List(ctx context.Context) ([]*capiv1beta1.Machine, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx)
	}
	return nil, fmt.Errorf("List not implemented")
}

func (m *mockMachineProvider) Create(ctx context.Context, nc *api.TKEMachineNodeClass, claim *v1.NodeClaim, its []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, nc, claim, its)
	}
	return nil, nil, fmt.Errorf("Create not implemented")
}

func (m *mockMachineProvider) Delete(ctx context.Context, claim *v1.NodeClaim) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, claim)
	}
	return fmt.Errorf("Delete not implemented")
}

// mockInstanceTypeProvider implements instancetype.Provider with configurable func fields.
type mockInstanceTypeProvider struct {
	ListFn                       func(context.Context, *api.TKEMachineNodeClass, bool) ([]*cloudprovider.InstanceType, error)
	BlockInstanceTypeFn          func(ctx context.Context, instName, capacityType, zone, message string)
	GetInsufficientFailureCountFn func(ctx context.Context, instName, capacityType, zone string) int
	AddInsufficientFailureFn     func(ctx context.Context, instName, capacityType, zone string)
}

func (m *mockInstanceTypeProvider) List(ctx context.Context, nc *api.TKEMachineNodeClass, refresh bool) ([]*cloudprovider.InstanceType, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, nc, refresh)
	}
	return nil, fmt.Errorf("List not implemented")
}

func (m *mockInstanceTypeProvider) BlockInstanceType(ctx context.Context, instName, capacityType, zone, message string) {
	if m.BlockInstanceTypeFn != nil {
		m.BlockInstanceTypeFn(ctx, instName, capacityType, zone, message)
	}
}

func (m *mockInstanceTypeProvider) GetInsufficientFailureCount(ctx context.Context, instName, capacityType, zone string) int {
	if m.GetInsufficientFailureCountFn != nil {
		return m.GetInsufficientFailureCountFn(ctx, instName, capacityType, zone)
	}
	return 0
}

func (m *mockInstanceTypeProvider) AddInsufficientFailure(ctx context.Context, instName, capacityType, zone string) {
	if m.AddInsufficientFailureFn != nil {
		m.AddInsufficientFailureFn(ctx, instName, capacityType, zone)
	}
}

// mockZoneProvider implements zone.Provider with configurable func fields.
type mockZoneProvider struct {
	ZoneFromIDFn func(string) (string, error)
	IDFromZoneFn func(string) (string, error)
}

func (m *mockZoneProvider) ZoneFromID(id string) (string, error) {
	if m.ZoneFromIDFn != nil {
		return m.ZoneFromIDFn(id)
	}
	return "", fmt.Errorf("ZoneFromID not implemented")
}

func (m *mockZoneProvider) IDFromZone(zone string) (string, error) {
	if m.IDFromZoneFn != nil {
		return m.IDFromZoneFn(zone)
	}
	return "", fmt.Errorf("IDFromZone not implemented")
}

// cpFakeClient is a minimal client.Client implementation for testing.
type cpFakeClient struct {
	objects       map[string]client.Object
	nodeClaimList *v1.NodeClaimList
	getErr        error
	listErr       error
}

func newCPFakeClient() *cpFakeClient {
	return &cpFakeClient{
		objects:       make(map[string]client.Object),
		nodeClaimList: &v1.NodeClaimList{},
	}
}

func (f *cpFakeClient) key(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

func (f *cpFakeClient) Get(ctx context.Context, objKey client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getErr != nil {
		return f.getErr
	}
	stored, ok := f.objects[f.key(objKey.Namespace, objKey.Name)]
	if !ok {
		return errors.NewNotFound(schema.GroupResource{Group: api.Group, Resource: "tkemachinenodeclasses"}, objKey.Name)
	}
	// Copy the stored object into the output by type-asserting
	switch out := obj.(type) {
	case *api.TKEMachineNodeClass:
		in, ok := stored.(*api.TKEMachineNodeClass)
		if !ok {
			return fmt.Errorf("stored object is not TKEMachineNodeClass")
		}
		in.DeepCopyInto(out)
	default:
		return fmt.Errorf("unsupported object type in cpFakeClient.Get: %T", obj)
	}
	return nil
}

func (f *cpFakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	switch out := list.(type) {
	case *v1.NodeClaimList:
		f.nodeClaimList.DeepCopyInto(out)
	default:
		return fmt.Errorf("unsupported list type in cpFakeClient.List: %T", list)
	}
	return nil
}

// Stubs for the rest of client.Client interface
func (f *cpFakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return nil
}
func (f *cpFakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}
func (f *cpFakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}
func (f *cpFakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}
func (f *cpFakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}
func (f *cpFakeClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return nil
}

type fakeStatusWriter struct{}

func (w *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}
func (w *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}
func (w *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (f *cpFakeClient) Status() client.SubResourceWriter {
	return &fakeStatusWriter{}
}

type fakeSubResourceClient struct{}

func (s *fakeSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}
func (s *fakeSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}
func (s *fakeSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}
func (s *fakeSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (f *cpFakeClient) SubResource(subResource string) client.SubResourceClient {
	return &fakeSubResourceClient{}
}
func (f *cpFakeClient) Scheme() *runtime.Scheme     { return runtime.NewScheme() }
func (f *cpFakeClient) RESTMapper() meta.RESTMapper  { return nil }
func (f *cpFakeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (f *cpFakeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return false, nil
}

// ---------------------------------------------------------------------------
// Helper: build a context with options (needed by instancetype.NewInstanceType
// which calls options.FromContext for VMMemoryOverheadPercent)
// ---------------------------------------------------------------------------

func testCtx() context.Context {
	ctx := context.Background()
	ctx = options.ToContext(ctx, &options.Options{
		VMMemoryOverheadPercent: 0.075,
		ClusterID:              "cls-test",
		Region:                 "ap-guangzhou",
	})
	return ctx
}

// ---------------------------------------------------------------------------
// Helper: build a ready TKEMachineNodeClass
// ---------------------------------------------------------------------------

func readyNodeClass(name string) *api.TKEMachineNodeClass {
	nc := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{
				{ID: "subnet-abc", Zone: "ap-guangzhou-3", ZoneID: "100003"},
			},
			SecurityGroups: []api.SecurityGroup{{ID: "sg-001"}},
			SSHKeys:        []api.SSHKey{{ID: "skey-001"}},
		},
	}
	nc.StatusConditions().SetTrue(status.ConditionReady)
	return nc
}

// ---------------------------------------------------------------------------
// Helper: build a valid Machine with ProviderSpec and labels/annotations
// that machineToNodeClaim can successfully parse.
// ---------------------------------------------------------------------------

func validMachine(name string, providerID string, zone string) *capiv1beta1.Machine {
	providerSpec := &capiv1beta1.CXMMachineProviderSpec{
		InstanceType: "S5.MEDIUM4",
	}
	rawExt, _ := capiv1beta1.RawExtensionFromProviderSpec(providerSpec)
	kubeletVersion := "1.28.0"
	mc := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				api.LabelInstanceCPU:      "2",
				api.LabelInstanceMemoryGB: "4",
				api.LabelInstanceFamily:   "S5",
				v1.CapacityTypeLabelKey:   v1.CapacityTypeOnDemand,
				corev1.LabelArchStable:    "amd64",
			},
			Annotations: map[string]string{
				api.AnnotationUnitPrice:                            "0.5",
				api.CapacityGroup + api.AnnotationCPU:              "2",
				api.CapacityGroup + api.AnnotationMemory:           "4Gi",
				api.CapacityGroup + api.AnnotationPods:             "110",
				api.CapacityGroup + api.AnnotationEphemeralStorage: "50G",
				api.AnnotationManagedBy:                            "cls-test",
			},
		},
		Spec: capiv1beta1.MachineSpec{
			ProviderID:     &providerID,
			KubeletVersion: &kubeletVersion,
			Zone:           zone,
			ProviderSpec: capiv1beta1.ProviderSpec{
				Value: rawExt,
			},
		},
	}
	return mc
}

// ---------------------------------------------------------------------------
// Helper: build a simple instance type with one offering
// ---------------------------------------------------------------------------

func simpleInstanceType(name, zone, zoneID, capacityType string) *cloudprovider.InstanceType {
	offering := &cloudprovider.Offering{
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zoneID),
			scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, zone),
		),
		Price:     0.5,
		Available: true,
	}
	return &cloudprovider.InstanceType{
		Name: name,
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, name),
			scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
			scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, "linux"),
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zoneID),
			scheduling.NewRequirement(api.LabelCBSToplogy, corev1.NodeSelectorOpIn, zone),
			scheduling.NewRequirement(api.LabelInstanceCPU, corev1.NodeSelectorOpIn, "2"),
			scheduling.NewRequirement(api.LabelInstanceMemoryGB, corev1.NodeSelectorOpIn, "4"),
			scheduling.NewRequirement(api.LabelInstanceFamily, corev1.NodeSelectorOpIn, "S5"),
			scheduling.NewRequirement(corev1.LabelWindowsBuild, corev1.NodeSelectorOpDoesNotExist),
		),
		Offerings: cloudprovider.Offerings{offering},
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("2"),
			corev1.ResourceMemory:           resource.MustParse("4Gi"),
			corev1.ResourcePods:             resource.MustParse("110"),
			corev1.ResourceEphemeralStorage: resource.MustParse("50G"),
		},
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      corev1.ResourceList{},
			SystemReserved:    corev1.ResourceList{},
			EvictionThreshold: corev1.ResourceList{},
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewCloudProvider(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	mp := &mockMachineProvider{}
	ip := &mockInstanceTypeProvider{}
	zp := &mockZoneProvider{}
	cp := NewCloudProvider(ctx, fc, mp, ip, zp)
	if cp == nil {
		t.Fatal("expected non-nil CloudProvider")
	}
	if cp.kubeClient != fc {
		t.Error("expected kubeClient to match")
	}
	if cp.machineProvider != mp {
		t.Error("expected machineProvider to match")
	}
	if cp.instancetypeProvider != ip {
		t.Error("expected instancetypeProvider to match")
	}
	if cp.zoneProvider != zp {
		t.Error("expected zoneProvider to match")
	}
}

func TestCreate_NodeClassNotFound(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	// No nodeClass stored -> Get returns NotFound
	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      &mockMachineProvider{},
		instancetypeProvider: &mockInstanceTypeProvider{},
		zoneProvider:         &mockZoneProvider{},
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "nonexistent",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when nodeClass not found")
	}
	if !cloudprovider.IsInsufficientCapacityError(err) {
		t.Errorf("expected InsufficientCapacityError, got %T: %v", err, err)
	}
}

func TestCreate_NodeClassNotReady(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	// Store a nodeClass that is NOT ready (no conditions set)
	nc := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "not-ready-class"},
	}
	fc.objects["not-ready-class"] = nc

	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      &mockMachineProvider{},
		instancetypeProvider: &mockInstanceTypeProvider{},
		zoneProvider:         &mockZoneProvider{},
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "not-ready-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when nodeClass is not ready")
	}
	if containsStr(err.Error(), "resolving tkemachinenodeclass") == false {
		t.Errorf("expected error message about resolving tkemachinenodeclass, got %q", err.Error())
	}
}

func TestCreate_ResolveInstanceTypesError(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	nc := readyNodeClass("my-class")
	fc.objects["my-class"] = nc

	ip := &mockInstanceTypeProvider{
		ListFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ bool) ([]*cloudprovider.InstanceType, error) {
			return nil, fmt.Errorf("instance type listing failed")
		},
	}
	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      &mockMachineProvider{},
		instancetypeProvider: ip,
		zoneProvider:         &mockZoneProvider{},
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "my-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when instance type listing fails")
	}
	if !containsStr(err.Error(), "instance type listing failed") {
		t.Errorf("expected original error in message, got %q", err.Error())
	}
}

func TestCreate_NoInstanceTypes(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	nc := readyNodeClass("my-class")
	fc.objects["my-class"] = nc

	ip := &mockInstanceTypeProvider{
		ListFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ bool) ([]*cloudprovider.InstanceType, error) {
			// Return instance types, but none will be compatible with the nodeClaim requirements
			it := simpleInstanceType("S5.MEDIUM4", "ap-guangzhou-3", "100003", v1.CapacityTypeOnDemand)
			return []*cloudprovider.InstanceType{it}, nil
		},
	}
	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      &mockMachineProvider{},
		instancetypeProvider: ip,
		zoneProvider:         &mockZoneProvider{},
	}
	// NodeClaim with a requirement that no instance type can satisfy
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "my-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
			Requirements: []v1.NodeSelectorRequirementWithMinValues{
				{
					Key:      corev1.LabelArchStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"arm64"}, // No arm64 instance type available
				},
			},
		},
	}
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when no instance types match")
	}
	if !cloudprovider.IsInsufficientCapacityError(err) {
		t.Errorf("expected InsufficientCapacityError, got %T: %v", err, err)
	}
}

func TestCreate_MachineCreateError(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	nc := readyNodeClass("my-class")
	fc.objects["my-class"] = nc

	returnedMachine := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				v1.CapacityTypeLabelKey: v1.CapacityTypeOnDemand,
			},
		},
		Spec: capiv1beta1.MachineSpec{
			Zone: "ap-guangzhou-3",
		},
	}
	returnedSpec := &capiv1beta1.CXMMachineProviderSpec{
		InstanceType: "S5.MEDIUM4",
	}
	var blockedInstName, blockedCapType, blockedZone string
	ip := &mockInstanceTypeProvider{
		ListFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ bool) ([]*cloudprovider.InstanceType, error) {
			it := simpleInstanceType("S5.MEDIUM4", "ap-guangzhou-3", "100003", v1.CapacityTypeOnDemand)
			return []*cloudprovider.InstanceType{it}, nil
		},
		BlockInstanceTypeFn: func(_ context.Context, instName, capacityType, zone, message string) {
			blockedInstName = instName
			blockedCapType = capacityType
			blockedZone = zone
		},
	}
	mp := &mockMachineProvider{
		CreateFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ *v1.NodeClaim, _ []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error) {
			return returnedMachine, returnedSpec, fmt.Errorf("create machine failed")
		},
	}
	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      mp,
		instancetypeProvider: ip,
		zoneProvider:         &mockZoneProvider{},
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "my-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when machine creation fails")
	}
	if !containsStr(err.Error(), "create machine failed") {
		t.Errorf("expected original error, got %q", err.Error())
	}
	// Verify BlockInstanceType was called
	if blockedInstName != "S5.MEDIUM4" {
		t.Errorf("expected blocked instance type S5.MEDIUM4, got %q", blockedInstName)
	}
	if blockedCapType != v1.CapacityTypeOnDemand {
		t.Errorf("expected blocked capacity type on-demand, got %q", blockedCapType)
	}
	if blockedZone != "ap-guangzhou-3" {
		t.Errorf("expected blocked zone ap-guangzhou-3, got %q", blockedZone)
	}
}

func TestCreate_Success(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	nc := readyNodeClass("my-class")
	fc.objects["my-class"] = nc

	providerID := "qcloud:///100003/ins-abc123"
	createdMachine := validMachine("np-test-abc", providerID, "ap-guangzhou-3")

	ip := &mockInstanceTypeProvider{
		ListFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ bool) ([]*cloudprovider.InstanceType, error) {
			it := simpleInstanceType("S5.MEDIUM4", "ap-guangzhou-3", "100003", v1.CapacityTypeOnDemand)
			return []*cloudprovider.InstanceType{it}, nil
		},
	}
	mp := &mockMachineProvider{
		CreateFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ *v1.NodeClaim, _ []*cloudprovider.InstanceType) (*capiv1beta1.Machine, *capiv1beta1.CXMMachineProviderSpec, error) {
			spec := &capiv1beta1.CXMMachineProviderSpec{InstanceType: "S5.MEDIUM4"}
			return createdMachine, spec, nil
		},
	}
	zp := &mockZoneProvider{
		IDFromZoneFn: func(zone string) (string, error) {
			return "100003", nil
		},
	}
	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      mp,
		instancetypeProvider: ip,
		zoneProvider:         zp,
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "my-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	result, err := cp.Create(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result NodeClaim")
	}
	if result.Status.ProviderID != providerID {
		t.Errorf("expected providerID %q, got %q", providerID, result.Status.ProviderID)
	}
}

func TestDelete_Success(t *testing.T) {
	ctx := testCtx()
	var deletedClaim *v1.NodeClaim
	mp := &mockMachineProvider{
		DeleteFn: func(_ context.Context, claim *v1.NodeClaim) error {
			deletedClaim = claim
			return nil
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	nc := &v1.NodeClaim{ObjectMeta: metav1.ObjectMeta{Name: "test-claim"}}
	err := cp.Delete(ctx, nc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedClaim == nil || deletedClaim.Name != "test-claim" {
		t.Error("expected Delete to be called with the correct NodeClaim")
	}
}

func TestDelete_Error(t *testing.T) {
	ctx := testCtx()
	mp := &mockMachineProvider{
		DeleteFn: func(_ context.Context, _ *v1.NodeClaim) error {
			return fmt.Errorf("delete failed")
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	err := cp.Delete(ctx, &v1.NodeClaim{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "delete failed" {
		t.Errorf("expected 'delete failed', got %q", err.Error())
	}
}

func TestGet_EmptyProviderID(t *testing.T) {
	ctx := testCtx()
	cp := &CloudProvider{machineProvider: &mockMachineProvider{}}
	_, err := cp.Get(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty providerID")
	}
	if !containsStr(err.Error(), "no providerID supplied") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGet_MachineProviderError(t *testing.T) {
	ctx := testCtx()
	mp := &mockMachineProvider{
		GetFn: func(_ context.Context, _ string) (*capiv1beta1.Machine, error) {
			return nil, fmt.Errorf("provider get error")
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	_, err := cp.Get(ctx, "qcloud:///100003/ins-abc123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsStr(err.Error(), "provider get error") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGet_MachineNotFound(t *testing.T) {
	ctx := testCtx()
	mp := &mockMachineProvider{
		GetFn: func(_ context.Context, _ string) (*capiv1beta1.Machine, error) {
			return nil, nil
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	result, err := cp.Get(ctx, "qcloud:///100003/ins-abc123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for not-found machine, got %v", result)
	}
}

func TestGet_Success(t *testing.T) {
	ctx := testCtx()
	providerID := "qcloud:///100003/ins-abc123"
	mc := validMachine("test-machine", providerID, "ap-guangzhou-3")

	mp := &mockMachineProvider{
		GetFn: func(_ context.Context, id string) (*capiv1beta1.Machine, error) {
			if id == providerID {
				return mc, nil
			}
			return nil, nil
		},
	}
	zp := &mockZoneProvider{
		IDFromZoneFn: func(zone string) (string, error) {
			return "100003", nil
		},
	}
	cp := &CloudProvider{
		machineProvider: mp,
		zoneProvider:    zp,
	}
	result, err := cp.Get(ctx, providerID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status.ProviderID != providerID {
		t.Errorf("expected providerID %q, got %q", providerID, result.Status.ProviderID)
	}
}

func TestGetInstanceTypes_NilNodePool(t *testing.T) {
	ctx := testCtx()
	cp := &CloudProvider{}
	_, err := cp.GetInstanceTypes(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil nodePool")
	}
	if !containsStr(err.Error(), "node pool reference is nil") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestGetInstanceTypes_NilNodeClassRef(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	cp := &CloudProvider{kubeClient: fc}
	nodePool := &v1.NodePool{
		Spec: v1.NodePoolSpec{
			Template: v1.NodeClaimTemplate{
				Spec: v1.NodeClaimTemplateSpec{
					NodeClassRef: nil,
				},
			},
		},
	}
	_, err := cp.GetInstanceTypes(ctx, nodePool)
	if err == nil {
		t.Fatal("expected error for nil nodeClassRef")
	}
	if !containsStr(err.Error(), "node class reference is nil") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestGetInstanceTypes_EmptyName(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	cp := &CloudProvider{kubeClient: fc}
	nodePool := &v1.NodePool{
		Spec: v1.NodePoolSpec{
			Template: v1.NodeClaimTemplate{
				Spec: v1.NodeClaimTemplateSpec{
					NodeClassRef: &v1.NodeClassReference{
						Name:  "",
						Kind:  "TKEMachineNodeClass",
						Group: api.Group,
					},
				},
			},
		},
	}
	_, err := cp.GetInstanceTypes(ctx, nodePool)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !containsStr(err.Error(), "node class reference name is empty") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestGetInstanceTypes_Success(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	nc := readyNodeClass("my-class")
	fc.objects["my-class"] = nc

	expectedIT := simpleInstanceType("S5.MEDIUM4", "ap-guangzhou-3", "100003", v1.CapacityTypeOnDemand)
	ip := &mockInstanceTypeProvider{
		ListFn: func(_ context.Context, _ *api.TKEMachineNodeClass, _ bool) ([]*cloudprovider.InstanceType, error) {
			return []*cloudprovider.InstanceType{expectedIT}, nil
		},
	}
	cp := &CloudProvider{
		kubeClient:           fc,
		instancetypeProvider: ip,
	}
	nodePool := &v1.NodePool{
		Spec: v1.NodePoolSpec{
			Template: v1.NodeClaimTemplate{
				Spec: v1.NodeClaimTemplateSpec{
					NodeClassRef: &v1.NodeClassReference{
						Name:  "my-class",
						Kind:  "TKEMachineNodeClass",
						Group: api.Group,
					},
				},
			},
		},
	}
	result, err := cp.GetInstanceTypes(ctx, nodePool)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 instance type, got %d", len(result))
	}
	if result[0].Name != "S5.MEDIUM4" {
		t.Errorf("expected instance type name S5.MEDIUM4, got %q", result[0].Name)
	}
}

func TestList_CPEmpty(t *testing.T) {
	ctx := testCtx()
	mp := &mockMachineProvider{
		ListFn: func(_ context.Context) ([]*capiv1beta1.Machine, error) {
			return []*capiv1beta1.Machine{}, nil
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	result, err := cp.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 node claims, got %d", len(result))
	}
}

func TestList_CPMachineProviderError(t *testing.T) {
	ctx := testCtx()
	mp := &mockMachineProvider{
		ListFn: func(_ context.Context) ([]*capiv1beta1.Machine, error) {
			return nil, fmt.Errorf("list machines error")
		},
	}
	cp := &CloudProvider{machineProvider: mp}
	_, err := cp.List(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsStr(err.Error(), "list machines error") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestList_CPSuccess(t *testing.T) {
	ctx := testCtx()
	providerID1 := "qcloud:///100003/ins-abc1"
	providerID2 := "qcloud:///100003/ins-abc2"
	mc1 := validMachine("machine-1", providerID1, "ap-guangzhou-3")
	mc2 := validMachine("machine-2", providerID2, "ap-guangzhou-3")

	mp := &mockMachineProvider{
		ListFn: func(_ context.Context) ([]*capiv1beta1.Machine, error) {
			return []*capiv1beta1.Machine{mc1, mc2}, nil
		},
	}
	zp := &mockZoneProvider{
		IDFromZoneFn: func(zone string) (string, error) {
			return "100003", nil
		},
	}
	cp := &CloudProvider{
		machineProvider: mp,
		zoneProvider:    zp,
	}
	result, err := cp.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 node claims, got %d", len(result))
	}
	if result[0].Status.ProviderID != providerID1 {
		t.Errorf("expected providerID %q, got %q", providerID1, result[0].Status.ProviderID)
	}
	if result[1].Status.ProviderID != providerID2 {
		t.Errorf("expected providerID %q, got %q", providerID2, result[1].Status.ProviderID)
	}
}

func TestResolveNodeClassFromNodeClaim_Terminating(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	now := metav1.Now()
	nc := readyNodeClass("terminating-class")
	nc.DeletionTimestamp = &now
	fc.objects["terminating-class"] = nc

	cp := &CloudProvider{
		kubeClient:           fc,
		machineProvider:      &mockMachineProvider{},
		instancetypeProvider: &mockInstanceTypeProvider{},
		zoneProvider:         &mockZoneProvider{},
	}
	nodeClaim := &v1.NodeClaim{
		Spec: v1.NodeClaimSpec{
			NodeClassRef: &v1.NodeClassReference{
				Name:  "terminating-class",
				Kind:  "TKEMachineNodeClass",
				Group: api.Group,
			},
		},
	}
	// Create calls resolveNodeClassFromNodeClaim internally.
	// A terminating nodeClass should cause an InsufficientCapacityError.
	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error for terminating nodeClass")
	}
	if !cloudprovider.IsInsufficientCapacityError(err) {
		t.Errorf("expected InsufficientCapacityError, got %T: %v", err, err)
	}
	if !containsStr(err.Error(), "is terminating") {
		t.Errorf("expected 'is terminating' in error, got %q", err.Error())
	}
}

// --- Phase 3: additional branch coverage ---

func TestGet_MachineToNodeClaimError(t *testing.T) {
	ctx := testCtx()
	// Machine with bad CPU label -> resolveMachineToInstanceType fails
	badMachine := validMachine("bad-machine", "qcloud:///100003/ins-bad", "ap-guangzhou-3")
	badMachine.Labels[api.LabelInstanceCPU] = "not-a-number"

	mp := &mockMachineProvider{
		GetFn: func(_ context.Context, _ string) (*capiv1beta1.Machine, error) {
			return badMachine, nil
		},
	}
	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{machineProvider: mp, zoneProvider: zp}
	_, err := cp.Get(ctx, "qcloud:///100003/ins-bad")
	if err == nil {
		t.Fatal("expected error from machineToNodeClaim")
	}
	if !containsStr(err.Error(), "unable to convert Machine to NodeClaim") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestList_CPMachineToNodeClaimError(t *testing.T) {
	ctx := testCtx()
	badMachine := validMachine("bad-machine", "qcloud:///100003/ins-bad", "ap-guangzhou-3")
	badMachine.Labels[api.LabelInstanceMemoryGB] = "invalid"

	mp := &mockMachineProvider{
		ListFn: func(_ context.Context) ([]*capiv1beta1.Machine, error) {
			return []*capiv1beta1.Machine{badMachine}, nil
		},
	}
	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{machineProvider: mp, zoneProvider: zp}
	_, err := cp.List(ctx)
	if err == nil {
		t.Fatal("expected error from machineToNodeClaim in List")
	}
}

func TestMachineToNodeClaim_PhaseDeleting(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("deleting-machine", "qcloud:///100003/ins-del", "ap-guangzhou-3")
	phase := capiv1beta1.PhaseDeleting
	mc.Status.Phase = &phase

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	nodeClaim, err := cp.machineToNodeClaim(ctx, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeClaim.DeletionTimestamp == nil {
		t.Error("expected DeletionTimestamp to be set for deleting machine")
	}
}

func TestMachineToNodeClaim_MissingNodePoolLabel(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("no-nodepool", "qcloud:///100003/ins-np", "ap-guangzhou-3")
	// validMachine doesn't set NodePoolLabelKey; verify it is missing from result
	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	nodeClaim, err := cp.machineToNodeClaim(ctx, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := nodeClaim.Labels[v1.NodePoolLabelKey]; ok {
		t.Error("expected NodePoolLabelKey to be absent")
	}
}

func TestMachineToNodeClaim_WithNodePoolLabel(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("with-nodepool", "qcloud:///100003/ins-wnp", "ap-guangzhou-3")
	mc.Labels[v1.NodePoolLabelKey] = "my-pool"

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	nodeClaim, err := cp.machineToNodeClaim(ctx, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeClaim.Labels[v1.NodePoolLabelKey] != "my-pool" {
		t.Errorf("expected NodePoolLabelKey 'my-pool', got %q", nodeClaim.Labels[v1.NodePoolLabelKey])
	}
}

func TestResolveMachineToInstanceType_BadProviderSpec(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("bad-spec", "qcloud:///100003/ins-bs", "ap-guangzhou-3")
	// ProviderSpecFromRawExtension returns no error for nil, so use invalid JSON
	mc.Spec.ProviderSpec.Value = &runtime.RawExtension{Raw: []byte("not-json")}

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	_, err := cp.machineToNodeClaim(ctx, mc)
	if err == nil {
		t.Fatal("expected error for invalid ProviderSpec JSON")
	}
}

func TestResolveMachineToInstanceType_MissingCPUCapacity(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("no-cpu", "qcloud:///100003/ins-nc", "ap-guangzhou-3")
	delete(mc.Annotations, api.CapacityGroup+api.AnnotationCPU)

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	_, err := cp.machineToNodeClaim(ctx, mc)
	if err == nil {
		t.Fatal("expected error for missing CPU capacity annotation")
	}
	if !containsStr(err.Error(), "no cpu capacity found") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestResolveMachineToInstanceType_MissingMemoryCapacity(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("no-mem", "qcloud:///100003/ins-nm", "ap-guangzhou-3")
	delete(mc.Annotations, api.CapacityGroup+api.AnnotationMemory)

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	_, err := cp.machineToNodeClaim(ctx, mc)
	if err == nil {
		t.Fatal("expected error for missing memory capacity annotation")
	}
	if !containsStr(err.Error(), "no memory capacity found") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestGetInstanceTypes_GetError(t *testing.T) {
	ctx := testCtx()
	fc := newCPFakeClient()
	fc.getErr = fmt.Errorf("server error")
	cp := &CloudProvider{kubeClient: fc}
	nodePool := &v1.NodePool{
		Spec: v1.NodePoolSpec{
			Template: v1.NodeClaimTemplate{
				Spec: v1.NodeClaimTemplateSpec{
					NodeClassRef: &v1.NodeClassReference{
						Name:  "my-class",
						Kind:  "TKEMachineNodeClass",
						Group: api.Group,
					},
				},
			},
		},
	}
	_, err := cp.GetInstanceTypes(ctx, nodePool)
	if err == nil {
		t.Fatal("expected error for server error on Get")
	}
	if !containsStr(err.Error(), "server error") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestResolveMachineToInstanceType_BadUnitPrice(t *testing.T) {
	ctx := testCtx()
	mc := validMachine("bad-price", "qcloud:///100003/ins-bp", "ap-guangzhou-3")
	mc.Annotations[api.AnnotationUnitPrice] = "not-a-float"

	zp := &mockZoneProvider{
		IDFromZoneFn: func(_ string) (string, error) { return "100003", nil },
	}
	cp := &CloudProvider{zoneProvider: zp}
	_, err := cp.machineToNodeClaim(ctx, mc)
	if err == nil {
		t.Fatal("expected error for bad unit price")
	}
}
