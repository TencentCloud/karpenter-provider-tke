package status

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// statusFakeClient is a minimal client.Client for testing the status controller.
type statusFakeClient struct {
	patchErr  error
	statusErr error
	patched   bool
}

func (c *statusFakeClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}
func (c *statusFakeClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}
func (c *statusFakeClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return nil
}
func (c *statusFakeClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return nil
}
func (c *statusFakeClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return nil
}
func (c *statusFakeClient) Patch(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
	c.patched = true
	if c.patchErr != nil {
		return c.patchErr
	}
	if nc, ok := obj.(*api.TKEMachineNodeClass); ok {
		if !controllerutil.ContainsFinalizer(nc, api.TerminationFinalizer) {
			controllerutil.AddFinalizer(nc, api.TerminationFinalizer)
		}
	}
	return nil
}
func (c *statusFakeClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}
func (c *statusFakeClient) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
	return nil
}
func (c *statusFakeClient) Watch(_ context.Context, _ client.ObjectList, _ ...client.ListOption) (watch.Interface, error) {
	return nil, fmt.Errorf("watch not implemented")
}
func (c *statusFakeClient) Status() client.SubResourceWriter {
	return &statusFakeStatusWriter{err: c.statusErr}
}
func (c *statusFakeClient) SubResource(_ string) client.SubResourceClient {
	return nil
}
func (c *statusFakeClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}
func (c *statusFakeClient) RESTMapper() meta.RESTMapper {
	return nil
}
func (c *statusFakeClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (c *statusFakeClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return false, nil
}

type statusFakeStatusWriter struct {
	err error
}

func (w *statusFakeStatusWriter) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return nil
}
func (w *statusFakeStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return nil
}
func (w *statusFakeStatusWriter) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return w.err
}

func TestController_NewController(t *testing.T) {
	c := NewController(
		&statusFakeClient{},
		&mockSubnetZoneProvider{},
		&mockVpcProvider{},
		&mockSSHKeyProvider{},
	)
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	if c.subnet == nil {
		t.Error("expected non-nil subnet reconciler")
	}
	if c.sg == nil {
		t.Error("expected non-nil sg reconciler")
	}
	if c.sshkey == nil {
		t.Error("expected non-nil sshkey reconciler")
	}
	if c.readiness == nil {
		t.Error("expected non-nil readiness reconciler")
	}
}

func TestController_Reconcile_AddFinalizer(t *testing.T) {
	fc := &statusFakeClient{}
	c := NewController(
		fc,
		&mockSubnetZoneProvider{},
		&mockVpcProvider{},
		&mockSSHKeyProvider{},
	)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fc.patched {
		t.Error("expected Patch to be called to add finalizer")
	}
}

func TestController_Reconcile_AlreadyHasFinalizer(t *testing.T) {
	fc := &statusFakeClient{}
	c := NewController(
		fc,
		&mockSubnetZoneProvider{},
		&mockVpcProvider{},
		&mockSSHKeyProvider{},
	)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
			Finalizers: []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestController_Reconcile_PatchFinalizerError(t *testing.T) {
	fc := &statusFakeClient{patchErr: fmt.Errorf("patch failed")}
	c := NewController(
		fc,
		&mockSubnetZoneProvider{},
		&mockVpcProvider{},
		&mockSSHKeyProvider{},
	)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error when patch fails")
	}
}

func TestController_Reconcile_SubReconcilerError(t *testing.T) {
	// Use a VPC provider that returns errors for both ListSubnets and ListSecurityGroups
	fc := &statusFakeClient{}
	c := NewController(
		fc,
		&mockSubnetZoneProvider{},
		&mockVpcProvider{
			listSubnetsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
				return nil, fmt.Errorf("subnet error")
			},
			listSecurityGroupsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
				return nil, fmt.Errorf("sg error")
			},
		},
		&mockSSHKeyProvider{
			listFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
				return nil, fmt.Errorf("sshkey error")
			},
		},
	)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
			Finalizers: []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error from sub-reconcilers")
	}
}

func TestController_Reconcile_StatusPatchError(t *testing.T) {
	// Sub-reconciler modifies nodeClass status, then status patch fails
	fc := &statusFakeClient{statusErr: fmt.Errorf("status patch failed")}
	c := NewController(
		fc,
		&mockSubnetZoneProvider{},
		&mockVpcProvider{
			listSubnetsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.Subnet, error) {
				return []*vpc.Subnet{
					{
						SubnetId:                lo.ToPtr("subnet-123"),
						Zone:                    lo.ToPtr("ap-guangzhou-3"),
						AvailableIpAddressCount: lo.ToPtr(uint64(50)),
					},
				}, nil
			},
			listSecurityGroupsFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*vpc.SecurityGroup, error) {
				return []*vpc.SecurityGroup{
					{SecurityGroupId: lo.ToPtr("sg-123")},
				}, nil
			},
		},
		&mockSSHKeyProvider{
			listFn: func(_ context.Context, _ *api.TKEMachineNodeClass) ([]*cvm.KeyPair, error) {
				return []*cvm.KeyPair{
					{KeyId: lo.ToPtr("skey-123")},
				}, nil
			},
		},
	)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Generation: 1,
			Finalizers: []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error from status patch")
	}
}
