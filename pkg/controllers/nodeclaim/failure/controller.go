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

package failure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/awslabs/operatorpkg/singleton"
	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/pkg/providers/instancetype"
	capiv1beta1 "github.com/tencentcloud/karpenter-provider-tke/staging/nativenode/v1beta1"
	"go.uber.org/multierr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
)

type Controller struct {
	kubeClient           client.Client
	instancetypeProvider instancetype.Provider
	successfulCount      uint64 // keeps track of successful reconciles for more aggressive requeueing near the start of the controller
}

func NewController(kubeClient client.Client, instancetypeProvider instancetype.Provider) *Controller {
	return &Controller{
		kubeClient:           kubeClient,
		instancetypeProvider: instancetypeProvider,
		successfulCount:      0,
	}
}

func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "nodeclaim.failure")

	machineList := &capiv1beta1.MachineList{}
	if err := c.kubeClient.List(ctx, machineList); err != nil {
		return reconcile.Result{}, err
	}
	insufficientMachines := lo.Filter(machineList.Items, func(m capiv1beta1.Machine, _ int) bool {
		return lo.FromPtr(m.Spec.ProviderID) == "" && lo.ContainsBy(m.GetOwnerReferences(), func(ref metav1.OwnerReference) bool { return ref.Kind == "NodeClaim" }) &&
			time.Since(m.CreationTimestamp.Time) > 30*time.Second && c.isFailureWithInsufficientResources(ctx, m)
	})
	c.refreshInstanceTypes(ctx, insufficientMachines)

	unknowFailureMachines := lo.Filter(machineList.Items, func(m capiv1beta1.Machine, _ int) bool {
		return lo.FromPtr(m.Spec.ProviderID) == "" && lo.ContainsBy(m.GetOwnerReferences(), func(ref metav1.OwnerReference) bool { return ref.Kind == "NodeClaim" }) &&
			time.Since(m.CreationTimestamp.Time) > 30*time.Second && lo.FromPtr(m.Status.FailureMessage) != "" && !c.isFailureWithInsufficientResources(ctx, m)
	})
	c.blockInstanceTypes(ctx, unknowFailureMachines)

	failureMachines := append(insufficientMachines, unknowFailureMachines...)
	errs := make([]error, len(failureMachines))
	workqueue.ParallelizeUntil(ctx, 100, len(failureMachines), func(i int) {
		errs[i] = c.deleteFailureMachine(ctx, failureMachines[i])
	})
	if err := multierr.Combine(errs...); err != nil {
		return reconcile.Result{}, err
	}
	c.successfulCount++
	return reconcile.Result{RequeueAfter: lo.Ternary(c.successfulCount <= 20, time.Second*10, time.Minute)}, nil
}

func (c *Controller) refreshInstanceTypes(ctx context.Context, insufficientMachines []capiv1beta1.Machine) {
	isRefreshed := false
	for _, m := range insufficientMachines {
		log.FromContext(ctx).Info("try to refresh instance types", "machine", m.Name)
		nodeClassName := m.GetLabels()[api.LabelNodeClass]
		if len(nodeClassName) == 0 {
			continue
		}
		nodeClass := &api.TKEMachineNodeClass{}
		if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClassName, Namespace: m.Namespace}, nodeClass); err != nil {
			log.FromContext(ctx).Error(err, "unable to get nodeclass", "nodeclass", nodeClassName)
			continue
		}

		providerSpec, err := capiv1beta1.ProviderSpecFromRawExtension(m.Spec.ProviderSpec.Value)
		if err != nil {
			log.FromContext(ctx).Error(err, "unable to get provider spec", "machine", m.Name)
			continue
		}
		insType := providerSpec.InstanceType
		capacityType := m.GetLabels()[v1.CapacityTypeLabelKey]
		zoneName := m.Spec.Zone

		c.instancetypeProvider.AddInsufficientFailure(ctx, insType, capacityType, zoneName)
		if capacityType == v1.CapacityTypeSpot {
			c.instancetypeProvider.AddInsufficientFailure(ctx, "*", capacityType, zoneName)
		}

		if !isRefreshed {
			_, err := c.instancetypeProvider.List(ctx, nodeClass, true)
			if err != nil {
				log.FromContext(ctx).Error(err, "unable to get instance types", "nodeclass", nodeClassName)
				time.Sleep(time.Second)
				continue
			} else {
				isRefreshed = true
			}
		}
	}
}

func (c *Controller) blockInstanceTypes(ctx context.Context, machines []capiv1beta1.Machine) {
	for _, m := range machines {
		providerSpec, err := capiv1beta1.ProviderSpecFromRawExtension(m.Spec.ProviderSpec.Value)
		if err != nil {
			log.FromContext(ctx).Error(err, "unable to get provider spec", "machine", m.Name)
			continue
		}
		insType := providerSpec.InstanceType
		capacityType := m.GetLabels()[v1.CapacityTypeLabelKey]
		zoneName := m.Spec.Zone
		failureMessage := lo.FromPtr(m.Status.FailureMessage)
		if strings.Contains(lo.FromPtr(m.Status.FailureMessage), "LimitExceeded.UserSpotQuota") {
			failureMessage = fmt.Sprintf("user spot quota in %s has been exceeded", zoneName)
			insType = "*"
		}
		c.instancetypeProvider.BlockInstanceType(ctx, insType, capacityType, zoneName, fmt.Sprintf("controller block: %s", failureMessage))
	}
}

func (c *Controller) deleteFailureMachine(ctx context.Context, machine capiv1beta1.Machine) error {
	log.FromContext(ctx).Info("try to delete failure machine", "machine", machine.Name)
	for _, owner := range machine.GetOwnerReferences() {
		if owner.Kind == "NodeClaim" {
			nc := &v1.NodeClaim{}
			nc.Name = owner.Name
			if err := c.kubeClient.Delete(ctx, nc); err != nil {
				return client.IgnoreNotFound(err)
			}
			log.FromContext(ctx).Info("deleted failure machine nodeclaim", "nodeclaim", owner.Name)
			if err := c.kubeClient.Delete(ctx, &machine); err != nil {
				return client.IgnoreNotFound(err)
			}
			log.FromContext(ctx).WithValues("Node", klog.KRef("", machine.Name)).Info("deleted failure machine", "machine", machine.Name, "failureMessage", lo.FromPtr(machine.Status.FailureMessage))
		}
		break
	}
	return nil
}

func (c *Controller) isFailureWithInsufficientResources(ctx context.Context, m capiv1beta1.Machine) bool {
	result := lo.FromPtr(m.Status.FailureReason) == capiv1beta1.InvalidConfigurationMachineError &&
		(strings.Contains(lo.FromPtr(m.Status.FailureMessage), "Insufficient resources of") ||
			strings.Contains(lo.FromPtr(m.Status.FailureMessage), "InvalidParameterValue.InsufficientOffering") ||
			strings.Contains(lo.FromPtr(m.Status.FailureMessage), "ResourceInsufficient.SpecifiedInstanceType") ||
			strings.Contains(lo.FromPtr(m.Status.FailureMessage), "LimitExceeded.SpotQuota"))
	if !result {
		return false
	}
	providerSpec, err := capiv1beta1.ProviderSpecFromRawExtension(m.Spec.ProviderSpec.Value)
	if err != nil {
		log.FromContext(ctx).Error(err, "unable to get provider spec", "machine", m.Name)
		return false
	}
	insType := providerSpec.InstanceType
	capacityType := m.GetLabels()[v1.CapacityTypeLabelKey]
	zoneName := m.Spec.Zone
	failureMessage := lo.FromPtr(m.Status.FailureMessage)
	// if spot is insufficient for 50 times(1h), all spot instance type will be processed as unknown failure
	if capacityType == v1.CapacityTypeSpot {
		if c.instancetypeProvider.GetInsufficientFailureCount(ctx, "*", capacityType, zoneName) > 50 {
			log.FromContext(ctx).Error(fmt.Errorf("insufficient %s failure in %s > 50 times", capacityType, zoneName), failureMessage, "zoneName", zoneName)
			return false
		}
	}
	// if an instance type is insufficient for 3 times(1h), this instance type wiil be processed as unknown failure
	if c.instancetypeProvider.GetInsufficientFailureCount(ctx, insType, capacityType, zoneName) > 3 {
		log.FromContext(ctx).Error(fmt.Errorf("insufficient %s failure with %s > 3 times", capacityType, insType), failureMessage, "instanceType", insType, "capacityType", capacityType, "zoneName", zoneName)
		return false
	}
	return true
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("nodeclaim.failure").
		WatchesRawSource(singleton.Source()).
		Complete(singleton.AsReconciler(c))
}
