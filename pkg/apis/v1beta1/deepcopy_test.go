package v1beta1

import (
	"testing"

	op "github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 1. DataDisk
// ---------------------------------------------------------------------------

func TestDataDisk_DeepCopy(t *testing.T) {
	fs := FileSystemEXT4
	mount := "/data"
	original := &DataDisk{
		Size:        100,
		Type:        DiskTypeCloudPremium,
		MountTarget: &mount,
		FileSystem:  &fs,
	}

	copied := original.DeepCopy()

	// Verify values are equal
	if copied.Size != original.Size {
		t.Errorf("expected Size %d, got %d", original.Size, copied.Size)
	}
	if copied.Type != original.Type {
		t.Errorf("expected Type %s, got %s", original.Type, copied.Type)
	}
	if *copied.MountTarget != *original.MountTarget {
		t.Errorf("expected MountTarget %s, got %s", *original.MountTarget, *copied.MountTarget)
	}
	if *copied.FileSystem != *original.FileSystem {
		t.Errorf("expected FileSystem %s, got %s", *original.FileSystem, *copied.FileSystem)
	}

	// Verify deep independence: mutating copied pointer fields must not affect original
	*copied.MountTarget = "/other"
	if *original.MountTarget != "/data" {
		t.Error("modifying copied MountTarget affected original")
	}

	*copied.FileSystem = FileSystemXFS
	if *original.FileSystem != FileSystemEXT4 {
		t.Error("modifying copied FileSystem affected original")
	}

	// nil receiver
	var nilDisk *DataDisk
	if nilDisk.DeepCopy() != nil {
		t.Error("DeepCopy of nil DataDisk should return nil")
	}
}

func TestDataDisk_DeepCopyInto(t *testing.T) {
	fs := FileSystemEXT4
	mount := "/data"
	original := DataDisk{
		Size:        200,
		Type:        DiskTypeCloudSSD,
		MountTarget: &mount,
		FileSystem:  &fs,
	}

	var out DataDisk
	original.DeepCopyInto(&out)

	if out.Size != original.Size {
		t.Errorf("expected Size %d, got %d", original.Size, out.Size)
	}
	if out.MountTarget == original.MountTarget {
		t.Error("DeepCopyInto should allocate new pointer for MountTarget")
	}
	if *out.MountTarget != *original.MountTarget {
		t.Errorf("expected MountTarget value %s, got %s", *original.MountTarget, *out.MountTarget)
	}
	if out.FileSystem == original.FileSystem {
		t.Error("DeepCopyInto should allocate new pointer for FileSystem")
	}
}

func TestDataDisk_DeepCopy_NilPointers(t *testing.T) {
	original := &DataDisk{
		Size: 50,
		Type: DiskTypeCloudPremium,
	}

	copied := original.DeepCopy()
	if copied.MountTarget != nil {
		t.Error("expected nil MountTarget in copy")
	}
	if copied.FileSystem != nil {
		t.Error("expected nil FileSystem in copy")
	}
}

// ---------------------------------------------------------------------------
// 2. InternetAccessible
// ---------------------------------------------------------------------------

func TestInternetAccessible_DeepCopy(t *testing.T) {
	ct := TrafficPostpaidByHour
	bwpID := "bwp-abc123"
	original := &InternetAccessible{
		MaxBandwidthOut:    lo.ToPtr(int32(50)),
		ChargeType:         &ct,
		BandwidthPackageID: &bwpID,
	}

	copied := original.DeepCopy()

	if *copied.MaxBandwidthOut != 50 {
		t.Errorf("expected MaxBandwidthOut 50, got %d", *copied.MaxBandwidthOut)
	}
	if *copied.ChargeType != TrafficPostpaidByHour {
		t.Errorf("expected ChargeType TrafficPostpaidByHour, got %s", *copied.ChargeType)
	}
	if *copied.BandwidthPackageID != "bwp-abc123" {
		t.Errorf("expected BandwidthPackageID bwp-abc123, got %s", *copied.BandwidthPackageID)
	}

	// Verify deep independence
	*copied.MaxBandwidthOut = 100
	if *original.MaxBandwidthOut != 50 {
		t.Error("modifying copied MaxBandwidthOut affected original")
	}
	*copied.ChargeType = BandwidthPackage
	if *original.ChargeType != TrafficPostpaidByHour {
		t.Error("modifying copied ChargeType affected original")
	}
	*copied.BandwidthPackageID = "bwp-changed"
	if *original.BandwidthPackageID != "bwp-abc123" {
		t.Error("modifying copied BandwidthPackageID affected original")
	}

	// nil receiver
	var nilIA *InternetAccessible
	if nilIA.DeepCopy() != nil {
		t.Error("DeepCopy of nil InternetAccessible should return nil")
	}
}

func TestInternetAccessible_DeepCopyInto(t *testing.T) {
	ct := BandwidthPackage
	original := InternetAccessible{
		MaxBandwidthOut: lo.ToPtr(int32(10)),
		ChargeType:      &ct,
	}

	var out InternetAccessible
	original.DeepCopyInto(&out)

	if out.MaxBandwidthOut == original.MaxBandwidthOut {
		t.Error("DeepCopyInto should allocate new pointer for MaxBandwidthOut")
	}
	if *out.MaxBandwidthOut != *original.MaxBandwidthOut {
		t.Errorf("expected MaxBandwidthOut %d, got %d", *original.MaxBandwidthOut, *out.MaxBandwidthOut)
	}
	if out.BandwidthPackageID != nil {
		t.Error("expected nil BandwidthPackageID in copy")
	}
}

// ---------------------------------------------------------------------------
// 3. LifecycleScript
// ---------------------------------------------------------------------------

func TestLifecycleScript_DeepCopy(t *testing.T) {
	original := &LifecycleScript{
		PreInitScript:  lo.ToPtr("echo pre"),
		PostInitScript: lo.ToPtr("echo post"),
	}

	copied := original.DeepCopy()

	if *copied.PreInitScript != "echo pre" {
		t.Errorf("expected PreInitScript 'echo pre', got %s", *copied.PreInitScript)
	}
	if *copied.PostInitScript != "echo post" {
		t.Errorf("expected PostInitScript 'echo post', got %s", *copied.PostInitScript)
	}

	// Verify deep independence
	*copied.PreInitScript = "echo changed"
	if *original.PreInitScript != "echo pre" {
		t.Error("modifying copied PreInitScript affected original")
	}
	*copied.PostInitScript = "echo other"
	if *original.PostInitScript != "echo post" {
		t.Error("modifying copied PostInitScript affected original")
	}

	// nil receiver
	var nilLS *LifecycleScript
	if nilLS.DeepCopy() != nil {
		t.Error("DeepCopy of nil LifecycleScript should return nil")
	}
}

func TestLifecycleScript_DeepCopyInto(t *testing.T) {
	original := LifecycleScript{
		PreInitScript: lo.ToPtr("setup"),
	}

	var out LifecycleScript
	original.DeepCopyInto(&out)

	if out.PreInitScript == original.PreInitScript {
		t.Error("DeepCopyInto should allocate new pointer for PreInitScript")
	}
	if *out.PreInitScript != "setup" {
		t.Errorf("expected PreInitScript 'setup', got %s", *out.PreInitScript)
	}
	if out.PostInitScript != nil {
		t.Error("expected nil PostInitScript in copy")
	}
}

// ---------------------------------------------------------------------------
// 4. SSHKey (no pointer fields)
// ---------------------------------------------------------------------------

func TestSSHKey_DeepCopy(t *testing.T) {
	original := &SSHKey{ID: "skey-abc123"}

	copied := original.DeepCopy()

	if copied.ID != "skey-abc123" {
		t.Errorf("expected ID skey-abc123, got %s", copied.ID)
	}

	// Mutating the copy should not affect original
	copied.ID = "skey-changed"
	if original.ID != "skey-abc123" {
		t.Error("modifying copied ID affected original")
	}

	// nil receiver
	var nilKey *SSHKey
	if nilKey.DeepCopy() != nil {
		t.Error("DeepCopy of nil SSHKey should return nil")
	}
}

func TestSSHKey_DeepCopyInto(t *testing.T) {
	original := SSHKey{ID: "skey-xyz"}
	var out SSHKey
	original.DeepCopyInto(&out)

	if out.ID != "skey-xyz" {
		t.Errorf("expected ID skey-xyz, got %s", out.ID)
	}
}

// ---------------------------------------------------------------------------
// 5. SSHKeySelectorTerm (Tags map)
// ---------------------------------------------------------------------------

func TestSSHKeySelectorTerm_DeepCopy(t *testing.T) {
	original := &SSHKeySelectorTerm{
		Tags: map[string]string{"env": "prod", "team": "infra"},
		ID:   "skey-sel1",
	}

	copied := original.DeepCopy()

	if copied.ID != "skey-sel1" {
		t.Errorf("expected ID skey-sel1, got %s", copied.ID)
	}
	if len(copied.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(copied.Tags))
	}
	if copied.Tags["env"] != "prod" {
		t.Errorf("expected tag env=prod, got %s", copied.Tags["env"])
	}

	// Verify deep independence of map
	copied.Tags["env"] = "staging"
	if original.Tags["env"] != "prod" {
		t.Error("modifying copied Tags affected original")
	}
	copied.Tags["new"] = "value"
	if _, ok := original.Tags["new"]; ok {
		t.Error("adding key to copied Tags affected original")
	}

	// nil receiver
	var nilTerm *SSHKeySelectorTerm
	if nilTerm.DeepCopy() != nil {
		t.Error("DeepCopy of nil SSHKeySelectorTerm should return nil")
	}
}

func TestSSHKeySelectorTerm_DeepCopy_NilTags(t *testing.T) {
	original := &SSHKeySelectorTerm{ID: "skey-notags"}

	copied := original.DeepCopy()
	if copied.Tags != nil {
		t.Error("expected nil Tags in copy when original has nil Tags")
	}
}

func TestSSHKeySelectorTerm_DeepCopyInto(t *testing.T) {
	original := SSHKeySelectorTerm{
		Tags: map[string]string{"k": "v"},
	}
	var out SSHKeySelectorTerm
	original.DeepCopyInto(&out)

	if out.Tags["k"] != "v" {
		t.Errorf("expected tag k=v, got %s", out.Tags["k"])
	}
}

// ---------------------------------------------------------------------------
// 6. SecurityGroup (no pointer fields)
// ---------------------------------------------------------------------------

func TestSecurityGroup_DeepCopy(t *testing.T) {
	original := &SecurityGroup{ID: "sg-abc123"}

	copied := original.DeepCopy()

	if copied.ID != "sg-abc123" {
		t.Errorf("expected ID sg-abc123, got %s", copied.ID)
	}

	copied.ID = "sg-changed"
	if original.ID != "sg-abc123" {
		t.Error("modifying copied ID affected original")
	}

	// nil receiver
	var nilSG *SecurityGroup
	if nilSG.DeepCopy() != nil {
		t.Error("DeepCopy of nil SecurityGroup should return nil")
	}
}

func TestSecurityGroup_DeepCopyInto(t *testing.T) {
	original := SecurityGroup{ID: "sg-xyz"}
	var out SecurityGroup
	original.DeepCopyInto(&out)

	if out.ID != "sg-xyz" {
		t.Errorf("expected ID sg-xyz, got %s", out.ID)
	}
}

// ---------------------------------------------------------------------------
// 7. SecurityGroupSelectorTerm (Tags map)
// ---------------------------------------------------------------------------

func TestSecurityGroupSelectorTerm_DeepCopy(t *testing.T) {
	original := &SecurityGroupSelectorTerm{
		Tags: map[string]string{"role": "web"},
		ID:   "sg-sel1",
	}

	copied := original.DeepCopy()

	if copied.ID != "sg-sel1" {
		t.Errorf("expected ID sg-sel1, got %s", copied.ID)
	}
	if copied.Tags["role"] != "web" {
		t.Errorf("expected tag role=web, got %s", copied.Tags["role"])
	}

	// Verify deep independence
	copied.Tags["role"] = "api"
	if original.Tags["role"] != "web" {
		t.Error("modifying copied Tags affected original")
	}

	// nil receiver
	var nilTerm *SecurityGroupSelectorTerm
	if nilTerm.DeepCopy() != nil {
		t.Error("DeepCopy of nil SecurityGroupSelectorTerm should return nil")
	}
}

func TestSecurityGroupSelectorTerm_DeepCopy_NilTags(t *testing.T) {
	original := &SecurityGroupSelectorTerm{ID: "sg-notags"}
	copied := original.DeepCopy()
	if copied.Tags != nil {
		t.Error("expected nil Tags in copy when original has nil Tags")
	}
}

func TestSecurityGroupSelectorTerm_DeepCopyInto(t *testing.T) {
	original := SecurityGroupSelectorTerm{
		Tags: map[string]string{"a": "b"},
	}
	var out SecurityGroupSelectorTerm
	original.DeepCopyInto(&out)

	if out.Tags["a"] != "b" {
		t.Errorf("expected tag a=b, got %s", out.Tags["a"])
	}
}

// ---------------------------------------------------------------------------
// 8. Subnet (no pointer fields)
// ---------------------------------------------------------------------------

func TestSubnet_DeepCopy(t *testing.T) {
	original := &Subnet{ID: "subnet-abc123", Zone: "ap-guangzhou-3", ZoneID: "100003"}

	copied := original.DeepCopy()

	if copied.ID != "subnet-abc123" {
		t.Errorf("expected ID subnet-abc123, got %s", copied.ID)
	}
	if copied.Zone != "ap-guangzhou-3" {
		t.Errorf("expected Zone ap-guangzhou-3, got %s", copied.Zone)
	}
	if copied.ZoneID != "100003" {
		t.Errorf("expected ZoneID 100003, got %s", copied.ZoneID)
	}

	copied.ID = "subnet-changed"
	if original.ID != "subnet-abc123" {
		t.Error("modifying copied ID affected original")
	}

	// nil receiver
	var nilSubnet *Subnet
	if nilSubnet.DeepCopy() != nil {
		t.Error("DeepCopy of nil Subnet should return nil")
	}
}

func TestSubnet_DeepCopyInto(t *testing.T) {
	original := Subnet{ID: "subnet-xyz", Zone: "ap-beijing-1"}
	var out Subnet
	original.DeepCopyInto(&out)

	if out.ID != "subnet-xyz" {
		t.Errorf("expected ID subnet-xyz, got %s", out.ID)
	}
	if out.Zone != "ap-beijing-1" {
		t.Errorf("expected Zone ap-beijing-1, got %s", out.Zone)
	}
}

// ---------------------------------------------------------------------------
// 9. SubnetSelectorTerm (Tags map)
// ---------------------------------------------------------------------------

func TestSubnetSelectorTerm_DeepCopy(t *testing.T) {
	original := &SubnetSelectorTerm{
		Tags: map[string]string{"zone": "*", "env": "prod"},
		ID:   "subnet-sel1",
	}

	copied := original.DeepCopy()

	if copied.ID != "subnet-sel1" {
		t.Errorf("expected ID subnet-sel1, got %s", copied.ID)
	}
	if len(copied.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(copied.Tags))
	}

	// Verify deep independence
	copied.Tags["zone"] = "ap-guangzhou-3"
	if original.Tags["zone"] != "*" {
		t.Error("modifying copied Tags affected original")
	}

	// nil receiver
	var nilTerm *SubnetSelectorTerm
	if nilTerm.DeepCopy() != nil {
		t.Error("DeepCopy of nil SubnetSelectorTerm should return nil")
	}
}

func TestSubnetSelectorTerm_DeepCopy_NilTags(t *testing.T) {
	original := &SubnetSelectorTerm{ID: "subnet-notags"}
	copied := original.DeepCopy()
	if copied.Tags != nil {
		t.Error("expected nil Tags in copy when original has nil Tags")
	}
}

func TestSubnetSelectorTerm_DeepCopyInto(t *testing.T) {
	original := SubnetSelectorTerm{
		Tags: map[string]string{"x": "y"},
	}
	var out SubnetSelectorTerm
	original.DeepCopyInto(&out)

	if out.Tags["x"] != "y" {
		t.Errorf("expected tag x=y, got %s", out.Tags["x"])
	}
}

// ---------------------------------------------------------------------------
// 10. SystemDisk (no pointer fields)
// ---------------------------------------------------------------------------

func TestSystemDisk_DeepCopy(t *testing.T) {
	original := &SystemDisk{Size: 50, Type: DiskTypeCloudPremium}

	copied := original.DeepCopy()

	if copied.Size != 50 {
		t.Errorf("expected Size 50, got %d", copied.Size)
	}
	if copied.Type != DiskTypeCloudPremium {
		t.Errorf("expected Type CloudPremium, got %s", copied.Type)
	}

	copied.Size = 100
	if original.Size != 50 {
		t.Error("modifying copied Size affected original")
	}

	// nil receiver
	var nilDisk *SystemDisk
	if nilDisk.DeepCopy() != nil {
		t.Error("DeepCopy of nil SystemDisk should return nil")
	}
}

func TestSystemDisk_DeepCopyInto(t *testing.T) {
	original := SystemDisk{Size: 80, Type: DiskTypeCloudSSD}
	var out SystemDisk
	original.DeepCopyInto(&out)

	if out.Size != 80 {
		t.Errorf("expected Size 80, got %d", out.Size)
	}
	if out.Type != DiskTypeCloudSSD {
		t.Errorf("expected Type CloudSSD, got %s", out.Type)
	}
}

// ---------------------------------------------------------------------------
// 11. TKEMachineNodeClass
// ---------------------------------------------------------------------------

func TestTKEMachineNodeClass_DeepCopy(t *testing.T) {
	original := &TKEMachineNodeClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TKEMachineNodeClass",
			APIVersion: "infrastructure.karpenter.tke/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-nodeclass",
			Labels: map[string]string{"app": "test"},
		},
		Spec: TKEMachineNodeClassSpec{
			SubnetSelectorTerms: []SubnetSelectorTerm{
				{ID: "subnet-abc123"},
			},
			SecurityGroupSelectorTerms: []SecurityGroupSelectorTerm{
				{Tags: map[string]string{"sg": "default"}},
			},
			SystemDisk: &SystemDisk{Size: 50, Type: DiskTypeCloudPremium},
			Tags:       map[string]string{"env": "test"},
		},
		Status: TKEMachineNodeClassStatus{
			Subnets:        []Subnet{{ID: "subnet-abc123", Zone: "ap-guangzhou-3"}},
			SecurityGroups: []SecurityGroup{{ID: "sg-abc123"}},
		},
	}

	copied := original.DeepCopy()

	// Verify basic fields
	if copied.Name != "test-nodeclass" {
		t.Errorf("expected Name test-nodeclass, got %s", copied.Name)
	}
	if copied.Kind != "TKEMachineNodeClass" {
		t.Errorf("expected Kind TKEMachineNodeClass, got %s", copied.Kind)
	}

	// Verify deep independence of spec
	copied.Spec.Tags["env"] = "prod"
	if original.Spec.Tags["env"] != "test" {
		t.Error("modifying copied Spec.Tags affected original")
	}

	copied.Spec.SubnetSelectorTerms[0].ID = "subnet-changed"
	if original.Spec.SubnetSelectorTerms[0].ID != "subnet-abc123" {
		t.Error("modifying copied SubnetSelectorTerms affected original")
	}

	copied.Spec.SystemDisk.Size = 200
	if original.Spec.SystemDisk.Size != 50 {
		t.Error("modifying copied SystemDisk affected original")
	}

	// Verify deep independence of status
	copied.Status.Subnets[0].ID = "subnet-changed"
	if original.Status.Subnets[0].ID != "subnet-abc123" {
		t.Error("modifying copied Status.Subnets affected original")
	}

	// Verify deep independence of ObjectMeta labels
	copied.Labels["app"] = "changed"
	if original.Labels["app"] != "test" {
		t.Error("modifying copied Labels affected original")
	}

	// nil receiver
	var nilNC *TKEMachineNodeClass
	if nilNC.DeepCopy() != nil {
		t.Error("DeepCopy of nil TKEMachineNodeClass should return nil")
	}
}

func TestTKEMachineNodeClass_DeepCopyInto(t *testing.T) {
	original := TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "into-test",
		},
		Spec: TKEMachineNodeClassSpec{
			Tags: map[string]string{"k": "v"},
		},
	}

	var out TKEMachineNodeClass
	original.DeepCopyInto(&out)

	if out.Name != "into-test" {
		t.Errorf("expected Name into-test, got %s", out.Name)
	}
	if out.Spec.Tags["k"] != "v" {
		t.Errorf("expected tag k=v, got %s", out.Spec.Tags["k"])
	}
}

func TestTKEMachineNodeClass_DeepCopyObject(t *testing.T) {
	original := &TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "object-test",
		},
	}

	obj := original.DeepCopyObject()
	if obj == nil {
		t.Fatal("DeepCopyObject returned nil")
	}

	// Verify it returns runtime.Object
	var _ runtime.Object = obj

	nc, ok := obj.(*TKEMachineNodeClass)
	if !ok {
		t.Fatal("DeepCopyObject did not return *TKEMachineNodeClass")
	}
	if nc.Name != "object-test" {
		t.Errorf("expected Name object-test, got %s", nc.Name)
	}

	// nil receiver returns nil
	var nilNC *TKEMachineNodeClass
	if nilNC.DeepCopyObject() != nil {
		t.Error("DeepCopyObject of nil TKEMachineNodeClass should return nil")
	}
}

// ---------------------------------------------------------------------------
// 12. TKEMachineNodeClassList
// ---------------------------------------------------------------------------

func TestTKEMachineNodeClassList_DeepCopy(t *testing.T) {
	original := &TKEMachineNodeClassList{
		TypeMeta: metav1.TypeMeta{
			Kind: "TKEMachineNodeClassList",
		},
		Items: []TKEMachineNodeClass{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "item-1"},
				Spec: TKEMachineNodeClassSpec{
					Tags: map[string]string{"idx": "1"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "item-2"},
			},
		},
	}

	copied := original.DeepCopy()

	if len(copied.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(copied.Items))
	}
	if copied.Items[0].Name != "item-1" {
		t.Errorf("expected first item name item-1, got %s", copied.Items[0].Name)
	}
	if copied.Items[1].Name != "item-2" {
		t.Errorf("expected second item name item-2, got %s", copied.Items[1].Name)
	}

	// Verify deep independence of items
	copied.Items[0].Spec.Tags["idx"] = "changed"
	if original.Items[0].Spec.Tags["idx"] != "1" {
		t.Error("modifying copied Items[0].Spec.Tags affected original")
	}

	copied.Items[0].Name = "item-changed"
	if original.Items[0].Name != "item-1" {
		t.Error("modifying copied Items[0].Name affected original")
	}

	// nil receiver
	var nilList *TKEMachineNodeClassList
	if nilList.DeepCopy() != nil {
		t.Error("DeepCopy of nil TKEMachineNodeClassList should return nil")
	}
}

func TestTKEMachineNodeClassList_DeepCopy_NilItems(t *testing.T) {
	original := &TKEMachineNodeClassList{
		TypeMeta: metav1.TypeMeta{Kind: "TKEMachineNodeClassList"},
	}

	copied := original.DeepCopy()
	if copied.Items != nil {
		t.Error("expected nil Items in copy when original has nil Items")
	}
}

func TestTKEMachineNodeClassList_DeepCopyInto(t *testing.T) {
	original := TKEMachineNodeClassList{
		Items: []TKEMachineNodeClass{
			{ObjectMeta: metav1.ObjectMeta{Name: "into-item"}},
		},
	}

	var out TKEMachineNodeClassList
	original.DeepCopyInto(&out)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	if out.Items[0].Name != "into-item" {
		t.Errorf("expected item name into-item, got %s", out.Items[0].Name)
	}
}

func TestTKEMachineNodeClassList_DeepCopyObject(t *testing.T) {
	original := &TKEMachineNodeClassList{
		Items: []TKEMachineNodeClass{
			{ObjectMeta: metav1.ObjectMeta{Name: "obj-item"}},
		},
	}

	obj := original.DeepCopyObject()
	if obj == nil {
		t.Fatal("DeepCopyObject returned nil")
	}

	var _ runtime.Object = obj

	list, ok := obj.(*TKEMachineNodeClassList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *TKEMachineNodeClassList")
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}
	if list.Items[0].Name != "obj-item" {
		t.Errorf("expected item name obj-item, got %s", list.Items[0].Name)
	}

	// nil receiver returns nil
	var nilList *TKEMachineNodeClassList
	if nilList.DeepCopyObject() != nil {
		t.Error("DeepCopyObject of nil TKEMachineNodeClassList should return nil")
	}
}

// ---------------------------------------------------------------------------
// 13. TKEMachineNodeClassSpec
// ---------------------------------------------------------------------------

func TestTKEMachineNodeClassSpec_DeepCopy(t *testing.T) {
	fs := FileSystemEXT4
	ct := BandwidthPackage
	original := &TKEMachineNodeClassSpec{
		SubnetSelectorTerms: []SubnetSelectorTerm{
			{Tags: map[string]string{"zone": "*"}, ID: "subnet-a"},
		},
		SecurityGroupSelectorTerms: []SecurityGroupSelectorTerm{
			{Tags: map[string]string{"sg": "default"}},
		},
		SSHKeySelectorTerms: []SSHKeySelectorTerm{
			{ID: "skey-abc"},
		},
		SystemDisk: &SystemDisk{Size: 50, Type: DiskTypeCloudPremium},
		DataDisks: []DataDisk{
			{Size: 100, Type: DiskTypeCloudSSD, FileSystem: &fs},
		},
		InternetAccessible: &InternetAccessible{
			MaxBandwidthOut: lo.ToPtr(int32(10)),
			ChargeType:      &ct,
		},
		LifecycleScript: &LifecycleScript{
			PreInitScript: lo.ToPtr("echo pre"),
		},
		Tags: map[string]string{"env": "staging"},
	}

	copied := original.DeepCopy()

	// Verify SubnetSelectorTerms deep independence
	copied.SubnetSelectorTerms[0].Tags["zone"] = "changed"
	if original.SubnetSelectorTerms[0].Tags["zone"] != "*" {
		t.Error("modifying copied SubnetSelectorTerms Tags affected original")
	}

	// Verify SecurityGroupSelectorTerms deep independence
	copied.SecurityGroupSelectorTerms[0].Tags["sg"] = "changed"
	if original.SecurityGroupSelectorTerms[0].Tags["sg"] != "default" {
		t.Error("modifying copied SecurityGroupSelectorTerms Tags affected original")
	}

	// Verify SSHKeySelectorTerms deep independence
	copied.SSHKeySelectorTerms[0].ID = "skey-changed"
	if original.SSHKeySelectorTerms[0].ID != "skey-abc" {
		t.Error("modifying copied SSHKeySelectorTerms affected original")
	}

	// Verify SystemDisk deep independence
	copied.SystemDisk.Size = 200
	if original.SystemDisk.Size != 50 {
		t.Error("modifying copied SystemDisk affected original")
	}

	// Verify DataDisks deep independence
	*copied.DataDisks[0].FileSystem = FileSystemXFS
	if *original.DataDisks[0].FileSystem != FileSystemEXT4 {
		t.Error("modifying copied DataDisks FileSystem affected original")
	}

	// Verify InternetAccessible deep independence
	*copied.InternetAccessible.MaxBandwidthOut = 99
	if *original.InternetAccessible.MaxBandwidthOut != 10 {
		t.Error("modifying copied InternetAccessible affected original")
	}

	// Verify LifecycleScript deep independence
	*copied.LifecycleScript.PreInitScript = "echo changed"
	if *original.LifecycleScript.PreInitScript != "echo pre" {
		t.Error("modifying copied LifecycleScript affected original")
	}

	// Verify Tags deep independence
	copied.Tags["env"] = "prod"
	if original.Tags["env"] != "staging" {
		t.Error("modifying copied Tags affected original")
	}

	// nil receiver
	var nilSpec *TKEMachineNodeClassSpec
	if nilSpec.DeepCopy() != nil {
		t.Error("DeepCopy of nil TKEMachineNodeClassSpec should return nil")
	}
}

func TestTKEMachineNodeClassSpec_DeepCopy_NilOptionalFields(t *testing.T) {
	original := &TKEMachineNodeClassSpec{
		SubnetSelectorTerms:        []SubnetSelectorTerm{{ID: "subnet-1"}},
		SecurityGroupSelectorTerms: []SecurityGroupSelectorTerm{{ID: "sg-1"}},
	}

	copied := original.DeepCopy()

	if copied.SystemDisk != nil {
		t.Error("expected nil SystemDisk in copy")
	}
	if copied.DataDisks != nil {
		t.Error("expected nil DataDisks in copy")
	}
	if copied.InternetAccessible != nil {
		t.Error("expected nil InternetAccessible in copy")
	}
	if copied.LifecycleScript != nil {
		t.Error("expected nil LifecycleScript in copy")
	}
	if copied.Tags != nil {
		t.Error("expected nil Tags in copy")
	}
	if copied.SSHKeySelectorTerms != nil {
		t.Error("expected nil SSHKeySelectorTerms in copy")
	}
}

func TestTKEMachineNodeClassSpec_DeepCopyInto(t *testing.T) {
	original := TKEMachineNodeClassSpec{
		SubnetSelectorTerms: []SubnetSelectorTerm{
			{ID: "subnet-into"},
		},
		SecurityGroupSelectorTerms: []SecurityGroupSelectorTerm{
			{ID: "sg-into"},
		},
		Tags: map[string]string{"k": "v"},
	}

	var out TKEMachineNodeClassSpec
	original.DeepCopyInto(&out)

	if len(out.SubnetSelectorTerms) != 1 || out.SubnetSelectorTerms[0].ID != "subnet-into" {
		t.Error("SubnetSelectorTerms not copied correctly")
	}
	if out.Tags["k"] != "v" {
		t.Errorf("expected tag k=v, got %s", out.Tags["k"])
	}
}

// ---------------------------------------------------------------------------
// 14. TKEMachineNodeClassStatus
// ---------------------------------------------------------------------------

func TestTKEMachineNodeClassStatus_DeepCopy(t *testing.T) {
	original := &TKEMachineNodeClassStatus{
		Subnets: []Subnet{
			{ID: "subnet-s1", Zone: "ap-guangzhou-3", ZoneID: "100003"},
			{ID: "subnet-s2", Zone: "ap-guangzhou-4"},
		},
		SecurityGroups: []SecurityGroup{
			{ID: "sg-s1"},
		},
		SSHKeys: []SSHKey{
			{ID: "skey-s1"},
		},
		Conditions: []op.Condition{
			{
				Type:   ConditionTypeNodeClassReady,
				Status: metav1.ConditionTrue,
			},
		},
	}

	copied := original.DeepCopy()

	// Verify lengths
	if len(copied.Subnets) != 2 {
		t.Fatalf("expected 2 subnets, got %d", len(copied.Subnets))
	}
	if len(copied.SecurityGroups) != 1 {
		t.Fatalf("expected 1 security group, got %d", len(copied.SecurityGroups))
	}
	if len(copied.SSHKeys) != 1 {
		t.Fatalf("expected 1 ssh key, got %d", len(copied.SSHKeys))
	}
	if len(copied.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(copied.Conditions))
	}

	// Verify values
	if copied.Subnets[0].ID != "subnet-s1" {
		t.Errorf("expected subnet ID subnet-s1, got %s", copied.Subnets[0].ID)
	}
	if copied.Conditions[0].Type != ConditionTypeNodeClassReady {
		t.Errorf("expected condition type %s, got %s", ConditionTypeNodeClassReady, copied.Conditions[0].Type)
	}

	// Verify deep independence of Subnets slice
	copied.Subnets[0].ID = "subnet-changed"
	if original.Subnets[0].ID != "subnet-s1" {
		t.Error("modifying copied Subnets affected original")
	}

	// Verify deep independence of SecurityGroups slice
	copied.SecurityGroups[0].ID = "sg-changed"
	if original.SecurityGroups[0].ID != "sg-s1" {
		t.Error("modifying copied SecurityGroups affected original")
	}

	// Verify deep independence of SSHKeys slice
	copied.SSHKeys[0].ID = "skey-changed"
	if original.SSHKeys[0].ID != "skey-s1" {
		t.Error("modifying copied SSHKeys affected original")
	}

	// Verify deep independence of Conditions slice
	copied.Conditions[0].Status = metav1.ConditionFalse
	if original.Conditions[0].Status != metav1.ConditionTrue {
		t.Error("modifying copied Conditions affected original")
	}

	// nil receiver
	var nilStatus *TKEMachineNodeClassStatus
	if nilStatus.DeepCopy() != nil {
		t.Error("DeepCopy of nil TKEMachineNodeClassStatus should return nil")
	}
}

func TestTKEMachineNodeClassStatus_DeepCopy_NilSlices(t *testing.T) {
	original := &TKEMachineNodeClassStatus{}

	copied := original.DeepCopy()

	if copied.Subnets != nil {
		t.Error("expected nil Subnets in copy")
	}
	if copied.SecurityGroups != nil {
		t.Error("expected nil SecurityGroups in copy")
	}
	if copied.SSHKeys != nil {
		t.Error("expected nil SSHKeys in copy")
	}
	if copied.Conditions != nil {
		t.Error("expected nil Conditions in copy")
	}
}

func TestTKEMachineNodeClassStatus_DeepCopyInto(t *testing.T) {
	original := TKEMachineNodeClassStatus{
		Subnets: []Subnet{{ID: "subnet-into1"}},
		Conditions: []op.Condition{
			{
				Type:   ConditionTypeNodeClassReady,
				Status: metav1.ConditionTrue,
			},
		},
	}

	var out TKEMachineNodeClassStatus
	original.DeepCopyInto(&out)

	if len(out.Subnets) != 1 || out.Subnets[0].ID != "subnet-into1" {
		t.Error("Subnets not copied correctly")
	}
	if len(out.Conditions) != 1 || out.Conditions[0].Type != ConditionTypeNodeClassReady {
		t.Error("Conditions not copied correctly")
	}

	// Verify independence
	out.Subnets[0].ID = "changed"
	if original.Subnets[0].ID != "subnet-into1" {
		t.Error("modifying out Subnets affected original")
	}
}
