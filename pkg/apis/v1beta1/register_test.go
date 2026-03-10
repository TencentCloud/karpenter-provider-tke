package v1beta1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	err := AddToScheme(scheme)
	if err != nil {
		t.Fatalf("unexpected error adding to scheme: %v", err)
	}
	// Verify that TKEMachineNodeClass is registered
	gvk := SchemeGroupVersion.WithKind("TKEMachineNodeClass")
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("expected TKEMachineNodeClass to be registered, got error: %v", err)
	}
	if obj == nil {
		t.Fatal("expected non-nil object for TKEMachineNodeClass")
	}

	// Verify TKEMachineNodeClassList is registered
	gvkList := SchemeGroupVersion.WithKind("TKEMachineNodeClassList")
	objList, err := scheme.New(gvkList)
	if err != nil {
		t.Fatalf("expected TKEMachineNodeClassList to be registered, got error: %v", err)
	}
	if objList == nil {
		t.Fatal("expected non-nil object for TKEMachineNodeClassList")
	}
}

func TestSchemeGroupVersion(t *testing.T) {
	if SchemeGroupVersion.Group != Group {
		t.Errorf("expected group %s, got %s", Group, SchemeGroupVersion.Group)
	}
	if SchemeGroupVersion.Version != "v1beta1" {
		t.Errorf("expected version v1beta1, got %s", SchemeGroupVersion.Version)
	}
}
