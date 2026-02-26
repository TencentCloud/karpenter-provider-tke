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

package machine

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	corev1 "k8s.io/api/core/v1"
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

type mockZoneProvider struct {
	zoneFromIDFunc func(string) (string, error)
	idFromZoneFunc func(string) (string, error)
}

func (m *mockZoneProvider) ZoneFromID(id string) (string, error) {
	if m.zoneFromIDFunc != nil {
		return m.zoneFromIDFunc(id)
	}
	return "ap-guangzhou-1", nil
}

func (m *mockZoneProvider) IDFromZone(zone string) (string, error) {
	if m.idFromZoneFunc != nil {
		return m.idFromZoneFunc(zone)
	}
	return "100001", nil
}

func createInstanceType(name string, cpu int64, memory int64, price float64, zone string, capacityType string) *cloudprovider.InstanceType {
	requirements := scheduling.NewRequirements(
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
		scheduling.NewRequirement(api.LabelInstanceFamily, corev1.NodeSelectorOpIn, "S3"),
		scheduling.NewRequirement(api.LabelInstanceCPU, corev1.NodeSelectorOpIn, fmt.Sprintf("%d", cpu)),
		scheduling.NewRequirement(api.LabelInstanceMemoryGB, corev1.NodeSelectorOpIn, fmt.Sprintf("%d", memory)),
	)

	capacity := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse(fmt.Sprintf("%d", cpu)),
		corev1.ResourceMemory:           resource.MustParse(fmt.Sprintf("%dGi", memory)),
		corev1.ResourcePods:             resource.MustParse("110"),
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}

	overhead := &cloudprovider.InstanceTypeOverhead{
		KubeReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		},
		SystemReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		},
		EvictionThreshold: corev1.ResourceList{
			corev1.ResourceMemory:           resource.MustParse("100Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
		},
	}

	offering := &cloudprovider.Offering{
		Requirements: requirements,
		Price:        price,
		Available:    true,
	}

	return &cloudprovider.InstanceType{
		Name:         name,
		Requirements: requirements,
		Capacity:     capacity,
		Overhead:     overhead,
		Offerings:    []*cloudprovider.Offering{offering},
	}
}

func createDefaultNodeClass() *api.TKEMachineNodeClass {
	return &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nodeclass",
			UID:  "test-uid",
		},
		Spec: api.TKEMachineNodeClassSpec{
			SubnetSelectorTerms: []api.SubnetSelectorTerm{
				{
					ID: "subnet-123456",
				},
			},
			SecurityGroupSelectorTerms: []api.SecurityGroupSelectorTerm{
				{
					ID: "sg-123456",
				},
			},
			SSHKeySelectorTerms: []api.SSHKeySelectorTerm{
				{
					ID: "skey-123456",
				},
			},
			SystemDisk: &api.SystemDisk{
				Size: 50,
				Type: api.DiskTypeCloudPremium,
			},
			DataDisks: []api.DataDisk{
				{
					Size: 100,
					Type: api.DiskTypeCloudSSD,
				},
			},
			Tags: map[string]string{
				"Environment": "test",
				"Team":        "karpenter",
			},
		},
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{
				{
					ID:   "subnet-123456",
					Zone: "ap-guangzhou-1",
				},
			},
			SecurityGroups: []api.SecurityGroup{
				{
					ID: "sg-123456",
				},
			},
			SSHKeys: []api.SSHKey{
				{
					ID: "skey-123456",
				},
			},
		},
	}
}

func createDefaultNodeClaim() *v1.NodeClaim {
	return &v1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nodeclaim",
			UID:  "test-nodeclaim-uid",
			Labels: map[string]string{
				v1.NodePoolLabelKey: "test-nodepool",
			},
			Annotations: map[string]string{
				"beta.karpenter.sh/tke.machine.spec/annotations": "key1=value1,key2=value2",
				"beta.karpenter.sh/tke.machine.meta/annotations": "meta1=meta-value1",
				"beta.karpenter.sh/tke.kubelet.arg/custom-arg":   "custom-value",
				"beta.karpenter.sh/tke.kernel.arg/custom-kernel": "kernel-value",
				"beta.karpenter.sh/tke.hosts.ip/192.168.1.1":     "host1.example.com,host2.example.com",
				"beta.karpenter.sh/tke.machine/nameservers":      "8.8.8.8,8.8.4.4",
			},
		},
		Spec: v1.NodeClaimSpec{
			Requirements: []v1.NodeSelectorRequirementWithMinValues{
				{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      corev1.LabelTopologyZone,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"ap-guangzhou-1"},
					},
				},
				{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      v1.CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{v1.CapacityTypeOnDemand},
					},
				},
			},
			Resources: v1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	}
}

func createFakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return &simpleFakeClient{
		objects: objects,
		scheme:  scheme,
	}
}

type simpleFakeClient struct {
	objects []client.Object
	scheme  *runtime.Scheme
}

func (c *simpleFakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	for _, o := range c.objects {
		if o.GetName() == key.Name && o.GetNamespace() == key.Namespace {
			return nil
		}
	}
	return fmt.Errorf("object not found")
}

func (c *simpleFakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// Handle MachineList
	if machineList, ok := list.(*capiv1beta1.MachineList); ok {
		machineList.Items = []capiv1beta1.Machine{}
		for _, o := range c.objects {
			if machine, ok := o.(*capiv1beta1.Machine); ok {
				machineList.Items = append(machineList.Items, *machine)
			}
		}
	}
	return nil
}

func (c *simpleFakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.objects = append(c.objects, obj)
	return nil
}

func (c *simpleFakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	// Remove the object from the list
	newObjects := []client.Object{}
	for _, o := range c.objects {
		if o.GetName() != obj.GetName() || o.GetNamespace() != obj.GetNamespace() {
			newObjects = append(newObjects, o)
		}
	}
	c.objects = newObjects
	return nil
}

func (c *simpleFakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (c *simpleFakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

func (c *simpleFakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}

func (c *simpleFakeClient) Status() client.StatusWriter {
	return &simpleStatusWriter{client: c}
}

func (c *simpleFakeClient) SubResource(subResource string) client.SubResourceClient {
	return &simpleSubResourceClient{client: c}
}

func (c *simpleFakeClient) Scheme() *runtime.Scheme {
	return c.scheme
}

func (c *simpleFakeClient) RESTMapper() meta.RESTMapper {
	return &simpleRESTMapper{}
}

func (c *simpleFakeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := c.scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object")
	}
	return gvks[0], nil
}

func (c *simpleFakeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

type simpleStatusWriter struct {
	client *simpleFakeClient
}

func (w *simpleStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (w *simpleStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (w *simpleStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

type simpleSubResourceClient struct {
	client *simpleFakeClient
}

func (c *simpleSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (c *simpleSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}

func (c *simpleSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (c *simpleSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

type simpleRESTMapper struct{}

func (m *simpleRESTMapper) ResourceFor(resource schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return resource, nil
}

func (m *simpleRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{
		Group:   resource.Group,
		Version: resource.Version,
		Kind:    resource.Resource,
	}, nil
}

func (m *simpleRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	gvk, _ := m.KindFor(resource)
	return []schema.GroupVersionKind{gvk}, nil
}

func (m *simpleRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return resource, nil
}

func (m *simpleRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return []schema.GroupVersionResource{input}, nil
}

func (m *simpleRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{
		Resource: schema.GroupVersionResource{
			Group:    gk.Group,
			Version:  "v1",
			Resource: gk.Kind,
		},
		GroupVersionKind: schema.GroupVersionKind{
			Group:   gk.Group,
			Version: "v1",
			Kind:    gk.Kind,
		},
	}, nil
}

func (m *simpleRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	mapping, _ := m.RESTMapping(gk, versions...)
	return []*meta.RESTMapping{mapping}, nil
}

func createScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = capiv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)
	return scheme
}

func TestCreate_BasicSuccess(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}

	if providerSpec == nil {
		t.Fatal("Expected providerSpec to be created, got nil")
	}

	if machine.Spec.DisplayName != nodeClaim.Name {
		t.Errorf("Expected DisplayName %s, got %s", nodeClaim.Name, machine.Spec.DisplayName)
	}

	if machine.Spec.Zone != "ap-guangzhou-1" {
		t.Errorf("Expected Zone ap-guangzhou-1, got %s", machine.Spec.Zone)
	}

	if machine.Spec.SubnetID != "subnet-123456" {
		t.Errorf("Expected SubnetID subnet-123456, got %s", machine.Spec.SubnetID)
	}

	expectedLabels := map[string]string{
		v1.NodePoolLabelKey:       "test-nodepool",
		api.LabelNodeClaim:        nodeClaim.Name,
		api.LabelNodeClass:        nodeClass.Name,
		api.LabelInstanceFamily:   "S3",
		api.LabelInstanceCPU:      "4",
		api.LabelInstanceMemoryGB: "8",
		v1.CapacityTypeLabelKey:   v1.CapacityTypeOnDemand,
	}

	for k, v := range expectedLabels {
		if machine.Labels[k] != v {
			t.Errorf("Expected label %s=%s, got %s", k, v, machine.Labels[k])
		}
	}

	if providerSpec.InstanceType != "S3.MEDIUM4" {
		t.Errorf("Expected InstanceType S3.MEDIUM4, got %s", providerSpec.InstanceType)
	}

	if providerSpec.InstanceChargeType != capiv1beta1.PostpaidByHourChargeType {
		t.Errorf("Expected InstanceChargeType %s, got %s", capiv1beta1.PostpaidByHourChargeType, providerSpec.InstanceChargeType)
	}

	if len(providerSpec.SecurityGroupIDs) != 1 || providerSpec.SecurityGroupIDs[0] != "sg-123456" {
		t.Errorf("Expected SecurityGroupIDs [sg-123456], got %v", providerSpec.SecurityGroupIDs)
	}

	if len(providerSpec.KeyIDs) != 1 || providerSpec.KeyIDs[0] != "skey-123456" {
		t.Errorf("Expected KeyIDs [skey-123456], got %v", providerSpec.KeyIDs)
	}
}

func TestCreate_MultipleInstanceTypes(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.LARGE8", 8, 16, 1.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
		createInstanceType("S3.XLARGE16", 16, 32, 2.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.InstanceType != "S3.MEDIUM4" {
		t.Errorf("Expected cheapest instance type S3.MEDIUM4, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[api.LabelInstanceCPU] != "4" {
		t.Errorf("Expected CPU label 4, got %s", machine.Labels[api.LabelInstanceCPU])
	}

	if machine.Labels[api.LabelInstanceMemoryGB] != "8" {
		t.Errorf("Expected Memory label 8, got %s", machine.Labels[api.LabelInstanceMemoryGB])
	}
}

func TestCreate_WithAnnotations(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine.Annotations == nil {
		t.Fatal("Expected machine annotations to be set")
	}

	if machine.Annotations[api.AnnotationManagedBy] == "" {
		t.Error("Expected managed by annotation to be set")
	}

	if machine.Annotations[api.AnnotationUnitPrice] == "" {
		t.Error("Expected unit price annotation to be set")
	}

	kubeletArgs := providerSpec.Management.KubeletArgs
	if len(kubeletArgs) == 0 {
		t.Error("Expected kubelet args to be set")
	}

	expectedTaintArg := "register-with-taints=karpenter.sh/unregistered:NoExecute"
	if !lo.Contains(kubeletArgs, expectedTaintArg) {
		t.Errorf("Expected taint arg %s in %v", expectedTaintArg, kubeletArgs)
	}
}

func TestCreate_DifferentCapacityTypes(t *testing.T) {
	tests := []struct {
		name           string
		capacityType   string
		expectedType   capiv1beta1.MachineType
		expectedCharge capiv1beta1.InstanceChargeType
	}{
		{
			name:           "OnDemand",
			capacityType:   v1.CapacityTypeOnDemand,
			expectedType:   capiv1beta1.MachineTypeNative,
			expectedCharge: capiv1beta1.PostpaidByHourChargeType,
		},
		{
			name:           "Spot",
			capacityType:   v1.CapacityTypeSpot,
			expectedType:   capiv1beta1.MachineTypeNativeCVM,
			expectedCharge: capiv1beta1.SpotpaidChargeType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := createScheme()
			ctx := context.Background()

			nodeClass := createDefaultNodeClass()
			nodeClaim := createDefaultNodeClaim()
			nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
				{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      v1.CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{tt.capacityType},
					},
				},
			}

			instanceTypes := []*cloudprovider.InstanceType{
				createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", tt.capacityType),
			}

			mockZoneProvider := &mockZoneProvider{}
			fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

			provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

			machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if machine.Spec.ProviderSpec.Type != tt.expectedType {
				t.Errorf("Expected MachineType %s, got %s", tt.expectedType, machine.Spec.ProviderSpec.Type)
			}

			if providerSpec.InstanceChargeType != tt.expectedCharge {
				t.Errorf("Expected InstanceChargeType %s, got %s", tt.expectedCharge, providerSpec.InstanceChargeType)
			}

			if machine.Labels[v1.CapacityTypeLabelKey] != tt.capacityType {
				t.Errorf("Expected CapacityType label %s, got %s", tt.capacityType, machine.Labels[v1.CapacityTypeLabelKey])
			}
		})
	}
}

func TestCreate_FullConfiguration(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClass.Spec.DataDisks = []api.DataDisk{
		{
			Size:        100,
			Type:        api.DiskTypeCloudSSD,
			MountTarget: lo.ToPtr("/data"),
			FileSystem:  lo.ToPtr(api.FileSystemEXT4),
		},
		{
			Size: 200,
			Type: api.DiskTypeCloudPremium,
		},
	}

	nodeClass.Spec.InternetAccessible = &api.InternetAccessible{
		MaxBandwidthOut: lo.ToPtr(int32(10)),
		ChargeType:      lo.ToPtr(api.TrafficPostpaidByHour),
	}

	nodeClass.Spec.LifecycleScript = &api.LifecycleScript{
		PreInitScript:  lo.ToPtr("echo 'pre-init'"),
		PostInitScript: lo.ToPtr("echo 'post-init'"),
	}

	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.DataDisks) != 2 {
		t.Errorf("Expected 2 data disks, got %d", len(providerSpec.DataDisks))
	}

	firstDisk := providerSpec.DataDisks[0]
	if firstDisk.DiskSize != 100 {
		t.Errorf("Expected first disk size 100, got %d", firstDisk.DiskSize)
	}
	if firstDisk.DiskType != capiv1beta1.CloudSSDDiskType {
		t.Errorf("Expected first disk type %s, got %s", capiv1beta1.CloudSSDDiskType, firstDisk.DiskType)
	}
	if firstDisk.MountTarget != "/data" {
		t.Errorf("Expected first disk mount target /data, got %s", firstDisk.MountTarget)
	}
	if firstDisk.FileSystem != "ext4" {
		t.Errorf("Expected first disk filesystem ext4, got %s", firstDisk.FileSystem)
	}

	if providerSpec.InternetAccessible == nil {
		t.Fatal("Expected InternetAccessible to be configured")
	}

	if providerSpec.InternetAccessible.MaxBandwidthOut != 10 {
		t.Errorf("Expected MaxBandwidthOut 10, got %d", providerSpec.InternetAccessible.MaxBandwidthOut)
	}

	if providerSpec.InternetAccessible.ChargeType != capiv1beta1.TrafficPostpaidByHour {
		t.Errorf("Expected ChargeType %s, got %s", capiv1beta1.TrafficPostpaidByHour, providerSpec.InternetAccessible.ChargeType)
	}

	if providerSpec.Lifecycle.PreInit != "echo 'pre-init'" {
		t.Errorf("Expected PreInit script 'echo pre-init', got %s", providerSpec.Lifecycle.PreInit)
	}

	if providerSpec.Lifecycle.PostInit != "echo 'post-init'" {
		t.Errorf("Expected PostInit script 'echo post-init', got %s", providerSpec.Lifecycle.PostInit)
	}

	if machine.Annotations[capiv1beta1.AnnotationMachineCloudTag] == "" {
		t.Error("Expected cloud tag annotation to be set")
	}
}

func TestCreate_ZoneFromIDFailure(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{
		zoneFromIDFunc: func(id string) (string, error) {
			return "", fmt.Errorf("zone not found")
		},
	}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if machine != nil {
		t.Error("Expected machine to be nil, got non-nil")
	}

	if providerSpec != nil {
		t.Error("Expected providerSpec to be nil, got non-nil")
	}

	expectedError := "getting zone failed"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}

func TestCreate_SubnetNotFound(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClass.Status.Subnets = []api.Subnet{
		{
			ID:   "subnet-999999",
			Zone: "ap-shanghai-1",
		},
	}

	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if machine != nil {
		t.Error("Expected machine to be nil, got non-nil")
	}

	if providerSpec != nil {
		t.Error("Expected providerSpec to be nil, got non-nil")
	}

	expectedError := "subnet for ap-guangzhou-1 not found"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}

func TestCreate_EmptyInstanceTypes(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{} // 空列表

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for empty instance types, but no panic occurred")
		}
	}()

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err == nil {
		t.Error("Expected error or panic, got nil error")
	}

	if machine != nil {
		t.Error("Expected machine to be nil, got non-nil")
	}

	if providerSpec != nil {
		t.Error("Expected providerSpec to be nil, got non-nil")
	}
}

func TestCreate_KubeClientCreateFailure(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		return
	}

	if providerSpec != nil {
		if providerSpec.InstanceType != "S3.MEDIUM4" {
			t.Errorf("Expected InstanceType S3.MEDIUM4, got %s", providerSpec.InstanceType)
		}
	}
}

func TestCreate_InvalidNodeClassConfig(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClass.Status.Subnets = []api.Subnet{}

	nodeClaim := createDefaultNodeClaim()
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if machine != nil {
		t.Error("Expected machine to be nil, got non-nil")
	}

	if providerSpec != nil {
		t.Error("Expected providerSpec to be nil, got non-nil")
	}

	expectedError := "subnet for ap-guangzhou-1 not found"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}

func TestCreate_InstanceTypeTruncation(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	var instanceTypes []*cloudprovider.InstanceType
	for i := 0; i < 70; i++ {
		instanceTypes = append(instanceTypes, createInstanceType(
			fmt.Sprintf("S3.MEDIUM%d", i),
			4, 8, float64(i)*0.1, "ap-guangzhou-1", v1.CapacityTypeOnDemand,
		))
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.InstanceType != "S3.MEDIUM0" {
		t.Errorf("Expected cheapest instance type S3.MEDIUM0, got %s", providerSpec.InstanceType)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}
}

func TestCreate_MinimalConfiguration(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClass.Spec.SystemDisk = nil
	nodeClass.Spec.DataDisks = nil
	nodeClass.Spec.InternetAccessible = nil
	nodeClass.Spec.LifecycleScript = nil
	nodeClass.Spec.Tags = nil

	nodeClaim := createDefaultNodeClaim()
	nodeClaim.Annotations = map[string]string{}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.SystemDisk.DiskSize != 50 {
		t.Errorf("Expected default system disk size 50, got %d", providerSpec.SystemDisk.DiskSize)
	}

	if providerSpec.SystemDisk.DiskType != capiv1beta1.CloudPremiumDiskType {
		t.Errorf("Expected default system disk type %s, got %s", capiv1beta1.CloudPremiumDiskType, providerSpec.SystemDisk.DiskType)
	}

	if len(providerSpec.DataDisks) != 0 {
		t.Errorf("Expected no data disks, got %d", len(providerSpec.DataDisks))
	}

	if providerSpec.InternetAccessible != nil {
		t.Error("Expected InternetAccessible to be nil")
	}

	if providerSpec.Lifecycle.PreInit != "" {
		t.Errorf("Expected PreInit to be empty, got %s", providerSpec.Lifecycle.PreInit)
	}
}

func TestCreate_NullAndEmptyValues(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	nodeClaim.Annotations["beta.karpenter.sh/tke.kubelet.arg/empty"] = ""
	nodeClaim.Annotations["beta.karpenter.sh/tke.hosts.ip/"] = "host1.example.com"
	nodeClaim.Annotations["beta.karpenter.sh/tke.machine/nameservers"] = ""

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	emptyKubeletArg := "empty="
	if lo.Contains(providerSpec.Management.KubeletArgs, emptyKubeletArg) {
		t.Errorf("Expected empty kubelet arg to be ignored, but found %s", emptyKubeletArg)
	}

	if len(providerSpec.Management.Nameservers) > 0 {
		t.Errorf("Expected empty nameservers to be ignored, got %v", providerSpec.Management.Nameservers)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}
}

func TestCreate_ResourceBoundaryValues(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MICRO2", 1, 2, 0.1, "ap-guangzhou-1", v1.CapacityTypeOnDemand),     // 最小值
		createInstanceType("S3.XLARGE32", 32, 64, 2.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand), // 较大值
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.InstanceType != "S3.MICRO2" {
		t.Errorf("Expected cheapest instance type S3.MICRO2, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[api.LabelInstanceCPU] != "1" {
		t.Errorf("Expected CPU label 1, got %s", machine.Labels[api.LabelInstanceCPU])
	}

	if machine.Labels[api.LabelInstanceMemoryGB] != "2" {
		t.Errorf("Expected Memory label 2, got %s", machine.Labels[api.LabelInstanceMemoryGB])
	}

	if machine.Annotations[api.AnnotationUnitPrice] == "" {
		t.Error("Expected unit price annotation to be set")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}

func TestCreate_WithGPUResources(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add GPU annotations
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationGPUCUDAKey] = "11.4"
	nodeClaim.Annotations[api.AnnotationGPUCUDNNKey] = "8.2.4"
	nodeClaim.Annotations[api.AnnotationGPUMIGEnableKey] = "true"
	nodeClaim.Annotations[api.AnnotationFabricKey] = "true"

	// Create instance type with GPU
	instanceType := createInstanceType("GN7.2XLARGE32", 8, 32, 2.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("1")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}

	// Verify GPU configuration
	if machine.Spec.GPUConfig.Driver != "470.82.01" {
		t.Errorf("Expected GPU Driver 470.82.01, got %s", machine.Spec.GPUConfig.Driver)
	}

	if machine.Spec.GPUConfig.CUDA != "11.4" {
		t.Errorf("Expected GPU CUDA 11.4, got %s", machine.Spec.GPUConfig.CUDA)
	}

	if machine.Spec.GPUConfig.CUDNN != "8.2.4" {
		t.Errorf("Expected GPU CUDNN 8.2.4, got %s", machine.Spec.GPUConfig.CUDNN)
	}

	if !machine.Spec.GPUConfig.MIGEnable {
		t.Error("Expected GPU MIGEnable to be true")
	}

	if !machine.Spec.GPUConfig.Fabric {
		t.Error("Expected GPU Fabric to be true")
	}

	// Verify GPU count annotation
	if machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount] != "1" {
		t.Errorf("Expected GPU count annotation to be 1, got %s", machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount])
	}

	if providerSpec == nil {
		t.Fatal("Expected providerSpec to be created, got nil")
	}
}

func TestCreate_WithGPUResourcesPartialConfig(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add only some GPU annotations
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationGPUCUDAKey] = "11.4"
	// No CUDNN, MIGEnable, or Fabric

	// Create instance type with GPU
	instanceType := createInstanceType("GN7.2XLARGE32", 8, 32, 2.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("2")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify partial GPU configuration
	if machine.Spec.GPUConfig.Driver != "470.82.01" {
		t.Errorf("Expected GPU Driver 470.82.01, got %s", machine.Spec.GPUConfig.Driver)
	}

	if machine.Spec.GPUConfig.CUDA != "11.4" {
		t.Errorf("Expected GPU CUDA 11.4, got %s", machine.Spec.GPUConfig.CUDA)
	}

	if machine.Spec.GPUConfig.CUDNN != "" {
		t.Errorf("Expected GPU CUDNN to be empty, got %s", machine.Spec.GPUConfig.CUDNN)
	}

	if machine.Spec.GPUConfig.MIGEnable {
		t.Error("Expected GPU MIGEnable to be false")
	}

	if machine.Spec.GPUConfig.Fabric {
		t.Error("Expected GPU Fabric to be false")
	}

	// Verify GPU count annotation
	if machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount] != "2" {
		t.Errorf("Expected GPU count annotation to be 2, got %s", machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount])
	}
}

func TestCreate_WithoutGPUResources(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add GPU annotations but instance type has no GPU
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationGPUCUDAKey] = "11.4"

	// Create instance type WITHOUT GPU
	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// GPU annotations should be ignored when instance has no GPU
	if machine.Spec.GPUConfig.Driver != "" {
		t.Errorf("Expected GPU Driver to be empty for non-GPU instance, got %s", machine.Spec.GPUConfig.Driver)
	}

	if machine.Spec.GPUConfig.CUDA != "" {
		t.Errorf("Expected GPU CUDA to be empty for non-GPU instance, got %s", machine.Spec.GPUConfig.CUDA)
	}

	// GPU count annotation should not be set
	if _, exists := machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount]; exists {
		t.Error("Expected GPU count annotation to not exist for non-GPU instance")
	}
}

func TestCreate_WithGPUMIGEnableFalse(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Set MIGEnable to false explicitly
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationGPUMIGEnableKey] = "false"

	// Create instance type with GPU
	instanceType := createInstanceType("GN7.2XLARGE32", 8, 32, 2.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("1")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// MIGEnable should be false
	if machine.Spec.GPUConfig.MIGEnable {
		t.Error("Expected GPU MIGEnable to be false")
	}
}

func TestCreate_WithGPUFabricFalse(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Set Fabric to false explicitly
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationFabricKey] = "false"

	// Create instance type with GPU
	instanceType := createInstanceType("GN7.2XLARGE32", 8, 32, 2.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("1")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Fabric should be false
	if machine.Spec.GPUConfig.Fabric {
		t.Error("Expected GPU Fabric to be false")
	}
}

func TestCreate_WithMultipleGPUs(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = "470.82.01"
	nodeClaim.Annotations[api.AnnotationGPUCUDAKey] = "11.4"

	// Create instance type with multiple GPUs
	instanceType := createInstanceType("GN7.8XLARGE128", 32, 128, 10.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("4")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify GPU count annotation for multiple GPUs
	if machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount] != "4" {
		t.Errorf("Expected GPU count annotation to be 4, got %s", machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount])
	}

	// Verify GPU configuration is still set
	if machine.Spec.GPUConfig.Driver != "470.82.01" {
		t.Errorf("Expected GPU Driver 470.82.01, got %s", machine.Spec.GPUConfig.Driver)
	}
}

func TestCreate_WithGPUEmptyAnnotations(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add empty GPU annotations
	nodeClaim.Annotations[api.AnnotationGPUDriverKey] = ""
	nodeClaim.Annotations[api.AnnotationGPUCUDAKey] = ""
	nodeClaim.Annotations[api.AnnotationGPUCUDNNKey] = ""

	// Create instance type with GPU
	instanceType := createInstanceType("GN7.2XLARGE32", 8, 32, 2.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceType.Capacity[corev1.ResourceName(api.ResourceNVIDIAGPU)] = resource.MustParse("1")

	instanceTypes := []*cloudprovider.InstanceType{instanceType}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Empty annotations should not set GPU config
	if machine.Spec.GPUConfig.Driver != "" {
		t.Errorf("Expected GPU Driver to be empty, got %s", machine.Spec.GPUConfig.Driver)
	}

	if machine.Spec.GPUConfig.CUDA != "" {
		t.Errorf("Expected GPU CUDA to be empty, got %s", machine.Spec.GPUConfig.CUDA)
	}

	if machine.Spec.GPUConfig.CUDNN != "" {
		t.Errorf("Expected GPU CUDNN to be empty, got %s", machine.Spec.GPUConfig.CUDNN)
	}

	// GPU count annotation should still be set
	if machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount] != "1" {
		t.Errorf("Expected GPU count annotation to be 1, got %s", machine.Annotations[api.CapacityGroup+api.AnnotationGPUCount])
	}
}

// Test Get method
func TestGet_Success(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Create a machine first
	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	// Test Get with valid providerID
	if machine.Spec.ProviderID != nil {
		retrievedMachine, err := provider.Get(ctx, *machine.Spec.ProviderID)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if retrievedMachine == nil {
			t.Fatal("Expected machine to be retrieved, got nil")
		}

		if retrievedMachine.Name != machine.Name {
			t.Errorf("Expected machine name %s, got %s", machine.Name, retrievedMachine.Name)
		}
	}
}

func TestGet_NotFound(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test Get with non-existent providerID
	_, err := provider.Get(ctx, "non-existent-provider-id")
	if err == nil {
		t.Fatal("Expected error for non-existent providerID, got nil")
	}
}

// Test List method
func TestList_Success(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	// Manually create a machine with proper name and owner reference
	machine := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nodeClaim.APIVersion,
				Kind:       "NodeClaim",
				Name:       nodeClaim.Name,
				UID:        nodeClaim.UID,
			}},
		},
	}

	err := fakeClient.Create(ctx, machine)
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	// Verify machine was created
	machineList := &capiv1beta1.MachineList{}
	err = fakeClient.List(ctx, machineList)
	if err != nil {
		t.Fatalf("Failed to list machines directly: %v", err)
	}
	t.Logf("Total machines in client: %d", len(machineList.Items))
	for i, m := range machineList.Items {
		t.Logf("Machine %d: Name=%s, OwnerRefs=%d", i, m.Name, len(m.OwnerReferences))
		for j, o := range m.OwnerReferences {
			t.Logf("  OwnerRef %d: Kind=%s, Name=%s", j, o.Kind, o.Name)
		}
	}

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test List
	machines, err := provider.List(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(machines) == 0 {
		t.Fatal("Expected at least one machine, got none")
	}

	// Verify the listed machine matches the created one
	found := false
	for _, m := range machines {
		if m.Name == machine.Name {
			found = true
			break
		}
	}
	if !found {
		t.Error("Created machine not found in list")
	}
}

func TestList_Empty(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test List without creating any machines
	machines, err := provider.List(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(machines) != 0 {
		t.Errorf("Expected no machines, got %d", len(machines))
	}
}

// Test Delete method
func TestDelete_WithProviderID(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	// Manually create a machine with proper name and provider ID
	providerID := "test-provider-id"
	machine := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nodeClaim.APIVersion,
				Kind:       "NodeClaim",
				Name:       nodeClaim.Name,
				UID:        nodeClaim.UID,
			}},
		},
		Spec: capiv1beta1.MachineSpec{
			ProviderID: &providerID,
		},
	}

	err := fakeClient.Create(ctx, machine)
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	// Update nodeClaim with providerID
	err = fakeClient.Get(ctx, client.ObjectKey{Name: nodeClaim.Name, Namespace: nodeClaim.Namespace}, nodeClaim)
	if err != nil {
		t.Fatalf("Failed to get nodeClaim: %v", err)
	}
	nodeClaim.Status.ProviderID = providerID
	err = fakeClient.Status().Update(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Failed to update nodeClaim: %v", err)
	}

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test Delete
	err = provider.Delete(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify machine is deleted
	_, err = provider.Get(ctx, providerID)
	if err == nil {
		t.Error("Expected machine to be deleted")
	}
}

func TestDelete_WithoutProviderID(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	// Manually create a machine without provider ID
	machine := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nodeClaim.APIVersion,
				Kind:       "NodeClaim",
				Name:       nodeClaim.Name,
				UID:        nodeClaim.UID,
			}},
		},
	}

	err := fakeClient.Create(ctx, machine)
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	// Get the latest nodeClaim from client
	err = fakeClient.Get(ctx, client.ObjectKey{Name: nodeClaim.Name, Namespace: nodeClaim.Namespace}, nodeClaim)
	if err != nil {
		t.Fatalf("Failed to get nodeClaim: %v", err)
	}

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test Delete without providerID (should search by owner reference)
	nodeClaim.Status.ProviderID = ""
	err = provider.Delete(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify machine is deleted by checking it's not in the list
	machineList := &capiv1beta1.MachineList{}
	err = fakeClient.List(ctx, machineList)
	if err != nil {
		t.Fatalf("Failed to list machines: %v", err)
	}

	for _, m := range machineList.Items {
		if m.Name == machine.Name {
			t.Error("Expected machine to be deleted")
		}
	}
}

func TestDelete_NotFound(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Test Delete without creating any machines
	nodeClaim.Status.ProviderID = ""
	err := provider.Delete(ctx, nodeClaim)
	if err == nil {
		t.Fatal("Expected error for non-existent machine, got nil")
	}
}

// Test getTargetAnnotations method
func TestGetTargetAnnotations_ValidFormat(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	annotations := map[string]string{
		"test-key": "key1=value1,key2=value2,key3=value3",
	}

	result := provider.getTargetAnnotations("test-key", annotations)

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %s", result["key1"])
	}

	if result["key2"] != "value2" {
		t.Errorf("Expected key2=value2, got %s", result["key2"])
	}

	if result["key3"] != "value3" {
		t.Errorf("Expected key3=value3, got %s", result["key3"])
	}
}

func TestGetTargetAnnotations_InvalidFormat(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	annotations := map[string]string{
		"test-key": "invalid,format,without,equals",
	}

	result := provider.getTargetAnnotations("test-key", annotations)

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result for invalid format, got %d entries", len(result))
	}
}

func TestGetTargetAnnotations_NotFound(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	annotations := map[string]string{
		"other-key": "key1=value1",
	}

	result := provider.getTargetAnnotations("test-key", annotations)

	if result != nil {
		t.Errorf("Expected nil result for non-existent key, got %v", result)
	}
}

func TestGetTargetAnnotations_MixedFormat(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	annotations := map[string]string{
		"test-key": "key1=value1,invalid,key2=value2",
	}

	result := provider.getTargetAnnotations("test-key", annotations)

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %s", result["key1"])
	}

	if result["key2"] != "value2" {
		t.Errorf("Expected key2=value2, got %s", result["key2"])
	}

	if _, exists := result["invalid"]; exists {
		t.Error("Expected invalid entry to be skipped")
	}
}

// Test isMixedCapacityLaunch method
func TestIsMixedCapacityLaunch_True(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Create nodeClaim with both spot and on-demand requirements
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeSpot, v1.CapacityTypeOnDemand},
			},
		},
	}

	// Create instance types with both spot and on-demand offerings
	spotInstance := createInstanceType("S3.MEDIUM4", 4, 8, 0.3, "ap-guangzhou-1", v1.CapacityTypeSpot)
	onDemandInstance := createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)

	instanceTypes := []*cloudprovider.InstanceType{spotInstance, onDemandInstance}

	result := provider.isMixedCapacityLaunch(nodeClaim, instanceTypes)

	if !result {
		t.Error("Expected isMixedCapacityLaunch to return true")
	}
}

func TestIsMixedCapacityLaunch_False_OnlySpot(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Create nodeClaim with only spot requirement
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeSpot},
			},
		},
	}

	spotInstance := createInstanceType("S3.MEDIUM4", 4, 8, 0.3, "ap-guangzhou-1", v1.CapacityTypeSpot)
	instanceTypes := []*cloudprovider.InstanceType{spotInstance}

	result := provider.isMixedCapacityLaunch(nodeClaim, instanceTypes)

	if result {
		t.Error("Expected isMixedCapacityLaunch to return false for spot-only")
	}
}

func TestIsMixedCapacityLaunch_False_OnlyOnDemand(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Create nodeClaim with only on-demand requirement
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeOnDemand},
			},
		},
	}

	onDemandInstance := createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	instanceTypes := []*cloudprovider.InstanceType{onDemandInstance}

	result := provider.isMixedCapacityLaunch(nodeClaim, instanceTypes)

	if result {
		t.Error("Expected isMixedCapacityLaunch to return false for on-demand-only")
	}
}

// Test filterUnwantedSpot function
func TestFilterUnwantedSpot_RemovesExpensiveSpot(t *testing.T) {
	// Create on-demand instance with price 1.0
	onDemandInstance := createInstanceType("S3.MEDIUM4", 4, 8, 1.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand)

	// Create spot instance with price higher than on-demand
	expensiveSpotInstance := createInstanceType("S3.LARGE8", 8, 16, 1.5, "ap-guangzhou-1", v1.CapacityTypeSpot)

	// Create spot instance with price lower than on-demand
	cheapSpotInstance := createInstanceType("S3.SMALL2", 2, 4, 0.5, "ap-guangzhou-1", v1.CapacityTypeSpot)

	instanceTypes := []*cloudprovider.InstanceType{onDemandInstance, expensiveSpotInstance, cheapSpotInstance}

	result := filterUnwantedSpot(instanceTypes)

	// Should keep on-demand and cheap spot, remove expensive spot
	if len(result) != 2 {
		t.Errorf("Expected 2 instance types after filtering, got %d", len(result))
	}

	hasExpensiveSpot := false
	for _, it := range result {
		if it.Name == "S3.LARGE8" {
			hasExpensiveSpot = true
		}
	}

	if hasExpensiveSpot {
		t.Error("Expected expensive spot instance to be filtered out")
	}
}

func TestFilterUnwantedSpot_KeepsAllWhenSpotCheaper(t *testing.T) {
	// Create on-demand instance with price 1.0
	onDemandInstance := createInstanceType("S3.MEDIUM4", 4, 8, 1.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand)

	// Create spot instances with prices lower than on-demand
	spotInstance1 := createInstanceType("S3.SMALL2", 2, 4, 0.5, "ap-guangzhou-1", v1.CapacityTypeSpot)
	spotInstance2 := createInstanceType("S3.LARGE8", 8, 16, 0.8, "ap-guangzhou-1", v1.CapacityTypeSpot)

	instanceTypes := []*cloudprovider.InstanceType{onDemandInstance, spotInstance1, spotInstance2}

	result := filterUnwantedSpot(instanceTypes)

	// Should keep all instances
	if len(result) != 3 {
		t.Errorf("Expected 3 instance types after filtering, got %d", len(result))
	}
}

// Test Create with minValues in requirements
func TestCreate_WithMinValues(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add minValues to requirements
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeOnDemand},
			},
			MinValues: lo.ToPtr(1),
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}
}

// Test filterInstanceTypes
func TestFilterInstanceTypes_MixedCapacity(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()
	mockZoneProvider := &mockZoneProvider{}
	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	// Create nodeClaim with both spot and on-demand requirements
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeSpot, v1.CapacityTypeOnDemand},
			},
		},
	}

	// Create instance types with both spot and on-demand offerings
	onDemandInstance := createInstanceType("S3.MEDIUM4", 4, 8, 1.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand)
	cheapSpotInstance := createInstanceType("S3.SMALL2", 2, 4, 0.5, "ap-guangzhou-1", v1.CapacityTypeSpot)
	expensiveSpotInstance := createInstanceType("S3.LARGE8", 8, 16, 1.5, "ap-guangzhou-1", v1.CapacityTypeSpot)

	instanceTypes := []*cloudprovider.InstanceType{onDemandInstance, cheapSpotInstance, expensiveSpotInstance}

	result := provider.filterInstanceTypes(nodeClaim, instanceTypes)

	// Should filter out expensive spot instance
	if len(result) > len(instanceTypes) {
		t.Errorf("Expected filtered result to have fewer or equal instances")
	}

	// Verify expensive spot is filtered out
	hasExpensiveSpot := false
	for _, it := range result {
		if it.Name == "S3.LARGE8" {
			hasExpensiveSpot = true
		}
	}

	if hasExpensiveSpot {
		t.Error("Expected expensive spot instance to be filtered out")
	}
}

// Test Create with InternetAccessible configuration
func TestCreate_WithInternetAccessible(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	// Add InternetAccessible configuration
	nodeClass.Spec.InternetAccessible = &api.InternetAccessible{
		MaxBandwidthOut:    lo.ToPtr(int32(100)),
		ChargeType:         lo.ToPtr(api.BandwidthPackage),
		BandwidthPackageID: lo.ToPtr("bwp-test"),
	}

	nodeClaim := createDefaultNodeClaim()

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.InternetAccessible == nil {
		t.Fatal("Expected InternetAccessible to be set")
	}

	if providerSpec.InternetAccessible.MaxBandwidthOut != 100 {
		t.Errorf("Expected MaxBandwidthOut 100, got %d", providerSpec.InternetAccessible.MaxBandwidthOut)
	}

	if providerSpec.InternetAccessible.ChargeType != capiv1beta1.BandwidthPackage {
		t.Errorf("Expected BandwidthPackage, got %s", providerSpec.InternetAccessible.ChargeType)
	}

	if providerSpec.InternetAccessible.BandwidthPackageID != "bwp-test" {
		t.Errorf("Expected BandwidthPackageID bwp-test, got %s", providerSpec.InternetAccessible.BandwidthPackageID)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created")
	}
}

// Test Create with various annotations
func TestCreate_WithAnnotations_KubeletArgs(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add kubelet args annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationKubeletArgPrefix + "max-pods":      "200",
		api.AnnotationKubeletArgPrefix + "kube-reserved": "cpu=100m,memory=100Mi",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.Management.KubeletArgs) == 0 {
		t.Fatal("Expected KubeletArgs to be set")
	}

	hasMaxPods := false
	for _, arg := range providerSpec.Management.KubeletArgs {
		if contains(arg, "max-pods=200") {
			hasMaxPods = true
		}
	}

	if !hasMaxPods {
		t.Error("Expected max-pods kubelet arg to be set")
	}
}

// Test Create with kernel args annotation
func TestCreate_WithAnnotations_KernelArgs(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add kernel args annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationKernelArgPrefix + "net.ipv4.ip_forward": "1",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.Management.KernelArgs) == 0 {
		t.Fatal("Expected KernelArgs to be set")
	}
}

// Test Create with hosts annotation
func TestCreate_WithAnnotations_Hosts(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add hosts annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationHostsPrefix + "192.168.1.1": "host1.example.com,host2.example.com",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.Management.Hosts) == 0 {
		t.Fatal("Expected Hosts to be set")
	}

	if providerSpec.Management.Hosts[0].IP != "192.168.1.1" {
		t.Errorf("Expected IP 192.168.1.1, got %s", providerSpec.Management.Hosts[0].IP)
	}
}

// Test Create with invalid hosts annotation (invalid IP)
func TestCreate_WithAnnotations_InvalidHosts(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add invalid hosts annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationHostsPrefix + "invalid-ip": "host1.example.com",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Invalid IP should be skipped
	if len(providerSpec.Management.Hosts) != 0 {
		t.Error("Expected invalid host to be skipped")
	}
}

// Test Create with nameservers annotation
func TestCreate_WithAnnotations_Nameservers(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add nameservers annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationNameserversKey: "8.8.8.8, 8.8.4.4",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.Management.Nameservers) == 0 {
		t.Fatal("Expected Nameservers to be set")
	}

	if len(providerSpec.Management.Nameservers) != 2 {
		t.Errorf("Expected 2 nameservers, got %d", len(providerSpec.Management.Nameservers))
	}
}

// Test Create with hostname annotation
func TestCreate_WithAnnotations_Hostname(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add hostname annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationHostnameKey: "custom-hostname",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if providerSpec.HostName != "custom-hostname" {
		t.Errorf("Expected hostname custom-hostname, got %s", providerSpec.HostName)
	}
}

// Test Create with runtime root annotation
func TestCreate_WithAnnotations_RuntimeRoot(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add runtime root annotation
	nodeClaim.Annotations = map[string]string{
		api.AnnotationRuntimeRootKey: "/var/lib/containerd",
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, _, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine.Spec.RuntimeRootDir != "/var/lib/containerd" {
		t.Errorf("Expected RuntimeRootDir /var/lib/containerd, got %s", machine.Spec.RuntimeRootDir)
	}
}

// Test Create with data disk annotations
func TestCreate_WithDataDiskAnnotations(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	// Add data disks
	nodeClass.Spec.DataDisks = []api.DataDisk{
		{
			Type:        "CLOUD_PREMIUM",
			Size:        100,
			FileSystem:  lo.ToPtr(api.FileSystemEXT4),
			MountTarget: lo.ToPtr("/data"),
		},
	}
	// Add data disk annotations
	nodeClass.Annotations = map[string]string{
		api.AnnotationDataDisksThroughputKey: "0=100",
		api.AnnotationDataDisksEncryptKey:    "0=ENCRYPT",
		api.AnnotationDataDisksKMSID:         "0=kms-123",
		api.AnnotationDataDisksSnapshotID:    "0=snap-123",
		api.AnnotationDataDisksImageCacheID:  "0=cache-123",
	}

	nodeClaim := createDefaultNodeClaim()

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	_, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(providerSpec.DataDisks) == 0 {
		t.Fatal("Expected DataDisks to be set")
	}

	disk := providerSpec.DataDisks[0]
	if disk.ThroughputPerformance != 100 {
		t.Errorf("Expected ThroughputPerformance 100, got %d", disk.ThroughputPerformance)
	}

	if disk.Encrypt != "ENCRYPT" {
		t.Errorf("Expected Encrypt ENCRYPT, got %s", disk.Encrypt)
	}

	if disk.KmsKeyId != "kms-123" {
		t.Errorf("Expected KmsKeyId kms-123, got %s", disk.KmsKeyId)
	}

	if disk.SnapshotId != "snap-123" {
		t.Errorf("Expected SnapshotId snap-123, got %s", disk.SnapshotId)
	}

	if disk.ImageCacheId != "cache-123" {
		t.Errorf("Expected ImageCacheId cache-123, got %s", disk.ImageCacheId)
	}

	if disk.FileSystem != string(api.FileSystemEXT4) {
		t.Errorf("Expected FileSystem ext4, got %s", disk.FileSystem)
	}

	if !disk.AutoFormatAndMount {
		t.Error("Expected AutoFormatAndMount to be true")
	}

	if disk.MountTarget != "/data" {
		t.Errorf("Expected MountTarget /data, got %s", disk.MountTarget)
	}
}

// Helper function to create ARM architecture instance type
func createARMInstanceType(name string, cpu int64, memory int64, price float64, zone string, capacityType string) *cloudprovider.InstanceType {
	requirements := scheduling.NewRequirements(
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "arm64"),
		scheduling.NewRequirement(api.LabelInstanceFamily, corev1.NodeSelectorOpIn, "SR1"),
		scheduling.NewRequirement(api.LabelInstanceCPU, corev1.NodeSelectorOpIn, fmt.Sprintf("%d", cpu)),
		scheduling.NewRequirement(api.LabelInstanceMemoryGB, corev1.NodeSelectorOpIn, fmt.Sprintf("%d", memory)),
	)

	capacity := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse(fmt.Sprintf("%d", cpu)),
		corev1.ResourceMemory:           resource.MustParse(fmt.Sprintf("%dGi", memory)),
		corev1.ResourcePods:             resource.MustParse("110"),
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}

	overhead := &cloudprovider.InstanceTypeOverhead{
		KubeReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		},
		SystemReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		},
		EvictionThreshold: corev1.ResourceList{
			corev1.ResourceMemory:           resource.MustParse("100Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
		},
	}

	offering := &cloudprovider.Offering{
		Requirements: requirements,
		Price:        price,
		Available:    true,
	}

	return &cloudprovider.InstanceType{
		Name:         name,
		Requirements: requirements,
		Capacity:     capacity,
		Overhead:     overhead,
		Offerings:    []*cloudprovider.Offering{offering},
	}
}

// Test Create with ARM architecture instance type
func TestCreate_ARMArchitecture(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add ARM architecture requirement
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"arm64"},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createARMInstanceType("SR1.MEDIUM4", 4, 8, 0.4, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine == nil {
		t.Fatal("Expected machine to be created, got nil")
	}

	if providerSpec.InstanceType != "SR1.MEDIUM4" {
		t.Errorf("Expected instance type SR1.MEDIUM4, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}

	if machine.Labels[api.LabelInstanceFamily] != "SR1" {
		t.Errorf("Expected instance family SR1, got %s", machine.Labels[api.LabelInstanceFamily])
	}

	if machine.Labels[api.LabelInstanceCPU] != "4" {
		t.Errorf("Expected CPU label 4, got %s", machine.Labels[api.LabelInstanceCPU])
	}

	if machine.Labels[api.LabelInstanceMemoryGB] != "8" {
		t.Errorf("Expected Memory label 8, got %s", machine.Labels[api.LabelInstanceMemoryGB])
	}
}

// Test Create with multiple ARM instance types
func TestCreate_MultipleARMInstanceTypes(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add ARM architecture requirement
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"arm64"},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createARMInstanceType("SR1.LARGE8", 8, 16, 0.8, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
		createARMInstanceType("SR1.MEDIUM4", 4, 8, 0.4, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
		createARMInstanceType("SR1.XLARGE16", 16, 32, 1.6, "ap-guangzhou-1", v1.CapacityTypeOnDemand),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should select the cheapest ARM instance type
	if providerSpec.InstanceType != "SR1.MEDIUM4" {
		t.Errorf("Expected cheapest instance type SR1.MEDIUM4, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}
}

// Test Create with mixed ARM and x86 instance types
func TestCreate_MixedArchitectures(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Allow both architectures
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"amd64", "arm64"},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createInstanceType("S3.MEDIUM4", 4, 8, 0.5, "ap-guangzhou-1", v1.CapacityTypeOnDemand),     // x86
		createARMInstanceType("SR1.MEDIUM4", 4, 8, 0.4, "ap-guangzhou-1", v1.CapacityTypeOnDemand), // ARM, cheaper
		createInstanceType("S3.LARGE8", 8, 16, 1.0, "ap-guangzhou-1", v1.CapacityTypeOnDemand),     // x86
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should select the cheapest instance type (ARM)
	if providerSpec.InstanceType != "SR1.MEDIUM4" {
		t.Errorf("Expected cheapest instance type SR1.MEDIUM4, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}
}

// Test Create with ARM Spot instances
func TestCreate_ARMSpotInstances(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add ARM architecture and Spot capacity type requirements
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"arm64"},
			},
		},
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      v1.CapacityTypeLabelKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v1.CapacityTypeSpot},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createARMInstanceType("SR1.MEDIUM4", 4, 8, 0.2, "ap-guangzhou-1", v1.CapacityTypeSpot),
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if machine.Spec.ProviderSpec.Type != capiv1beta1.MachineTypeNativeCVM {
		t.Errorf("Expected MachineType %s, got %s", capiv1beta1.MachineTypeNativeCVM, machine.Spec.ProviderSpec.Type)
	}

	if providerSpec.InstanceChargeType != capiv1beta1.SpotpaidChargeType {
		t.Errorf("Expected InstanceChargeType %s, got %s", capiv1beta1.SpotpaidChargeType, providerSpec.InstanceChargeType)
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}

	if machine.Labels[v1.CapacityTypeLabelKey] != v1.CapacityTypeSpot {
		t.Errorf("Expected CapacityType label %s, got %s", v1.CapacityTypeSpot, machine.Labels[v1.CapacityTypeLabelKey])
	}
}

// Test Create with ARM different capacity types
func TestCreate_ARMDifferentCapacityTypes(t *testing.T) {
	tests := []struct {
		name           string
		capacityType   string
		expectedType   capiv1beta1.MachineType
		expectedCharge capiv1beta1.InstanceChargeType
	}{
		{
			name:           "ARM OnDemand",
			capacityType:   v1.CapacityTypeOnDemand,
			expectedType:   capiv1beta1.MachineTypeNative,
			expectedCharge: capiv1beta1.PostpaidByHourChargeType,
		},
		{
			name:           "ARM Spot",
			capacityType:   v1.CapacityTypeSpot,
			expectedType:   capiv1beta1.MachineTypeNativeCVM,
			expectedCharge: capiv1beta1.SpotpaidChargeType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := createScheme()
			ctx := context.Background()

			nodeClass := createDefaultNodeClass()
			nodeClaim := createDefaultNodeClaim()
			nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
				{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      corev1.LabelArchStable,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"arm64"},
					},
				},
				{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      v1.CapacityTypeLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{tt.capacityType},
					},
				},
			}

			instanceTypes := []*cloudprovider.InstanceType{
				createARMInstanceType("SR1.MEDIUM4", 4, 8, 0.4, "ap-guangzhou-1", tt.capacityType),
			}

			mockZoneProvider := &mockZoneProvider{}
			fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

			provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

			machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if machine.Spec.ProviderSpec.Type != tt.expectedType {
				t.Errorf("Expected MachineType %s, got %s", tt.expectedType, machine.Spec.ProviderSpec.Type)
			}

			if providerSpec.InstanceChargeType != tt.expectedCharge {
				t.Errorf("Expected InstanceChargeType %s, got %s", tt.expectedCharge, providerSpec.InstanceChargeType)
			}

			if machine.Labels[v1.CapacityTypeLabelKey] != tt.capacityType {
				t.Errorf("Expected CapacityType label %s, got %s", tt.capacityType, machine.Labels[v1.CapacityTypeLabelKey])
			}

			if machine.Labels[corev1.LabelArchStable] != "arm64" {
				t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
			}
		})
	}
}

// Test Create with ARM resource boundary values
func TestCreate_ARMResourceBoundaryValues(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add ARM architecture requirement
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"arm64"},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createARMInstanceType("SR1.SMALL2", 2, 4, 0.2, "ap-guangzhou-1", v1.CapacityTypeOnDemand),     // Small
		createARMInstanceType("SR1.XLARGE32", 32, 64, 3.2, "ap-guangzhou-1", v1.CapacityTypeOnDemand), // Large
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should select the cheapest ARM instance type
	if providerSpec.InstanceType != "SR1.SMALL2" {
		t.Errorf("Expected cheapest instance type SR1.SMALL2, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[api.LabelInstanceCPU] != "2" {
		t.Errorf("Expected CPU label 2, got %s", machine.Labels[api.LabelInstanceCPU])
	}

	if machine.Labels[api.LabelInstanceMemoryGB] != "4" {
		t.Errorf("Expected Memory label 4, got %s", machine.Labels[api.LabelInstanceMemoryGB])
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}

	if machine.Annotations[api.AnnotationUnitPrice] == "" {
		t.Error("Expected unit price annotation to be set")
	}
}

// Test Create with ARM and specific resource requirements
func TestCreate_ARMWithResourceRequirements(t *testing.T) {
	scheme := createScheme()
	ctx := context.Background()

	nodeClass := createDefaultNodeClass()
	nodeClaim := createDefaultNodeClaim()

	// Add ARM architecture and resource requirements
	nodeClaim.Spec.Requirements = []v1.NodeSelectorRequirementWithMinValues{
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      corev1.LabelArchStable,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"arm64"},
			},
		},
		{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      api.LabelInstanceCPU,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"8", "16"},
			},
		},
	}

	instanceTypes := []*cloudprovider.InstanceType{
		createARMInstanceType("SR1.SMALL2", 2, 4, 0.2, "ap-guangzhou-1", v1.CapacityTypeOnDemand),     // Should be filtered out
		createARMInstanceType("SR1.LARGE8", 8, 16, 0.8, "ap-guangzhou-1", v1.CapacityTypeOnDemand),    // Matches
		createARMInstanceType("SR1.XLARGE16", 16, 32, 1.6, "ap-guangzhou-1", v1.CapacityTypeOnDemand), // Matches
	}

	mockZoneProvider := &mockZoneProvider{}
	fakeClient := createFakeClient(scheme, nodeClass, nodeClaim)

	provider := NewDefaultProvider(ctx, fakeClient, mockZoneProvider, "test-cluster")

	machine, providerSpec, err := provider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should select SR1.LARGE8 as it's cheaper and matches requirements
	if providerSpec.InstanceType != "SR1.LARGE8" {
		t.Errorf("Expected instance type SR1.LARGE8, got %s", providerSpec.InstanceType)
	}

	if machine.Labels[api.LabelInstanceCPU] != "8" {
		t.Errorf("Expected CPU label 8, got %s", machine.Labels[api.LabelInstanceCPU])
	}

	if machine.Labels[corev1.LabelArchStable] != "arm64" {
		t.Errorf("Expected architecture label arm64, got %s", machine.Labels[corev1.LabelArchStable])
	}
}
