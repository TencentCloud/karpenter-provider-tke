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

package common

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karptesting "sigs.k8s.io/karpenter/pkg/utils/testing"

	"github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	"github.com/tencentcloud/karpenter-provider-tke/test/pkg/debug"
)

func init() {
	lo.Must0(v1beta1.AddToScheme(scheme.Scheme))
	// karpv1 types are registered via init() in doc.go
	_ = karpv1.NodePool{}
}

type Environment struct {
	Context    context.Context
	Config     *rest.Config
	Client     client.Client
	KubeClient client.Client // raw client without cache
	Monitor    *Monitor

	StartingNodeCount int
	DefaultTimeout    time.Duration
}

func NewEnvironment(t *testing.T) *Environment {
	ctx := karptesting.TestContextWithLogger(t)
	config := controllerruntime.GetConfigOrDie()
	config.QPS = 200
	config.Burst = 300

	c := newCacheSyncingClient(ctx, config)
	kubeClient := lo.Must(client.New(config, client.Options{Scheme: scheme.Scheme}))
	monitor := NewMonitor(ctx, c)

	return &Environment{
		Context:        ctx,
		Config:         config,
		Client:         c,
		KubeClient:     kubeClient,
		Monitor:        monitor,
		DefaultTimeout: 16 * time.Minute,
	}
}

func newCacheSyncingClient(ctx context.Context, config *rest.Config) client.Client {
	cacheObj := lo.Must(cache.New(config, cache.Options{Scheme: scheme.Scheme}))

	// index Pod by spec.nodeName
	lo.Must0(cacheObj.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
		pod := o.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}))
	// index Event by involvedObject.kind
	lo.Must0(cacheObj.IndexField(ctx, &corev1.Event{}, "involvedObject.kind", func(o client.Object) []string {
		event := o.(*corev1.Event)
		return []string{event.InvolvedObject.Kind}
	}))
	// index Node by spec.unschedulable
	lo.Must0(cacheObj.IndexField(ctx, &corev1.Node{}, "spec.unschedulable", func(o client.Object) []string {
		node := o.(*corev1.Node)
		if node.Spec.Unschedulable {
			return []string{"true"}
		}
		return []string{"false"}
	}))

	go func() {
		lo.Must0(cacheObj.Start(ctx))
	}()
	if !cacheObj.WaitForCacheSync(ctx) {
		panic("cache failed to sync")
	}

	return lo.Must(client.New(config, client.Options{
		Scheme: scheme.Scheme,
		Cache: &client.CacheOptions{
			Reader: cacheObj,
		},
	}))
}

func (env *Environment) DebugMonitor() *debug.Monitor {
	return debug.New(env.Context, env.Config, env.Client)
}
