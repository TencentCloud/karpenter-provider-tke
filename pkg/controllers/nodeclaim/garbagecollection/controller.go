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
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	"github.com/awslabs/operatorpkg/singleton"
	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
)

// stuckMachineDeletionThreshold defines how long a Machine must have been in deletion
// before garbage collection will forcibly strip karpenter's termination finalizer from
// its associated Node. Set conservatively: legitimate Machine deletions in TKE may
// take a few minutes; we only intervene once deletion is well past the normal envelope.
const stuckMachineDeletionThreshold = 5 * time.Minute

type Controller struct {
	kubeClient      client.Client
	cloudProvider   cloudprovider.CloudProvider
	successfulCount uint64 // keeps track of successful reconciles for more aggressive requeueing near the start of the controller
}

func NewController(kubeClient client.Client, cloudProvider cloudprovider.CloudProvider) *Controller {
	return &Controller{
		kubeClient:      kubeClient,
		cloudProvider:   cloudProvider,
		successfulCount: 0,
	}
}

func (c *Controller) Reconcile(ctx context.Context) (reconciler.Result, error) {
	ctx = injection.WithControllerName(ctx, "machine.garbagecollection")

	// We LIST machines on the CloudProvider BEFORE we grab Machines/Nodes on the cluster so that we make sure that, if
	// LISTing instances takes a long time, our information is more updated by the time we get to Machine and Node LIST
	// This works since our CloudProvider instances are deleted based on whether the Machine exists or not, not vise-versa
	retrieved, err := c.cloudProvider.List(ctx)
	if err != nil {
		return reconciler.Result{}, fmt.Errorf("listing cloudprovider machines, %w", err)
	}
	managedRetrieved := lo.Filter(retrieved, func(nc *v1.NodeClaim, _ int) bool {
		return nc.Annotations[api.AnnotationManagedBy] != "" && nc.DeletionTimestamp.IsZero()
	})
	nodeClaimList := &v1.NodeClaimList{}
	if err = c.kubeClient.List(ctx, nodeClaimList); err != nil {
		return reconciler.Result{}, err
	}
	machineList := &capiv1beta1.MachineList{}
	if err = c.kubeClient.List(ctx, machineList); err != nil {
		return reconciler.Result{}, err
	}
	resolvedOwnedMachines := sets.New(lo.FilterMap(nodeClaimList.Items, func(n v1.NodeClaim, _ int) (string, bool) {
		return n.Annotations[api.AnnotationOwnedMachine], n.Annotations[api.AnnotationOwnedMachine] != ""
	})...)
	errs := make([]error, len(retrieved))
	workqueue.ParallelizeUntil(ctx, 100, len(managedRetrieved), func(i int) {
		if !resolvedOwnedMachines.Has(managedRetrieved[i].Annotations[api.AnnotationOwnedMachine]) &&
			time.Since(managedRetrieved[i].CreationTimestamp.Time) > time.Second*30 {
			errs[i] = c.garbageCollect(ctx, managedRetrieved[i], machineList)
		}
	})
	if err = multierr.Combine(errs...); err != nil {
		return reconciler.Result{}, err
	}
	if err = c.unstickTerminatingMachines(ctx, machineList); err != nil {
		return reconciler.Result{}, err
	}
	c.successfulCount++
	return reconciler.Result{RequeueAfter: lo.Ternary(c.successfulCount <= 20, time.Second*10, time.Minute)}, nil
}

func (c *Controller) garbageCollect(ctx context.Context, nodeClaim *v1.NodeClaim, machineList *capiv1beta1.MachineList) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("owned-machine", nodeClaim.Annotations[api.AnnotationOwnedMachine]))
	if err := c.cloudProvider.Delete(ctx, nodeClaim); err != nil {
		return cloudprovider.IgnoreNodeClaimNotFoundError(err)
	}
	log.FromContext(ctx).Info("garbage collected cloudprovider instance")

	// Go ahead and cleanup the node if we know that it exists to make scheduling go quicker
	if machine, ok := lo.Find(machineList.Items, func(m capiv1beta1.Machine) bool {
		return m.Name == nodeClaim.Annotations[api.AnnotationOwnedMachine]
	}); ok {
		if err := c.kubeClient.Delete(ctx, &machine); err != nil {
			return client.IgnoreNotFound(err)
		}
		log.FromContext(ctx).WithValues("Node", klog.KRef("", machine.Name)).Info("garbage collected node")
	}
	return nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("machine.garbagecollection").
		WatchesRawSource(singleton.Source()).
		Complete(singleton.AsReconciler(c))
}

// unstickTerminatingMachines is a safety net for the deadlock between TKE's Machine
// controller (which waits for the associated Node to be deleted before completing
// Machine finalization) and karpenter's Node termination controller (which only
// removes the karpenter.sh/termination finalizer from the Node after the cloud
// instance reports NotFound). On the normal Delete path the machine provider strips
// the finalizer up front. This GC path catches stragglers — for example, machines
// that were already stuck before this fix was rolled out, or where the karpenter
// reconcile loop has stopped driving (e.g. NodeClaim was force-deleted).
//
// To stay safe we only act when:
//  1. The Machine has a DeletionTimestamp that's older than stuckMachineDeletionThreshold
//     (legitimate TKE Machine deletions can take a few minutes; we intervene only
//     after the deletion is clearly stuck);
//  2. The associated Node also carries a DeletionTimestamp (deletion has been
//     consented by karpenter / kubectl — we don't tear out finalizers on Nodes
//     that nobody is trying to delete);
//  3. The Node still carries the karpenter.sh/termination finalizer.
//
// We only strip that single finalizer, leaving any other finalizers (CCM, CSI, etc.)
// intact.
func (c *Controller) unstickTerminatingMachines(ctx context.Context, machineList *capiv1beta1.MachineList) error {
	var errs []error
	for i := range machineList.Items {
		m := &machineList.Items[i]
		if m.DeletionTimestamp == nil {
			continue
		}
		if time.Since(m.DeletionTimestamp.Time) < stuckMachineDeletionThreshold {
			continue
		}
		if m.Spec.ProviderID == nil || *m.Spec.ProviderID == "" {
			continue
		}
		nodeList := &corev1.NodeList{}
		if err := c.kubeClient.List(ctx, nodeList, client.MatchingFields{"spec.providerID": *m.Spec.ProviderID}); err != nil {
			errs = append(errs, fmt.Errorf("listing nodes for stuck machine %s: %w", m.Name, err))
			continue
		}
		for j := range nodeList.Items {
			node := &nodeList.Items[j]
			if node.DeletionTimestamp == nil {
				continue
			}
			if !controllerutil.ContainsFinalizer(node, v1.TerminationFinalizer) {
				continue
			}
			stored := node.DeepCopy()
			controllerutil.RemoveFinalizer(node, v1.TerminationFinalizer)
			if equality.Semantic.DeepEqual(stored, node) {
				continue
			}
			if err := c.kubeClient.Patch(ctx, node, client.StrategicMergeFrom(stored)); err != nil {
				errs = append(errs, fmt.Errorf("removing karpenter finalizer from stuck node %s: %w", node.Name, client.IgnoreNotFound(err)))
				continue
			}
			log.FromContext(ctx).
				WithValues("Node", klog.KRef("", node.Name), "Machine", klog.KRef("", m.Name)).
				Info("removed karpenter termination finalizer from node to unblock stuck machine deletion")
		}
	}
	return multierr.Combine(errs...)
}
