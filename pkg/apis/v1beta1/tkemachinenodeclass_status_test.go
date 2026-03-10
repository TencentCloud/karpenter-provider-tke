package v1beta1

import (
	"testing"

	op "github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStatusConditions(t *testing.T) {
	nc := &TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
	}
	// StatusConditions() returns a ConditionSet (value type), just verify it doesn't panic
	cs := nc.StatusConditions()
	// Set a condition to verify the condition set works
	cs.SetTrue(ConditionTypeNodeClassReady)
	cond := cs.Get(ConditionTypeNodeClassReady)
	if !cond.IsTrue() {
		t.Error("expected Ready condition to be true after SetTrue")
	}
}

func TestGetSetConditions(t *testing.T) {
	nc := &TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
	}
	conditions := []op.Condition{
		{
			Type:   ConditionTypeNodeClassReady,
			Status: metav1.ConditionTrue,
		},
	}
	nc.SetConditions(conditions)
	got := nc.GetConditions()
	if len(got) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got))
	}
	if got[0].Type != ConditionTypeNodeClassReady {
		t.Errorf("expected condition type %s, got %s", ConditionTypeNodeClassReady, got[0].Type)
	}
	if got[0].Status != metav1.ConditionTrue {
		t.Errorf("expected condition status True, got %s", got[0].Status)
	}
}

func TestGetConditions_Empty(t *testing.T) {
	nc := &TKEMachineNodeClass{}
	got := nc.GetConditions()
	if len(got) != 0 {
		t.Errorf("expected 0 conditions for new object, got %d", len(got))
	}
}
