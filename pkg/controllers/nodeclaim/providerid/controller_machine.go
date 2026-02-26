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

package providerid

import (
	"context"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
)

type ControllerMachine struct {
	kubeClient client.Client
}

func NewControllerMachine(kubeClient client.Client) *ControllerMachine {
	return &ControllerMachine{
		kubeClient: kubeClient,
	}
}

func (c *ControllerMachine) Reconcile(ctx context.Context, machine *capiv1beta1.Machine) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "machine.providerid")
	nodeClaimName := ""
	for _, ref := range machine.OwnerReferences {
		if ref.Kind == "NodeClaim" {
			nodeClaimName = ref.Name
			break
		}
	}
	if nodeClaimName == "" {
		return reconcile.Result{}, nil
	}
	nodeClaim := &v1.NodeClaim{}
	err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClaimName}, nodeClaim)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	stored := nodeClaim.DeepCopy()
	storedMachine := machine.DeepCopy()
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("nodeClaimName", nodeClaim.Name, "machineName", machine.Name))

	nodeClaim.Status.ProviderID = lo.FromPtr(machine.Spec.ProviderID)
	machine.Labels[api.LabelProviderIDInitialized] = "true"
	log.FromContext(ctx).Info("updating nodeclaim status", "providerID", nodeClaim.Status.ProviderID)
	if !equality.Semantic.DeepEqual(nodeClaim, stored) {
		if err := c.kubeClient.Status().Patch(ctx, nodeClaim, client.MergeFrom(stored)); err != nil {
			log.FromContext(ctx).Error(err, "updated nodeclaim failed", nodeClaim.Status.ProviderID, stored.Status.ProviderID)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
	}
	if !equality.Semantic.DeepEqual(machine, storedMachine) {
		if err := c.kubeClient.Status().Patch(ctx, machine, client.MergeFrom(storedMachine)); err != nil {
			log.FromContext(ctx).Error(err, "updated machine failed", machine.Spec.ProviderID, machine.Name)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
	}
	return reconcile.Result{}, nil
}

func (c *ControllerMachine) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("machine.providerid").
		For(&capiv1beta1.Machine{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(o client.Object) bool {
			return len(lo.FromPtr(o.(*capiv1beta1.Machine).Spec.ProviderID)) != 0 &&
				o.(*capiv1beta1.Machine).OwnerReferences != nil &&
				(o.(*capiv1beta1.Machine).GetLabels() != nil &&
					o.(*capiv1beta1.Machine).GetLabels()[api.LabelProviderIDInitialized] == "" &&
					o.(*capiv1beta1.Machine).GetLabels()[api.LabelNodeClaim] != "")
		})).
		// Ok with using the default MaxConcurrentReconciles of 1 to avoid throttling from CreateTag write API
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
