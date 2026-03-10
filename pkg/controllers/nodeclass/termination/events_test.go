package termination

import (
	"testing"

	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestWaitingOnNodeClaimTerminationEvent(t *testing.T) {
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
			UID:  types.UID("test-uid-123"),
		},
	}
	names := []string{"nc-1", "nc-2", "nc-3"}
	event := WaitingOnNodeClaimTerminationEvent(nodeClass, names)

	if event.InvolvedObject != nodeClass {
		t.Error("expected involved object to be the node class")
	}
	if event.Type != corev1.EventTypeNormal {
		t.Errorf("expected Normal event type, got %s", event.Type)
	}
	if event.Reason != "WaitingOnNodeClaimTermination" {
		t.Errorf("expected reason 'WaitingOnNodeClaimTermination', got %s", event.Reason)
	}
	if event.Message == "" {
		t.Error("expected non-empty message")
	}
	if len(event.DedupeValues) != 1 {
		t.Fatalf("expected 1 dedupe value, got %d", len(event.DedupeValues))
	}
	if event.DedupeValues[0] != "test-uid-123" {
		t.Errorf("expected dedupe value 'test-uid-123', got %s", event.DedupeValues[0])
	}
}

func TestWaitingOnNodeClaimTerminationEvent_ManyNames(t *testing.T) {
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
			UID:  types.UID("uid"),
		},
	}
	names := []string{"nc-1", "nc-2", "nc-3", "nc-4", "nc-5", "nc-6", "nc-7"}
	event := WaitingOnNodeClaimTerminationEvent(nodeClass, names)
	// PrettySlice with maxItems=5 should truncate
	expected := "Waiting on NodeClaim termination for nc-1, nc-2, nc-3, nc-4, nc-5 and 2 other(s)"
	if event.Message != expected {
		t.Errorf("expected message %q, got %q", expected, event.Message)
	}
}

func TestWaitingOnNodeClaimTerminationEvent_EmptyNames(t *testing.T) {
	nodeClass := &api.TKEMachineNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
			UID:  types.UID("uid"),
		},
	}
	event := WaitingOnNodeClaimTerminationEvent(nodeClass, []string{})
	expected := "Waiting on NodeClaim termination for "
	if event.Message != expected {
		t.Errorf("expected message %q, got %q", expected, event.Message)
	}
}
