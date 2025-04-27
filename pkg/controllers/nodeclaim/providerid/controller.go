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
	"fmt"
	"time"

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

type Controller struct {
	kubeClient client.Client
}

func NewController(kubeClient client.Client) *Controller {
	return &Controller{
		kubeClient: kubeClient,
	}
}

func (c *Controller) Reconcile(ctx context.Context, nodeClaim *v1.NodeClaim) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "nodeclaim.providerid")

	if len(nodeClaim.Status.ProviderID) != 0 {
		return reconcile.Result{}, nil
	}

	stored := nodeClaim.DeepCopy()
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("name", nodeClaim.Name))

	machineList := &capiv1beta1.MachineList{}
	listOptions := []client.ListOption{
		client.MatchingLabels{
			api.LabelNodeClaim: nodeClaim.Name,
		},
	}
	err := c.kubeClient.List(ctx, machineList, listOptions...)
	if err != nil {
		return reconcile.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("listing machines failed: %v", err)
	}

	if len(machineList.Items) == 0 {
		log.FromContext(ctx).Info("no machines found for nodeclaim", "nodClaim", nodeClaim.Name)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if len(lo.FromPtr(machineList.Items[0].Spec.ProviderID)) == 0 {
		log.FromContext(ctx).Info("waiting providerID creating for nodeclaim", machineList.Items[0].Name, nodeClaim.Name)
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
	nodeClaim.Status.ProviderID = lo.FromPtr(machineList.Items[0].Spec.ProviderID)
	log.FromContext(ctx).Info("updating nodeclaim status", nodeClaim.Status.ProviderID, stored.Status.ProviderID)
	if !equality.Semantic.DeepEqual(nodeClaim, stored) {
		if err := c.kubeClient.Status().Patch(ctx, nodeClaim, client.MergeFrom(stored)); err != nil {
			log.FromContext(ctx).Info("updated nodeclaim status", nodeClaim.Status.ProviderID, stored.Status.ProviderID)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
	}
	return reconcile.Result{}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("nodeclaim.providerid").
		For(&v1.NodeClaim{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(o client.Object) bool {
			return len(o.(*v1.NodeClaim).Status.ProviderID) == 0
		})).
		// Ok with using the default MaxConcurrentReconciles of 1 to avoid throttling from CreateTag write API
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
