package vpc

import (
	"testing"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
)

func TestGetSubnetFilterSets_IDFilter(t *testing.T) {
	terms := []api.SubnetSelectorTerm{
		{ID: "subnet-12345"},
		{ID: "subnet-67890"},
	}
	filterSets := getSubnetFilterSets(terms)
	// Both IDs should be in a single batch (since < 5)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set for 2 IDs, got %d", len(filterSets))
	}
	idFilter := filterSets[0][0]
	if lo.FromPtr(idFilter.Name) != "subnet-id" {
		t.Errorf("expected filter name 'subnet-id', got %q", lo.FromPtr(idFilter.Name))
	}
	if len(idFilter.Values) != 2 {
		t.Errorf("expected 2 values, got %d", len(idFilter.Values))
	}
}

func TestGetSubnetFilterSets_IDFilter_BatchSize(t *testing.T) {
	// 7 IDs should create 2 batches (5 + 2)
	terms := []api.SubnetSelectorTerm{
		{ID: "subnet-1"},
		{ID: "subnet-2"},
		{ID: "subnet-3"},
		{ID: "subnet-4"},
		{ID: "subnet-5"},
		{ID: "subnet-6"},
		{ID: "subnet-7"},
	}
	filterSets := getSubnetFilterSets(terms)
	if len(filterSets) != 2 {
		t.Fatalf("expected 2 batches for 7 IDs, got %d", len(filterSets))
	}
	// First batch: 5 IDs
	if len(filterSets[0][0].Values) != 5 {
		t.Errorf("expected 5 IDs in first batch, got %d", len(filterSets[0][0].Values))
	}
	// Second batch: 2 IDs
	if len(filterSets[1][0].Values) != 2 {
		t.Errorf("expected 2 IDs in second batch, got %d", len(filterSets[1][0].Values))
	}
}

func TestGetSubnetFilterSets_IDFilter_ExactBatchSize(t *testing.T) {
	// 5 IDs should create exactly 1 batch
	terms := []api.SubnetSelectorTerm{
		{ID: "subnet-1"},
		{ID: "subnet-2"},
		{ID: "subnet-3"},
		{ID: "subnet-4"},
		{ID: "subnet-5"},
	}
	filterSets := getSubnetFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 batch for 5 IDs, got %d", len(filterSets))
	}
	if len(filterSets[0][0].Values) != 5 {
		t.Errorf("expected 5 IDs in batch, got %d", len(filterSets[0][0].Values))
	}
}

func TestGetSubnetFilterSets_TagFilter(t *testing.T) {
	terms := []api.SubnetSelectorTerm{
		{Tags: map[string]string{"env": "prod"}},
	}
	filterSets := getSubnetFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag:env" {
		t.Errorf("expected filter name 'tag:env', got %q", lo.FromPtr(filter.Name))
	}
}

func TestGetSubnetFilterSets_TagWildcard(t *testing.T) {
	terms := []api.SubnetSelectorTerm{
		{Tags: map[string]string{"karpenter": "*"}},
	}
	filterSets := getSubnetFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag-key" {
		t.Errorf("expected filter name 'tag-key', got %q", lo.FromPtr(filter.Name))
	}
}

func TestGetSubnetFilterSets_Mixed(t *testing.T) {
	terms := []api.SubnetSelectorTerm{
		{ID: "subnet-12345"},
		{Tags: map[string]string{"env": "prod"}},
	}
	filterSets := getSubnetFilterSets(terms)
	// Should have 2 filter sets: tag filter + ID batch
	if len(filterSets) != 2 {
		t.Errorf("expected 2 filter sets, got %d", len(filterSets))
	}
}

func TestGetSubnetFilterSets_Empty(t *testing.T) {
	filterSets := getSubnetFilterSets(nil)
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets for nil terms, got %d", len(filterSets))
	}
}

func TestGetSubnetFilterSets_EmptyTerms(t *testing.T) {
	filterSets := getSubnetFilterSets([]api.SubnetSelectorTerm{})
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets for empty terms, got %d", len(filterSets))
	}
}
