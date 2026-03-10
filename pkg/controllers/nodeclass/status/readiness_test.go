package status

import (
	"context"
	"testing"

	"github.com/awslabs/operatorpkg/status"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadiness_Reconcile_NoSubnets(t *testing.T) {
	r := Readiness{}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
		Status: api.TKEMachineNodeClassStatus{
			Subnets:        []api.Subnet{},
			SecurityGroups: []api.SecurityGroup{{ID: "sg-123"}},
		},
	}
	_, err := r.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := nodeClass.StatusConditions().Get(status.ConditionReady)
	if cond.IsTrue() {
		t.Error("expected Ready condition to be false when no subnets")
	}
}

func TestReadiness_Reconcile_NoSecurityGroups(t *testing.T) {
	r := Readiness{}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
		Status: api.TKEMachineNodeClassStatus{
			Subnets:        []api.Subnet{{ID: "subnet-123", Zone: "ap-guangzhou-3"}},
			SecurityGroups: []api.SecurityGroup{},
		},
	}
	_, err := r.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := nodeClass.StatusConditions().Get(status.ConditionReady)
	if cond.IsTrue() {
		t.Error("expected Ready condition to be false when no security groups")
	}
}

func TestReadiness_Reconcile_AllReady(t *testing.T) {
	r := Readiness{}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
		Status: api.TKEMachineNodeClassStatus{
			Subnets:        []api.Subnet{{ID: "subnet-123", Zone: "ap-guangzhou-3"}},
			SecurityGroups: []api.SecurityGroup{{ID: "sg-123"}},
		},
	}
	_, err := r.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := nodeClass.StatusConditions().Get(status.ConditionReady)
	if !cond.IsTrue() {
		t.Error("expected Ready condition to be true when subnets and security groups are present")
	}
}
