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

package status

import (
	"context"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/vpc"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/zone"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/api/equality"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/karpenter/pkg/operator/injection"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	sshkeyprovider "github.com/tencentcloud/karpenter-provider-tke/pkg/providers/sshkey"
	"sigs.k8s.io/karpenter/pkg/utils/result"
)

type nodeClassStatusReconciler interface {
	Reconcile(context.Context, *api.TKEMachineNodeClass) (reconcile.Result, error)
}

type Controller struct {
	kubeClient client.Client

	subnet    *Subnet
	sg        *SecurityGroup
	sshkey    *SSHKey
	readiness *Readiness
}

func NewController(kubeClient client.Client, zoneProvider zone.Provider, vpcProvider vpc.Provider, sshKeyProvider sshkeyprovider.Provider) *Controller {
	return &Controller{
		kubeClient: kubeClient,

		subnet:    &Subnet{zoneProvider: zoneProvider, vpcProvider: vpcProvider},
		sg:        &SecurityGroup{vpcProvider: vpcProvider},
		sshkey:    &SSHKey{sshKeyProvider: sshKeyProvider},
		readiness: &Readiness{},
	}
}

func (c *Controller) Reconcile(ctx context.Context, nodeClass *api.TKEMachineNodeClass) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "nodeclass.status")

	if !controllerutil.ContainsFinalizer(nodeClass, api.TerminationFinalizer) {
		stored := nodeClass.DeepCopy()
		controllerutil.AddFinalizer(nodeClass, api.TerminationFinalizer)
		if err := c.kubeClient.Patch(ctx, nodeClass, client.MergeFrom(stored)); err != nil {
			return reconcile.Result{}, err
		}
	}
	stored := nodeClass.DeepCopy()

	var results []reconcile.Result
	var errs error
	for _, reconciler := range []nodeClassStatusReconciler{
		c.subnet,
		c.sg,
		c.sshkey,
		c.readiness,
	} {
		res, err := reconciler.Reconcile(ctx, nodeClass)
		errs = multierr.Append(errs, err)
		results = append(results, res)
	}

	if !equality.Semantic.DeepEqual(stored, nodeClass) {
		if err := c.kubeClient.Status().Patch(ctx, nodeClass, client.MergeFrom(stored)); err != nil {
			errs = multierr.Append(errs, client.IgnoreNotFound(err))
		}
	}
	if errs != nil {
		return reconcile.Result{}, errs
	}
	return result.Min(results...), nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("nodeclass.status").
		For(&api.TKEMachineNodeClass{}).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
