package vpc

import (
	"testing"

	"github.com/samber/lo"
	api "github.com/tencentcloud/karpenter-provider-tke/pkg/apis/v1beta1"
)

func TestGetSGFilterSets_IDFilter(t *testing.T) {
	terms := []api.SecurityGroupSelectorTerm{
		{ID: "sg-12345"},
		{ID: "sg-67890"},
	}
	filterSets := getSGFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set (consolidated IDs), got %d", len(filterSets))
	}
	// The ID filter should contain both IDs
	idFilter := filterSets[0][0]
	if lo.FromPtr(idFilter.Name) != "security-group-id" {
		t.Errorf("expected filter name 'security-group-id', got %q", lo.FromPtr(idFilter.Name))
	}
	if len(idFilter.Values) != 2 {
		t.Errorf("expected 2 values, got %d", len(idFilter.Values))
	}
}

func TestGetSGFilterSets_TagFilter(t *testing.T) {
	terms := []api.SecurityGroupSelectorTerm{
		{Tags: map[string]string{"env": "prod"}},
	}
	filterSets := getSGFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag:env" {
		t.Errorf("expected filter name 'tag:env', got %q", lo.FromPtr(filter.Name))
	}
}

func TestGetSGFilterSets_TagWildcard(t *testing.T) {
	terms := []api.SecurityGroupSelectorTerm{
		{Tags: map[string]string{"karpenter": "*"}},
	}
	filterSets := getSGFilterSets(terms)
	if len(filterSets) != 1 {
		t.Fatalf("expected 1 filter set, got %d", len(filterSets))
	}
	filter := filterSets[0][0]
	if lo.FromPtr(filter.Name) != "tag-key" {
		t.Errorf("expected filter name 'tag-key', got %q", lo.FromPtr(filter.Name))
	}
}

func TestGetSGFilterSets_Mixed(t *testing.T) {
	terms := []api.SecurityGroupSelectorTerm{
		{ID: "sg-12345"},
		{Tags: map[string]string{"env": "prod"}},
	}
	filterSets := getSGFilterSets(terms)
	// Should have 2 filter sets: one for tags, one for consolidated IDs
	if len(filterSets) != 2 {
		t.Errorf("expected 2 filter sets, got %d", len(filterSets))
	}
}

func TestGetSGFilterSets_Empty(t *testing.T) {
	filterSets := getSGFilterSets(nil)
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets for nil terms, got %d", len(filterSets))
	}
}

func TestGetSGFilterSets_EmptyTerms(t *testing.T) {
	filterSets := getSGFilterSets([]api.SecurityGroupSelectorTerm{})
	if len(filterSets) != 0 {
		t.Errorf("expected 0 filter sets for empty terms, got %d", len(filterSets))
	}
}
