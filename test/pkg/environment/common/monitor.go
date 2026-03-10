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
	"fmt"
	"sync"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

type Monitor struct {
	ctx context.Context
	mu  sync.RWMutex

	kubeClient   client.Client
	resetNodes   map[string]*corev1.Node
	createdNodes map[string]*corev1.Node
}

func NewMonitor(ctx context.Context, kubeClient client.Client) *Monitor {
	return &Monitor{
		ctx:          ctx,
		kubeClient:   kubeClient,
		resetNodes:   make(map[string]*corev1.Node),
		createdNodes: make(map[string]*corev1.Node),
	}
}

func (m *Monitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.resetNodes = make(map[string]*corev1.Node)
	m.createdNodes = make(map[string]*corev1.Node)

	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		fmt.Printf("[MONITOR] failed to list nodes: %v\n", err)
		return
	}
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		if _, ok := n.Labels[karpv1.NodePoolLabelKey]; ok {
			m.resetNodes[n.Name] = n
		}
	}
}

func (m *Monitor) NodeCount() int {
	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		return 0
	}
	count := 0
	for i := range nodeList.Items {
		if _, ok := nodeList.Items[i].Labels[karpv1.NodePoolLabelKey]; ok {
			count++
		}
	}
	return count
}

func (m *Monitor) NodeCountAtReset() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.resetNodes)
}

func (m *Monitor) CreatedNodeCount() int {
	return len(m.CreatedNodes())
}

func (m *Monitor) CreatedNodes() []*corev1.Node {
	m.mu.RLock()
	resetNames := make(map[string]bool)
	for name := range m.resetNodes {
		resetNames[name] = true
	}
	m.mu.RUnlock()

	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		return nil
	}
	var created []*corev1.Node
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		if _, ok := n.Labels[karpv1.NodePoolLabelKey]; !ok {
			continue
		}
		if !resetNames[n.Name] {
			created = append(created, n)
		}
	}
	return created
}

func (m *Monitor) InitializedNodes() []*corev1.Node {
	m.mu.RLock()
	resetNames := make(map[string]bool)
	for name := range m.resetNodes {
		resetNames[name] = true
	}
	m.mu.RUnlock()

	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		return nil
	}
	var initialized []*corev1.Node
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		if _, ok := n.Labels[karpv1.NodePoolLabelKey]; !ok {
			continue
		}
		if resetNames[n.Name] {
			continue
		}
		if n.Labels[karpv1.NodeInitializedLabelKey] == "true" {
			initialized = append(initialized, n)
		}
	}
	return initialized
}

func (m *Monitor) InitializedNodeCount() int {
	return len(m.InitializedNodes())
}

// KarpenterNodes returns all nodes with karpenter.sh/nodepool label
func (m *Monitor) KarpenterNodes() []*corev1.Node {
	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		return nil
	}
	var nodes []*corev1.Node
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		if _, ok := n.Labels[karpv1.NodePoolLabelKey]; ok {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// DeletedNodes returns nodes that were present at reset but are now gone.
func (m *Monitor) DeletedNodes() []*corev1.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeList := &corev1.NodeList{}
	if err := m.kubeClient.List(m.ctx, nodeList); err != nil {
		return nil
	}
	currentNames := make(map[string]bool)
	for i := range nodeList.Items {
		currentNames[nodeList.Items[i].Name] = true
	}
	var deleted []*corev1.Node
	for name, node := range m.resetNodes {
		if !currentNames[name] {
			deleted = append(deleted, node)
		}
	}
	return deleted
}

// RunningPods returns pods matching the selector that are in Running phase and Ready.
func (m *Monitor) RunningPods(selector labels.Selector) []*corev1.Pod {
	podList := &corev1.PodList{}
	if err := m.kubeClient.List(m.ctx, podList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil
	}
	var running []*corev1.Pod
	for i := range podList.Items {
		p := &podList.Items[i]
		if p.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				running = append(running, p)
				break
			}
		}
	}
	return running
}

// PendingPodsCount returns the count of Pending pods matching the selector.
func (m *Monitor) PendingPodsCount(selector labels.Selector) int {
	podList := &corev1.PodList{}
	if err := m.kubeClient.List(m.ctx, podList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return 0
	}
	count := 0
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodPending {
			count++
		}
	}
	return count
}

// RestartCount returns a map of "pod/container" -> restart count for the given namespace.
func (m *Monitor) RestartCount(namespace string) map[string]int32 {
	podList := &corev1.PodList{}
	if err := m.kubeClient.List(m.ctx, podList, client.InNamespace(namespace)); err != nil {
		return nil
	}
	result := make(map[string]int32)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			key := fmt.Sprintf("%s/%s", pod.Name, cs.Name)
			result[key] = cs.RestartCount
		}
	}
	return result
}

// NodesAboveResetCount returns all karpenter nodes above the reset snapshot count
func (m *Monitor) NodesAboveResetCount() int {
	return lo.Max([]int{0, m.NodeCount() - m.NodeCountAtReset()})
}

func compare(actual int, comparator string, expected int) bool {
	switch comparator {
	case "==":
		return actual == expected
	case ">=":
		return actual >= expected
	case "<=":
		return actual <= expected
	case ">":
		return actual > expected
	case "<":
		return actual < expected
	default:
		return actual == expected
	}
}
