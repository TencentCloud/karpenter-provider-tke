package status

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockVpcProvider struct {
	listSubnetsFn        func(context.Context, *api.TKEMachineNodeClass) ([]*vpc.Subnet, error)
	listSecurityGroupsFn func(context.Context, *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error)
}

func (m *mockVpcProvider) ListSubnets(ctx context.Context, nc *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
	if m.listSubnetsFn != nil {
		return m.listSubnetsFn(ctx, nc)
	}
	return nil, nil
}

func (m *mockVpcProvider) ListSecurityGroups(ctx context.Context, nc *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
	if m.listSecurityGroupsFn != nil {
		return m.listSecurityGroupsFn(ctx, nc)
	}
	return nil, nil
}

func TestSecurityGroup_Reconcile_Error(t *testing.T) {
	sg := &SecurityGroup{
		vpcProvider: &mockVpcProvider{
			listSecurityGroupsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
				return nil, fmt.Errorf("sg list failed")
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	_, err := sg.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error")
	}
	if nodeClass.Status.SecurityGroups != nil {
		t.Error("expected nil security groups on error")
	}
}

func TestSecurityGroup_Reconcile_Empty(t *testing.T) {
	sg := &SecurityGroup{
		vpcProvider: &mockVpcProvider{
			listSecurityGroupsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
				return []*vpc.SecurityGroup{}, nil
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status: api.TKEMachineNodeClassStatus{
			SecurityGroups: []api.SecurityGroup{{ID: "old"}},
		},
	}
	result, err := sg.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeClass.Status.SecurityGroups != nil {
		t.Error("expected nil security groups when empty list returned")
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue for empty result")
	}
}

func TestSecurityGroup_Reconcile_Success(t *testing.T) {
	sg := &SecurityGroup{
		vpcProvider: &mockVpcProvider{
			listSecurityGroupsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
				return []*vpc.SecurityGroup{
					{SecurityGroupId: lo.ToPtr("sg-bbb")},
					{SecurityGroupId: lo.ToPtr("sg-aaa")},
				}, nil
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	result, err := sg.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodeClass.Status.SecurityGroups) != 2 {
		t.Fatalf("expected 2 security groups, got %d", len(nodeClass.Status.SecurityGroups))
	}
	// Verify sorted by ID
	if nodeClass.Status.SecurityGroups[0].ID != "sg-aaa" {
		t.Errorf("expected first sg to be sg-aaa, got %s", nodeClass.Status.SecurityGroups[0].ID)
	}
	if nodeClass.Status.SecurityGroups[1].ID != "sg-bbb" {
		t.Errorf("expected second sg to be sg-bbb, got %s", nodeClass.Status.SecurityGroups[1].ID)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue, got %v", result.RequeueAfter)
	}
}
