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

// Package interruption proactively drains TKE spot instances that have been
// flagged for reclamation. TKE's tke-node-termination-handler DaemonSet adds
// the taint tke-node-termination-handler/spot-itn to a node when the underlying
// spot VM enters the reclaim window; this controller watches for that taint
// and deletes the associated NodeClaim so Karpenter's termination controller
// cordons and drains the node before the VM disappears.
package interruption

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/operatorpkg/reasonable"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"
	"sigs.k8s.io/karpenter/pkg/metrics"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
	nodeclaimutil "sigs.k8s.io/karpenter/pkg/utils/nodeclaim"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	interruptionevents "github.com/tencentcloud/karpenter-provider-tke/pkg/controllers/interruption/events"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/instancetype"
)

// SpotInterruptionTaintKey is the taint that tke-node-termination-handler
// applies to a node when TKE has flagged its underlying spot VM for reclaim.
// Observing this taint is our signal to begin a graceful drain.
const SpotInterruptionTaintKey = "tke-node-termination-handler/spot-itn"

// disruptionReason is the metric label reported to
// metrics.NodeClaimsDisruptedTotal when we drain in response to the taint.
const disruptionReason = "spot_interruption"

// offeringCooldown is the duration for which we mark the affected
// (instanceType, zone, spot) offering as unavailable after observing an
// interruption. The goal is to prevent Karpenter from immediately
// re-scheduling onto the same spot pool that just handed us a reclaim notice
// while keeping the exclusion window short enough that a temporarily-unlucky
// pool becomes usable again quickly. AWS uses 3 minutes; we sit slightly
// longer to account for TKE's spot pool signals typically taking a bit more
// time to settle.
const offeringCooldown = 5 * time.Minute

// Controller watches Node objects for the spot-ITN taint. When the taint
// appears on a Node backed by a NodeClaim managed by this cloudprovider, the
// controller marks the affected offering unavailable and deletes the NodeClaim,
// delegating cordon and drain to Karpenter's termination controller.
type Controller struct {
	kubeClient           client.Client
	cloudProvider        cloudprovider.CloudProvider
	instanceTypeProvider instancetype.Provider
	recorder             events.Recorder
}

func NewController(kubeClient client.Client, cloudProvider cloudprovider.CloudProvider, instanceTypeProvider instancetype.Provider, recorder events.Recorder) *Controller {
	return &Controller{
		kubeClient:           kubeClient,
		cloudProvider:        cloudProvider,
		instanceTypeProvider: instanceTypeProvider,
		recorder:             recorder,
	}
}

func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "interruption.spot")

	node := &corev1.Node{}
	if err := c.kubeClient.Get(ctx, req.NamespacedName, node); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting node, %w", err)
	}

	// Re-check the taint against the live object. The predicate may have fired
	// on a slightly stale event; if the taint has since been removed we must
	// not drain.
	if !hasSpotInterruptionTaint(node) {
		return reconcile.Result{}, nil
	}
	if node.Spec.ProviderID == "" {
		// The Node hasn't been fully bootstrapped yet. Nothing we can do until
		// the machine controller populates providerID; a subsequent Node update
		// (which will still carry the taint) will re-enqueue.
		return reconcile.Result{}, nil
	}

	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"Node", klog.KObj(node),
		"providerID", node.Spec.ProviderID,
	))

	nodeClaims, err := nodeclaimutil.ListManaged(ctx, c.kubeClient, c.cloudProvider, nodeclaimutil.ForProviderID(node.Spec.ProviderID))
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("listing nodeclaims for providerID %s, %w", node.Spec.ProviderID, err)
	}
	if len(nodeClaims) == 0 {
		// Not a NodeClaim we manage — either a static node, a node from another
		// provider, or the NodeClaim was already garbage-collected. Nothing to do.
		return reconcile.Result{}, nil
	}

	for _, nodeClaim := range nodeClaims {
		if err := c.drainNodeClaim(ctx, node, nodeClaim); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// drainNodeClaim marks the affected offering unavailable and deletes the
// NodeClaim, which triggers Karpenter's standard cordon-and-drain flow via
// the termination controller. Called only after the spot-ITN taint has been
// confirmed on the associated node.
//
// The offering is marked unavailable *before* the delete so that any pods
// evicted during drain are re-scheduled onto a different pool. If we deleted
// first, Karpenter's provisioner could pick the same spot pool that just
// reclaimed us and immediately hit the same interruption on the replacement
// node.
func (c *Controller) drainNodeClaim(ctx context.Context, node *corev1.Node, nodeClaim *karpv1.NodeClaim) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("NodeClaim", klog.KObj(nodeClaim)))

	if !nodeClaim.DeletionTimestamp.IsZero() {
		return nil
	}

	c.markOfferingUnavailable(ctx, nodeClaim)
	c.recorder.Publish(interruptionevents.SpotInterrupted(node, nodeClaim)...)

	if err := c.kubeClient.Delete(ctx, nodeClaim); err != nil {
		return client.IgnoreNotFound(fmt.Errorf("deleting nodeclaim on spot interruption, %w", err))
	}
	log.FromContext(ctx).Info("initiated drain in response to spot interruption taint")

	metrics.NodeClaimsDisruptedTotal.Inc(map[string]string{
		metrics.ReasonLabel:       disruptionReason,
		metrics.NodePoolLabel:     nodeClaim.Labels[karpv1.NodePoolLabelKey],
		metrics.CapacityTypeLabel: nodeClaim.Labels[karpv1.CapacityTypeLabelKey],
	})
	return nil
}

// markOfferingUnavailable adds the affected (instanceType, capacityType, zone)
// tuple to the instancetype provider's blacklist for offeringCooldown. Uses
// the same zone label (api.LabelCBSToplogy = zone name) as the isBlocked key
// in pkg/providers/instancetype/instancetype.go, which is populated with
// insType.Zone (the raw zone name, not the zoneID).
//
// Best-effort: if a label is missing we skip silently — the drain still
// proceeds. Missing labels shouldn't happen in practice because Karpenter
// core propagates offering requirements onto the NodeClaim.
func (c *Controller) markOfferingUnavailable(ctx context.Context, nodeClaim *karpv1.NodeClaim) {
	insType := nodeClaim.Labels[corev1.LabelInstanceTypeStable]
	capacityType := nodeClaim.Labels[karpv1.CapacityTypeLabelKey]
	zone := nodeClaim.Labels[api.LabelCBSToplogy]
	if insType == "" || capacityType == "" || zone == "" {
		log.FromContext(ctx).V(1).Info("skipping offering cooldown: missing label(s) on NodeClaim",
			"instanceType", insType, "capacityType", capacityType, "zone", zone)
		return
	}
	c.instanceTypeProvider.MarkOfferingUnavailable(ctx, insType, capacityType, zone,
		"spot interruption taint tke-node-termination-handler/spot-itn observed", offeringCooldown)
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("interruption.spot").
		For(&corev1.Node{}).
		WithEventFilter(spotInterruptionTaintPredicate()).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), &nodeReconciler{c: c}))
}

// nodeReconciler adapts Controller.Reconcile to the typed reconcile.ObjectReconciler
// signature expected by reconcile.AsReconciler. Doing the Get ourselves inside
// Controller.Reconcile keeps the not-found path (Node was deleted between the
// predicate firing and the reconcile) clean and lets us re-verify the taint
// against the live object.
type nodeReconciler struct {
	c *Controller
}

func (r *nodeReconciler) Reconcile(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	return r.c.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(node)})
}

// spotInterruptionTaintPredicate enqueues a Node only when the spot-ITN taint
// is (still) present after the change. On Update we short-circuit when the
// taint state didn't change — no need to re-reconcile a Node that was already
// tainted and remains tainted for unrelated reasons.
func spotInterruptionTaintPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			node, ok := e.Object.(*corev1.Node)
			return ok && hasSpotInterruptionTaint(node)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			newNode, ok := e.ObjectNew.(*corev1.Node)
			if !ok || !hasSpotInterruptionTaint(newNode) {
				return false
			}
			oldNode, ok := e.ObjectOld.(*corev1.Node)
			if !ok {
				return true
			}
			// Only reconcile the transition into (or already-tainted refresh
			// after providerID is populated). Skip repeated updates that leave
			// the taint state unchanged and providerID unchanged — the drain
			// is already in flight.
			if !hasSpotInterruptionTaint(oldNode) {
				return true
			}
			return oldNode.Spec.ProviderID != newNode.Spec.ProviderID
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

func hasSpotInterruptionTaint(node *corev1.Node) bool {
	for i := range node.Spec.Taints {
		if node.Spec.Taints[i].Key == SpotInterruptionTaintKey {
			return true
		}
	}
	return false
}
