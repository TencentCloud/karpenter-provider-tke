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

package interruption

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func nodeWithTaints(taintKeys ...string) *corev1.Node {
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	for _, k := range taintKeys {
		n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{Key: k, Effect: corev1.TaintEffectNoSchedule})
	}
	return n
}

func TestHasSpotInterruptionTaint(t *testing.T) {
	cases := []struct {
		name   string
		taints []string
		want   bool
	}{
		{"no taints", nil, false},
		{"unrelated taint", []string{"other/taint"}, false},
		{"spot itn only", []string{SpotInterruptionTaintKey}, true},
		{"spot itn with others", []string{"other/taint", SpotInterruptionTaintKey, "yet.another/taint"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSpotInterruptionTaint(nodeWithTaints(tc.taints...))
			if got != tc.want {
				t.Fatalf("hasSpotInterruptionTaint(%v) = %v, want %v", tc.taints, got, tc.want)
			}
		})
	}
}

func TestSpotInterruptionTaintKeyMatchesTKENodeTerminationHandler(t *testing.T) {
	// Guard against accidental drift: tke-node-termination-handler on TKE nodes
	// applies this exact key. Changing it in code without coordinating with the
	// DaemonSet silently disables the interruption handler.
	const expected = "tke-node-termination-handler/spot-itn"
	if SpotInterruptionTaintKey != expected {
		t.Fatalf("SpotInterruptionTaintKey drift: %q != %q", SpotInterruptionTaintKey, expected)
	}
}

func TestPredicate_CreateFunc(t *testing.T) {
	p := spotInterruptionTaintPredicate()
	if p.Create(event.CreateEvent{Object: nodeWithTaints()}) {
		t.Fatalf("create with no taint should not enqueue")
	}
	if !p.Create(event.CreateEvent{Object: nodeWithTaints(SpotInterruptionTaintKey)}) {
		t.Fatalf("create with spot-itn taint should enqueue")
	}
}

func TestPredicate_UpdateFunc(t *testing.T) {
	p := spotInterruptionTaintPredicate()

	// no taint -> no taint: skip
	if p.Update(event.UpdateEvent{ObjectOld: nodeWithTaints(), ObjectNew: nodeWithTaints()}) {
		t.Fatalf("update untainted -> untainted should not enqueue")
	}
	// no taint -> taint: enqueue (this is the primary event we care about)
	if !p.Update(event.UpdateEvent{ObjectOld: nodeWithTaints(), ObjectNew: nodeWithTaints(SpotInterruptionTaintKey)}) {
		t.Fatalf("update untainted -> tainted should enqueue")
	}
	// taint -> no taint: skip (nothing to do, taint was removed)
	if p.Update(event.UpdateEvent{ObjectOld: nodeWithTaints(SpotInterruptionTaintKey), ObjectNew: nodeWithTaints()}) {
		t.Fatalf("update tainted -> untainted should not enqueue")
	}
	// taint -> taint with same providerID: skip (avoid noisy re-reconciles)
	tainted1 := nodeWithTaints(SpotInterruptionTaintKey)
	tainted1.Spec.ProviderID = "qcloud:///gz/ins-1"
	tainted2 := tainted1.DeepCopy()
	if p.Update(event.UpdateEvent{ObjectOld: tainted1, ObjectNew: tainted2}) {
		t.Fatalf("update tainted -> tainted with same providerID should not re-enqueue")
	}
	// taint -> taint with providerID freshly populated: enqueue
	taintedNoPID := nodeWithTaints(SpotInterruptionTaintKey)
	taintedWithPID := taintedNoPID.DeepCopy()
	taintedWithPID.Spec.ProviderID = "qcloud:///gz/ins-2"
	if !p.Update(event.UpdateEvent{ObjectOld: taintedNoPID, ObjectNew: taintedWithPID}) {
		t.Fatalf("update tainted -> tainted with newly populated providerID should enqueue")
	}
}

func TestPredicate_DeleteAndGenericFuncs(t *testing.T) {
	p := spotInterruptionTaintPredicate()
	if p.Delete(event.DeleteEvent{Object: nodeWithTaints(SpotInterruptionTaintKey)}) {
		t.Fatalf("delete events should never enqueue (nothing to drain when node is gone)")
	}
	if p.Generic(event.GenericEvent{Object: nodeWithTaints(SpotInterruptionTaintKey)}) {
		t.Fatalf("generic events should not enqueue")
	}
}
