package status

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockSSHKeyProvider struct {
	listFn func(context.Context, *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error)
}

func (m *mockSSHKeyProvider) List(ctx context.Context, nc *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
	if m.listFn != nil {
		return m.listFn(ctx, nc)
	}
	return nil, nil
}

func TestSSHKey_Reconcile_Error(t *testing.T) {
	s := &SSHKey{
		sshKeyProvider: &mockSSHKeyProvider{
			listFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
				return nil, fmt.Errorf("ssh key list failed")
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
	if nodeClass.Status.SSHKeys != nil {
		t.Error("expected nil ssh keys on error")
	}
}

func TestSSHKey_Reconcile_Empty(t *testing.T) {
	s := &SSHKey{
		sshKeyProvider: &mockSSHKeyProvider{
			listFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
				return []*cvm.KeyPair{}, nil
			},
		},
	}
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status: api.TKEMachineNodeClassStatus{
			SSHKeys: []api.SSHKey{{ID: "old"}},
		},
	}
	result, err := s.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeClass.Status.SSHKeys != nil {
		t.Error("expected nil ssh keys when empty list returned")
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue for empty result")
	}
}

func TestSSHKey_Reconcile_Success(t *testing.T) {
	s := &SSHKey{
		sshKeyProvider: &mockSSHKeyProvider{
			listFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
				return []*cvm.KeyPair{
					{KeyId: lo.ToPtr("skey-bbb")},
					{KeyId: lo.ToPtr("skey-aaa")},
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
	if len(nodeClass.Status.SSHKeys) != 2 {
		t.Fatalf("expected 2 ssh keys, got %d", len(nodeClass.Status.SSHKeys))
	}
	// Verify sorted by ID
	if nodeClass.Status.SSHKeys[0].ID != "skey-aaa" {
		t.Errorf("expected first key to be skey-aaa, got %s", nodeClass.Status.SSHKeys[0].ID)
	}
	if nodeClass.Status.SSHKeys[1].ID != "skey-bbb" {
		t.Errorf("expected second key to be skey-bbb, got %s", nodeClass.Status.SSHKeys[1].ID)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue, got %v", result.RequeueAfter)
	}
}
