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

type mockSubnetZoneProvider struct {
	idFromZoneFn func(string) (string, error)
	zoneFromIDFn func(string) (string, error)
}

func (m *mockSubnetZoneProvider) IDFromZone(zone string) (string, error) {
	if m.idFromZoneFn != nil {
		return m.idFromZoneFn(zone)
	}
	return "100001", nil
}

func (m *mockSubnetZoneProvider) ZoneFromID(id string) (string, error) {
	if m.zoneFromIDFn != nil {
		return m.zoneFromIDFn(id)
	}
	return "ap-guangzhou-1", nil
}

func TestSubnet_Reconcile_Error(t *testing.T) {
	s := &Subnet{
		zoneProvider: &mockSubnetZoneProvider{},
		vpcProvider: &mockVpcProvider{
			listSubnetsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
				return nil, fmt.Errorf("subnet list failed")
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	_, err := s.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error")
	}
	if nodeClass.Status.Subnets != nil {
		t.Error("expected nil subnets on error")
	}
}

func TestSubnet_Reconcile_Empty(t *testing.T) {
	s := &Subnet{
		zoneProvider: &mockSubnetZoneProvider{},
		vpcProvider: &mockVpcProvider{
			listSubnetsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
				return []*vpc.Subnet{}, nil
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status: api.TKEMachineNodeClassStatus{
			Subnets: []api.Subnet{{ID: "old"}},
		},
	}
	result, err := s.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeClass.Status.Subnets != nil {
		t.Error("expected nil subnets when empty list returned")
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue for empty result")
	}
}

func TestSubnet_Reconcile_Success(t *testing.T) {
	s := &Subnet{
		zoneProvider: &mockSubnetZoneProvider{
			idFromZoneFn: func(zone string) (string, error) {
				switch zone {
				case "ap-guangzhou-3":
					return "100003", nil
				case "ap-guangzhou-4":
					return "100004", nil
				default:
					return "", fmt.Errorf("unknown zone")
				}
			},
		},
		vpcProvider: &mockVpcProvider{
			listSubnetsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
				return []*vpc.Subnet{
					{
						SubnetId:                lo.ToPtr("subnet-bbb"),
						Zone:                    lo.ToPtr("ap-guangzhou-3"),
						AvailableIpAddressCount: lo.ToPtr(uint64(10)),
					},
					{
						SubnetId:                lo.ToPtr("subnet-aaa"),
						Zone:                    lo.ToPtr("ap-guangzhou-4"),
						AvailableIpAddressCount: lo.ToPtr(uint64(100)),
					},
					{
						SubnetId:                lo.ToPtr("subnet-ccc"),
						Zone:                    lo.ToPtr("ap-guangzhou-3"),
						AvailableIpAddressCount: lo.ToPtr(uint64(10)),
					},
				}, nil
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	result, err := s.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodeClass.Status.Subnets) != 3 {
		t.Fatalf("expected 3 subnets, got %d", len(nodeClass.Status.Subnets))
	}
	// Sorted by IP count descending, then ID ascending
	// subnet-aaa: 100 IPs
	// subnet-bbb: 10 IPs
	// subnet-ccc: 10 IPs
	if nodeClass.Status.Subnets[0].ID != "subnet-aaa" {
		t.Errorf("expected first subnet to be subnet-aaa (100 IPs), got %s", nodeClass.Status.Subnets[0].ID)
	}
	if nodeClass.Status.Subnets[1].ID != "subnet-bbb" {
		t.Errorf("expected second subnet to be subnet-bbb (10 IPs, ID < ccc), got %s", nodeClass.Status.Subnets[1].ID)
	}
	if nodeClass.Status.Subnets[2].ID != "subnet-ccc" {
		t.Errorf("expected third subnet to be subnet-ccc, got %s", nodeClass.Status.Subnets[2].ID)
	}
	// Verify zone mapping
	if nodeClass.Status.Subnets[0].Zone != "ap-guangzhou-4" {
		t.Errorf("expected zone ap-guangzhou-4 for subnet-aaa, got %s", nodeClass.Status.Subnets[0].Zone)
	}
	if nodeClass.Status.Subnets[0].ZoneID != "100004" {
		t.Errorf("expected zoneID 100004 for subnet-aaa, got %s", nodeClass.Status.Subnets[0].ZoneID)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue, got %v", result.RequeueAfter)
	}
}
