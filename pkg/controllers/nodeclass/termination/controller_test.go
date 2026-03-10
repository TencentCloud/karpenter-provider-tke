package termination

import (
	"context"
	"fmt"
	"testing"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/events"
)

// mockRecorder implements events.Recorder.
type mockRecorder struct {
	published []events.Event
}

func (r *mockRecorder) Publish(evts ...events.Event) {
	r.published = append(r.published, evts...)
}

// termFakeClient is a minimal client.Client for testing the termination controller.
type termFakeClient struct {
	listFn   func(context.Context, client.ObjectList, ...client.ListOption) error
	updateFn func(context.Context, client.Object, ...client.UpdateOption) error
}

func (c *termFakeClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}
func (c *termFakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listFn != nil {
		return c.listFn(ctx, list, opts...)
	}
	return nil
}
func (c *termFakeClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return nil
}
func (c *termFakeClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return nil
}
func (c *termFakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateFn != nil {
		return c.updateFn(ctx, obj, opts...)
	}
	return nil
}
func (c *termFakeClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return nil
}
func (c *termFakeClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}
func (c *termFakeClient) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
	return nil
}
func (c *termFakeClient) Watch(_ context.Context, _ client.ObjectList, _ ...client.ListOption) (watch.Interface, error) {
	return nil, fmt.Errorf("watch not implemented")
}
func (c *termFakeClient) Status() client.SubResourceWriter {
	return &termFakeStatusWriter{}
}
func (c *termFakeClient) SubResource(_ string) client.SubResourceClient {
	return nil
}
func (c *termFakeClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}
func (c *termFakeClient) RESTMapper() meta.RESTMapper {
	return nil
}
func (c *termFakeClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (c *termFakeClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return false, nil
}

type termFakeStatusWriter struct{}

func (w *termFakeStatusWriter) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return nil
}
func (w *termFakeStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return nil
}
func (w *termFakeStatusWriter) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return nil
}

func TestTermination_NewController(t *testing.T) {
	c := NewController(&termFakeClient{}, &mockRecorder{})
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
}

func TestTermination_Reconcile_NotDeleting(t *testing.T) {
	c := NewController(&termFakeClient{}, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	result, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("expected no requeue for non-deleting nodeClass")
	}
}

func TestTermination_Reconcile_Deleting(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
			// Return empty list - no node claims using this class
			return nil
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	result, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Finalizer should be removed
	if controllerutil.ContainsFinalizer(nodeClass, api.TerminationFinalizer) {
		t.Error("expected finalizer to be removed")
	}
	_ = result
}

func TestTermination_Finalize_NoFinalizer(t *testing.T) {
	now := metav1.Now()
	c := NewController(&termFakeClient{}, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			// No finalizer
		},
	}
	result, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("expected no requeue when no finalizer")
	}
}

func TestTermination_Finalize_HasNodeClaims(t *testing.T) {
	now := metav1.Now()
	recorder := &mockRecorder{}
	fc := &termFakeClient{
		listFn: func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
			if ncList, ok := list.(*v1.NodeClaimList); ok {
				ncList.Items = []v1.NodeClaim{
					{ObjectMeta: metav1.ObjectMeta{Name: "nc-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "nc-2"}},
				}
			}
			return nil
		},
	}
	c := NewController(fc, recorder)
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	result, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should requeue and keep the finalizer
	if result.RequeueAfter == 0 {
		t.Error("expected requeue when node claims still exist")
	}
	if !controllerutil.ContainsFinalizer(nodeClass, api.TerminationFinalizer) {
		t.Error("expected finalizer to still be present")
	}
	if len(recorder.published) == 0 {
		t.Error("expected event to be published")
	}
}

func TestTermination_Finalize_NoNodeClaims_RemoveFinalizer(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
			return nil // empty list
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if controllerutil.ContainsFinalizer(nodeClass, api.TerminationFinalizer) {
		t.Error("expected finalizer to be removed")
	}
}

func TestTermination_Finalize_ListError(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
			return fmt.Errorf("list error")
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestTermination_Finalize_UpdateConflict(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
			return nil // empty list
		},
		updateFn: func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
			return errors.NewConflict(schema.GroupResource{}, "test", fmt.Errorf("conflict"))
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	result, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On conflict, should requeue
	if !result.Requeue {
		t.Error("expected Requeue on conflict")
	}
}

func TestTermination_Finalize_GenericUpdateError(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
			return nil // empty list
		},
		updateFn: func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
			return fmt.Errorf("server error")
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err == nil {
		t.Fatal("expected error for generic update error")
	}
}

func TestTermination_Finalize_UpdateNotFoundIgnored(t *testing.T) {
	now := metav1.Now()
	fc := &termFakeClient{
		listFn: func(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
			return nil // empty list
		},
		updateFn: func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
			return errors.NewNotFound(schema.GroupResource{}, "test")
		},
	}
	c := NewController(fc, &mockRecorder{})
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &now,
			Finalizers:        []string{api.TerminationFinalizer},
		},
	}
	// NotFound errors should be ignored by client.IgnoreNotFound
	_, err := c.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("expected no error for NotFound update error (should be ignored), got %v", err)
	}
}
