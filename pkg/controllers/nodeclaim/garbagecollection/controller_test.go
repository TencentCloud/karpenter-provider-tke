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

package garbagecollection

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/samber/lo"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const karpenterFinalizer = "karpenter.sh/termination"

// fakeClient is a focused, in-memory client.Client supporting:
//   - List for MachineList and NodeList (with spec.providerID field selector)
//   - Patch that reflects finalizer mutations onto the stored object
//
// Other methods are stubs returning nil — enough for the unstickTerminatingMachines path.
type fakeClient struct {
	objects []client.Object
}

func newFakeClient(objs ...client.Object) *fakeClient {
	return &fakeClient{objects: objs}
}

func (c *fakeClient) findNode(name string) *corev1.Node {
	for _, o := range c.objects {
		if n, ok := o.(*corev1.Node); ok && n.Name == name {
			return n
		}
	}
	return nil
}

func (c *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return fmt.Errorf("Get not implemented")
}

func (c *fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(listOpts)
	}
	if ml, ok := list.(*capiv1beta1.MachineList); ok {
		ml.Items = []capiv1beta1.Machine{}
		for _, o := range c.objects {
			if m, ok := o.(*capiv1beta1.Machine); ok {
				ml.Items = append(ml.Items, *m)
			}
		}
		return nil
	}
	if nl, ok := list.(*corev1.NodeList); ok {
		wantProviderID := ""
		if listOpts.FieldSelector != nil {
			for _, req := range listOpts.FieldSelector.Requirements() {
				if req.Field == "spec.providerID" {
					wantProviderID = req.Value
				}
			}
		}
		nl.Items = []corev1.Node{}
		for _, o := range c.objects {
			n, ok := o.(*corev1.Node)
			if !ok {
				continue
			}
			if wantProviderID != "" && n.Spec.ProviderID != wantProviderID {
				continue
			}
			nl.Items = append(nl.Items, *n)
		}
		return nil
	}
	return nil
}

func (c *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.objects = append(c.objects, obj)
	return nil
}

func (c *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}

func (c *fakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (c *fakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	for i, stored := range c.objects {
		if stored.GetName() != obj.GetName() || stored.GetNamespace() != obj.GetNamespace() {
			continue
		}
		stored.SetFinalizers(obj.GetFinalizers())
		c.objects[i] = stored
		return nil
	}
	return nil
}

func (c *fakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}
func (c *fakeClient) Status() client.StatusWriter         { return stubStatusWriter{} }
func (c *fakeClient) SubResource(string) client.SubResourceClient { return stubSubResourceClient{} }
func (c *fakeClient) Scheme() *runtime.Scheme             { return runtime.NewScheme() }
func (c *fakeClient) RESTMapper() meta.RESTMapper         { return stubMapper{} }
func (c *fakeClient) GroupVersionKindFor(runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (c *fakeClient) IsObjectNamespaced(runtime.Object) (bool, error)         { return false, nil }
func (c *fakeClient) Watch(context.Context, client.ObjectList, ...client.ListOption) (watch.Interface, error) {
	return nil, fmt.Errorf("watch not implemented")
}
func (c *fakeClient) Apply(context.Context, runtime.ApplyConfiguration, ...client.ApplyOption) error {
	return nil
}

type stubStatusWriter struct{}

func (stubStatusWriter) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return nil
}
func (stubStatusWriter) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return nil
}
func (stubStatusWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return nil
}

type stubSubResourceClient struct{}

func (stubSubResourceClient) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return nil
}
func (stubSubResourceClient) Get(context.Context, client.Object, client.Object, ...client.SubResourceGetOption) error {
	return nil
}
func (stubSubResourceClient) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return nil
}
func (stubSubResourceClient) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return nil
}

type stubMapper struct{}

func (stubMapper) ResourceFor(r schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return r, nil
}
func (stubMapper) KindFor(schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (stubMapper) KindsFor(schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}
func (stubMapper) ResourceSingularizer(s string) (string, error) { return s, nil }
func (stubMapper) ResourcesFor(r schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return []schema.GroupVersionResource{r}, nil
}
func (stubMapper) RESTMapping(schema.GroupKind, ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{}, nil
}
func (stubMapper) RESTMappings(schema.GroupKind, ...string) ([]*meta.RESTMapping, error) {
	return nil, nil
}

// --- test fixtures ---

func makeMachine(name, providerID string, deletedAgo time.Duration) *capiv1beta1.Machine {
	m := &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       capiv1beta1.MachineSpec{ProviderID: lo.ToPtr(providerID)},
	}
	if deletedAgo > 0 {
		t := metav1.NewTime(time.Now().Add(-deletedAgo))
		m.DeletionTimestamp = &t
	}
	return m
}

func makeNode(name, providerID string, finalizers []string, deleting bool) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Finalizers: finalizers},
		Spec:       corev1.NodeSpec{ProviderID: providerID},
	}
	if deleting {
		t := metav1.Now()
		n.DeletionTimestamp = &t
	}
	return n
}

func nodeHasFinalizer(n *corev1.Node, f string) bool {
	if n == nil {
		return false
	}
	return slices.Contains(n.Finalizers, f)
}

func runUnstick(t *testing.T, c *fakeClient) {
	t.Helper()
	ctl := &Controller{kubeClient: c}
	ml := &capiv1beta1.MachineList{}
	if err := c.List(context.Background(), ml); err != nil {
		t.Fatalf("list machines: %v", err)
	}
	if err := ctl.unstickTerminatingMachines(context.Background(), ml); err != nil {
		t.Fatalf("unstickTerminatingMachines returned error: %v", err)
	}
}

// --- tests ---

func TestUnstick_StuckPastThreshold_StripsKarpenterFinalizer(t *testing.T) {
	pid := "qcloud:///gz/ins-1"
	m := makeMachine("m1", pid, stuckMachineDeletionThreshold+time.Minute)
	n := makeNode("n1", pid, []string{karpenterFinalizer}, true)
	c := newFakeClient(m, n)

	runUnstick(t, c)

	if nodeHasFinalizer(c.findNode("n1"), karpenterFinalizer) {
		t.Fatalf("expected karpenter finalizer to be stripped")
	}
}

func TestUnstick_BelowThreshold_NoChange(t *testing.T) {
	pid := "qcloud:///gz/ins-2"
	m := makeMachine("m2", pid, time.Minute) // only 1 min in deletion
	n := makeNode("n2", pid, []string{karpenterFinalizer}, true)
	c := newFakeClient(m, n)

	runUnstick(t, c)

	if !nodeHasFinalizer(c.findNode("n2"), karpenterFinalizer) {
		t.Fatalf("expected karpenter finalizer to remain (below threshold)")
	}
}

func TestUnstick_NodeWithoutDeletionTimestamp_NoChange(t *testing.T) {
	pid := "qcloud:///gz/ins-3"
	m := makeMachine("m3", pid, stuckMachineDeletionThreshold+time.Minute)
	n := makeNode("n3", pid, []string{karpenterFinalizer}, false) // no DT
	c := newFakeClient(m, n)

	runUnstick(t, c)

	if !nodeHasFinalizer(c.findNode("n3"), karpenterFinalizer) {
		t.Fatalf("expected karpenter finalizer to remain when Node has no DeletionTimestamp")
	}
}

func TestUnstick_PreservesOtherFinalizers(t *testing.T) {
	pid := "qcloud:///gz/ins-4"
	m := makeMachine("m4", pid, stuckMachineDeletionThreshold+time.Minute)
	n := makeNode("n4", pid,
		[]string{"ccm.io/protection", karpenterFinalizer, "csi.io/volume"}, true)
	c := newFakeClient(m, n)

	runUnstick(t, c)

	got := c.findNode("n4")
	if nodeHasFinalizer(got, karpenterFinalizer) {
		t.Fatalf("karpenter finalizer should be stripped")
	}
	if !nodeHasFinalizer(got, "ccm.io/protection") || !nodeHasFinalizer(got, "csi.io/volume") {
		t.Fatalf("other finalizers should be preserved: %v", got.Finalizers)
	}
}

func TestUnstick_NoKarpenterFinalizer_NoOp(t *testing.T) {
	pid := "qcloud:///gz/ins-5"
	m := makeMachine("m5", pid, stuckMachineDeletionThreshold+time.Minute)
	n := makeNode("n5", pid, []string{"other.io/foo"}, true)
	c := newFakeClient(m, n)

	runUnstick(t, c)

	got := c.findNode("n5")
	if len(got.Finalizers) != 1 || got.Finalizers[0] != "other.io/foo" {
		t.Fatalf("other finalizers should not be touched: %v", got.Finalizers)
	}
}

func TestUnstick_MachineWithoutDeletionTimestamp_NoOp(t *testing.T) {
	pid := "qcloud:///gz/ins-6"
	m := makeMachine("m6", pid, 0) // not deleting
	n := makeNode("n6", pid, []string{karpenterFinalizer}, true)
	c := newFakeClient(m, n)

	runUnstick(t, c)

	if !nodeHasFinalizer(c.findNode("n6"), karpenterFinalizer) {
		t.Fatalf("expected karpenter finalizer to remain when Machine is not deleting")
	}
}

func TestUnstick_MachineWithoutProviderID_NoOp(t *testing.T) {
	m := makeMachine("m7", "", stuckMachineDeletionThreshold+time.Minute) // empty ProviderID
	n := makeNode("n7", "", []string{karpenterFinalizer}, true)
	c := newFakeClient(m, n)

	// Should not panic / not strip (we don't act without ProviderID).
	runUnstick(t, c)

	// Node still has finalizer because lookup is keyed on non-empty ProviderID.
	if !nodeHasFinalizer(c.findNode("n7"), karpenterFinalizer) {
		t.Fatalf("expected finalizer to remain when Machine has no ProviderID")
	}
}

func TestUnstick_MultipleMachines_HandledIndependently(t *testing.T) {
	pid1, pid2 := "qcloud:///gz/ins-8a", "qcloud:///gz/ins-8b"
	stuck := makeMachine("stuck", pid1, stuckMachineDeletionThreshold+time.Minute)
	young := makeMachine("young", pid2, time.Minute)

	nStuck := makeNode("nStuck", pid1, []string{karpenterFinalizer}, true)
	nYoung := makeNode("nYoung", pid2, []string{karpenterFinalizer}, true)

	c := newFakeClient(stuck, young, nStuck, nYoung)
	runUnstick(t, c)

	if nodeHasFinalizer(c.findNode("nStuck"), karpenterFinalizer) {
		t.Fatalf("stuck node should have been unblocked")
	}
	if !nodeHasFinalizer(c.findNode("nYoung"), karpenterFinalizer) {
		t.Fatalf("young node finalizer should remain")
	}
}

// guard that the karpenterFinalizer constant in tests matches the production constant.
func TestKarpenterFinalizerConstantMatchesUpstream(t *testing.T) {
	if v1.TerminationFinalizer != karpenterFinalizer {
		t.Fatalf("test constant %q drift from upstream v1.TerminationFinalizer %q",
			karpenterFinalizer, v1.TerminationFinalizer)
	}
}
